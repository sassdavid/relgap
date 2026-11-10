package mdindex

import (
	"context"
	"sync"
)

// Key names one file: a path in a repository at a ref.
type Key struct {
	Repo, Ref, Path string
}

// FetchFunc returns the content of the file k names.
type FetchFunc func(ctx context.Context, k Key) ([]byte, error)

// Cache fetches each file once and parses it once per heading pattern. It is safe for
// concurrent use and lives for one digest.
type Cache struct {
	fetch FetchFunc

	mu    sync.Mutex
	files map[Key]*file
}

type file struct {
	once sync.Once
	src  []byte
	err  error

	mu      sync.Mutex
	indexes map[string]*Index // by heading pattern
}

// NewCache returns a cache that reads files with fetch.
func NewCache(fetch FetchFunc) *Cache {
	return &Cache{fetch: fetch, files: map[Key]*file{}}
}

// Get returns the index of the file k names, fetching and parsing it on first use. A failed
// fetch is remembered, so it isn't retried within the digest.
func (c *Cache) Get(ctx context.Context, k Key, o Options) (*Index, error) {
	c.mu.Lock()
	f, ok := c.files[k]
	if !ok {
		f = &file{indexes: map[string]*Index{}}
		c.files[k] = f
	}
	c.mu.Unlock()
	f.once.Do(func() { f.src, f.err = c.fetch(ctx, k) })
	if f.err != nil {
		return nil, f.err
	}
	pattern := ""
	if o.Heading != nil {
		pattern = o.Heading.String()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if x, cached := f.indexes[pattern]; cached {
		return x, nil
	}
	x, err := Parse(f.src, o)
	if err != nil {
		return nil, err
	}
	f.indexes[pattern] = x
	return x, nil
}
