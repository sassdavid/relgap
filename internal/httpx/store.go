package httpx

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/klauspost/compress/zstd"
)

// formatDir names the on-disk format. A new format gets a new directory and prune removes the old.
const formatDir = "v1"

const (
	entryExt   = ".zst"
	tmpPattern = "*.tmp"
	dirMode    = 0o700
	fileMode   = 0o600
	maxBody    = 64 << 20 // decompressed bytes per entry
)

// keptHeaders returns the response headers an entry stores. Request headers are never stored.
func keptHeaders() []string {
	return []string{"Content-Type", "ETag", "Last-Modified", "Link", "X-Github-Media-Type"}
}

// meta is the first line of an entry file, as plain JSON. The zstd body follows it.
type meta struct {
	Method       string      `json:"method"`
	URL          string      `json:"url"`
	Host         string      `json:"host"`
	Header       http.Header `json:"header"`
	ETag         string      `json:"etag,omitempty"`
	LastModified string      `json:"last_modified,omitempty"`
	StoredAt     time.Time   `json:"stored_at"`        // last fetched or revalidated
	Newest       time.Time   `json:"newest,omitzero"`  // newest date the body describes; dates the TTL
	Status       int         `json:"status,omitempty"` // 404 for a remembered miss; 0 means 200
}

type entry struct {
	meta meta
	body []byte
}

func newMeta(req *http.Request, resp *http.Response, now time.Time) meta {
	h := http.Header{}
	for _, k := range keptHeaders() {
		if v := resp.Header.Values(k); len(v) > 0 {
			h[http.CanonicalHeaderKey(k)] = v
		}
	}
	m := meta{
		Method:       req.Method,
		URL:          req.URL.Redacted(),
		Host:         req.URL.Host,
		Header:       h,
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
		StoredAt:     now.UTC(),
	}
	if t, err := http.ParseTime(m.LastModified); err == nil {
		m.Newest = t.UTC()
	}
	return m
}

// revalidated records a 304: the entry is fresh again and takes any new validators.
func (m *meta) revalidated(h http.Header, now time.Time) {
	m.StoredAt = now.UTC()
	if v := h.Get("ETag"); v != "" {
		m.ETag = v
		m.Header.Set("ETag", v)
	}
	if v := h.Get("Last-Modified"); v != "" {
		m.LastModified = v
		m.Header.Set("Last-Modified", v)
	}
}

// store reads and writes entry files under dir/http/v1/<2 hex>/<sha256>.zst.
type store struct {
	root string // dir/http; empty when the cache is off

	once sync.Once
	enc  *zstd.Encoder
	dec  *zstd.Decoder
	err  error
}

func newStore(dir string) *store {
	s := &store{}
	if dir != "" {
		s.root = filepath.Join(dir, "http")
	}
	return s
}

func (s *store) codec() error {
	s.once.Do(func() {
		s.enc, s.err = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
		if s.err == nil {
			s.dec, s.err = zstd.NewReader(nil, zstd.WithDecoderMaxMemory(maxBody))
		}
	})
	return s.err
}

func (s *store) close() {
	if s.dec != nil {
		s.dec.Close()
	}
}

func (s *store) path(key string) string {
	return filepath.Join(s.root, formatDir, key[:2], key+entryExt)
}

func (s *store) load(key string) (*entry, bool) {
	if s.root == "" || s.codec() != nil {
		return nil, false
	}
	f, err := os.Open(s.path(key))
	if err != nil {
		return nil, false
	}
	defer f.Close()
	r := bufio.NewReader(f)
	m, err := readMeta(r)
	if err != nil {
		return nil, false
	}
	z, err := io.ReadAll(r)
	if err != nil {
		return nil, false
	}
	body, err := s.dec.DecodeAll(z, nil)
	if err != nil {
		return nil, false
	}
	return &entry{meta: m, body: body}, true
}

func readMeta(r *bufio.Reader) (meta, error) {
	var m meta
	line, err := r.ReadBytes('\n')
	if err != nil {
		return m, fmt.Errorf("read entry header: %w", err)
	}
	if err = json.Unmarshal(line, &m); err != nil {
		return m, fmt.Errorf("decode entry header: %w", err)
	}
	return m, nil
}

// save writes e through a temporary file and a rename, so readers never see half an entry.
func (s *store) save(key string, e *entry, now time.Time) error {
	if s.root == "" {
		return nil
	}
	if err := s.codec(); err != nil {
		return fmt.Errorf("start zstd: %w", err)
	}
	head, err := json.Marshal(e.meta)
	if err != nil {
		return fmt.Errorf("encode entry header: %w", err)
	}
	path := s.path(key)
	if err = os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}
	var buf bytes.Buffer
	buf.Write(head)
	buf.WriteByte('\n')
	buf.Write(s.enc.EncodeAll(e.body, nil))
	tmp, err := os.CreateTemp(filepath.Dir(path), tmpPattern)
	if err != nil {
		return fmt.Errorf("create cache entry: %w", err)
	}
	_, err = tmp.Write(buf.Bytes())
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write cache entry: %w", err)
	}
	s.touch(key, now)
	return nil
}

// touch sets the entry's mtime to its last use, which prune reads.
func (s *store) touch(key string, now time.Time) {
	if s.root != "" {
		_ = os.Chtimes(s.path(key), now, now)
	}
}

// Info describes the cache on disk.
type Info struct {
	Dir     string `json:"dir"`
	Entries int    `json:"entries"`
	Bytes   int64  `json:"bytes"`
	Fresh   int    `json:"fresh"`
	Stale   int    `json:"stale"`
}

// Removed says what [Cache.Clear] or [Cache.Prune] deleted.
type Removed struct {
	Dir     string `json:"dir"`
	Entries int    `json:"entries"`
	Bytes   int64  `json:"bytes"`
}

// Info counts the entries and their size, and how many are within their TTL.
func (c *Cache) Info() (Info, error) {
	info := Info{Dir: c.store.root}
	now := c.opts.Clock()
	err := c.store.walk(func(path string, fi fs.FileInfo) error {
		if !isEntry(path) {
			return nil
		}
		info.Entries++
		info.Bytes += fi.Size()
		m, err := readMetaFile(path)
		if err == nil && c.fresh(m, now) {
			info.Fresh++
		} else {
			info.Stale++
		}
		return nil
	})
	return info, err
}

// Clear deletes every entry.
func (c *Cache) Clear() (Removed, error) {
	r := Removed{Dir: c.store.root}
	err := c.store.walk(func(_ string, fi fs.FileInfo) error {
		r.Entries++
		r.Bytes += fi.Size()
		return nil
	})
	if err != nil {
		return r, err
	}
	if err = os.RemoveAll(c.store.root); err != nil {
		return r, fmt.Errorf("clear cache: %w", err)
	}
	return r, nil
}

// Prune deletes entries not used for olderThan, unreadable entries, leftover temporary files
// and entries in an old format. Expired entries stay, since their validators make 304s possible.
func (c *Cache) Prune(olderThan time.Duration) (Removed, error) {
	r := Removed{Dir: c.store.root}
	cutoff := c.opts.Clock().Add(-olderThan)
	current := filepath.Join(c.store.root, formatDir) + string(filepath.Separator)
	err := c.store.walk(func(path string, fi fs.FileInfo) error {
		drop := !strings.HasPrefix(path, current) || fi.ModTime().Before(cutoff)
		if !drop && isEntry(path) {
			_, metaErr := readMetaFile(path)
			drop = metaErr != nil
		} else if !drop {
			// A temporary file older than an hour belongs to a run that died mid-write.
			drop = fi.ModTime().Before(c.opts.Clock().Add(-time.Hour))
		}
		if !drop {
			return nil
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("prune cache: %w", err)
		}
		r.Entries++
		r.Bytes += fi.Size()
		return nil
	})
	if err == nil {
		removeEmptyDirs(c.store.root)
	}
	return r, err
}

// walk calls fn for every regular file under the store root. A missing root is empty.
func (s *store) walk(fn func(path string, fi fs.FileInfo) error) error {
	if s.root == "" {
		return nil
	}
	err := filepath.WalkDir(s.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return err //nolint:wrapcheck // wrapped once below
		}
		return fn(path, fi)
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("walk %s: %w", s.root, err)
	}
	return nil
}

func isEntry(path string) bool { return strings.HasSuffix(path, entryExt) }

func readMetaFile(path string) (meta, error) {
	f, err := os.Open(path)
	if err != nil {
		return meta{}, fmt.Errorf("open entry: %w", err)
	}
	defer f.Close()
	return readMeta(bufio.NewReader(f))
}

// removeEmptyDirs removes empty directories below root, deepest first.
func removeEmptyDirs(root string) {
	var dirs []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() && path != root {
			dirs = append(dirs, path)
		}
		return nil
	})
	for _, d := range slices.Backward(dirs) {
		_ = os.Remove(d) // fails, as intended, when the directory still has files
	}
}
