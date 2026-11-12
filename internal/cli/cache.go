package cli

import (
	"fmt"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/sassdavid/relgap/internal/app"
	"github.com/sassdavid/relgap/internal/buildinfo"
	"github.com/sassdavid/relgap/internal/config"
	"github.com/sassdavid/relgap/internal/github"
	"github.com/sassdavid/relgap/internal/httpx"
	"github.com/sassdavid/relgap/internal/render"
)

const defaultPruneAge = 30 * 24 * time.Hour

// ttlSources maps the source names cache.ttl_override accepts to the hosts they cover.
func ttlSources() map[string]string {
	return map[string]string{
		"github":    httpx.GitHubAPIHost,
		"endoflife": "endoflife.date",
		"osv":       httpx.OSVHost,
	}
}

func newCacheCmd(d *app.Deps, g *globals) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cache",
		Short: "Inspect or clear the HTTP cache",
		Long: `relgap keeps HTTP responses under $XDG_CACHE_HOME/relgap/http (config cache.dir,
env RELGAP_CACHE_DIR). Each entry holds the response body and a few response headers (content
type, ETag, Last-Modified, Link); request headers and tokens are never stored.

An entry is fresh for a time that depends on the age of the newest release it describes:
1 hour under 7 days, 1 day under 6 months, 10 days when older. endoflife.date and OSV answers
stay fresh for 1 day, and a 404 for 1 hour. cache.ttl_override sets a fixed TTL per source
(github, endoflife, osv) or host. A stale GET is revalidated with If-None-Match, and an
authenticated 304 from GitHub doesn't count against the rate limit.

--offline answers from the cache only, --refresh revalidates everything, --no-cache skips it.`,
		Args: noArgs,
	}
	cmd.AddCommand(newCacheInfoCmd(d, g), newCacheClearCmd(d, g), newCachePruneCmd(d, g))
	return cmd
}

func newCacheInfoCmd(d *app.Deps, g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: "Show the cache directory, entry count, size and freshness",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := g.cacheForMaintenance(cmd, d)
			if err != nil {
				return err
			}
			defer c.Close()
			info, err := c.Info()
			if err != nil {
				return err //nolint:wrapcheck // httpx errors name the path
			}
			if g.outFormat == render.FormatJSON {
				return render.JSON(d.Stdout, render.NewEnvelope(buildinfo.Get().Version, d.Clock(), info))
			}
			return writeTable(d, [][2]string{
				{"dir", info.Dir},
				{"entries", strconv.Itoa(info.Entries)},
				{"bytes", strconv.FormatInt(info.Bytes, 10)},
				{"fresh", strconv.Itoa(info.Fresh)},
				{"stale", strconv.Itoa(info.Stale)},
			})
		},
	}
}

func newCacheClearCmd(d *app.Deps, g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "clear",
		Short: "Delete every cache entry",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := g.cacheForMaintenance(cmd, d)
			if err != nil {
				return err
			}
			defer c.Close()
			r, err := c.Clear()
			if err != nil {
				return err //nolint:wrapcheck // httpx errors name the path
			}
			return writeRemoved(d, g, r)
		},
	}
}

func newCachePruneCmd(d *app.Deps, g *globals) *cobra.Command {
	var olderThan time.Duration
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Delete entries not used for a while",
		Long: `Deletes entries that no run has used for --older-than (default 720h, 30 days), entries
relgap can't read, entries in an older format and temporary files left by an interrupted run.
Expired entries stay: their ETags let the next run revalidate them for free.`,
		Example: `  relgap cache prune
  relgap cache prune --older-than 168h`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if olderThan <= 0 {
				return usageError(fmt.Errorf("--older-than must be positive, got %s", olderThan))
			}
			c, err := g.cacheForMaintenance(cmd, d)
			if err != nil {
				return err
			}
			defer c.Close()
			r, err := c.Prune(olderThan)
			if err != nil {
				return err //nolint:wrapcheck // httpx errors name the path
			}
			return writeRemoved(d, g, r)
		},
	}
	cmd.Flags().DurationVar(&olderThan, "older-than", defaultPruneAge, "delete entries unused for this long")
	return cmd
}

func writeRemoved(d *app.Deps, g *globals, r httpx.Removed) error {
	if g.outFormat == render.FormatJSON {
		return render.JSON(d.Stdout, render.NewEnvelope(buildinfo.Get().Version, d.Clock(), r))
	}
	return writeTable(d, [][2]string{
		{"dir", r.Dir},
		{"removed", strconv.Itoa(r.Entries)},
		{"bytes", strconv.FormatInt(r.Bytes, 10)},
	})
}

func writeTable(d *app.Deps, rows [][2]string) error {
	tw := tabwriter.NewWriter(d.Stdout, 0, 0, 2, ' ', 0) //nolint:mnd // column padding
	for _, r := range rows {
		_, _ = fmt.Fprintf(tw, "%s\t%s\n", r[0], r[1])
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}

func (g *globals) cacheForMaintenance(cmd *cobra.Command, d *app.Deps) (*httpx.Cache, error) {
	if err := g.outFormat.Check(render.FormatTTY, render.FormatPlain, render.FormatJSON); err != nil {
		return nil, usageError(err)
	}
	if _, err := g.loadConfig(cmd, d); err != nil {
		return nil, err
	}
	return g.newCache(d)
}

// newCache builds the HTTP cache from config and the cache mode flags. Config must be loaded.
func (g *globals) newCache(d *app.Deps) (*httpx.Cache, error) {
	cfg := d.Config.Config.Cache
	dir := cfg.Dir
	if dir == "" {
		base, err := config.CacheDirPath(d.Env.Lookup, runtime.GOOS)
		if err != nil {
			return nil, fmt.Errorf("find cache dir: %w", err)
		}
		dir = base
	}
	overrides, err := parseTTLOverride(cfg.TTLOverride)
	if err != nil {
		return nil, usageError(err)
	}
	mode := httpx.ModeDefault
	switch {
	case g.offline:
		mode = httpx.ModeOffline
	case g.refresh:
		mode = httpx.ModeRefresh
	case g.noCache:
		mode = httpx.ModeNoCache
	}
	return httpx.NewCache(httpx.CacheOptions{
		Dir:         dir,
		Mode:        mode,
		Clock:       d.Clock,
		Logger:      d.Logger,
		TTLOverride: overrides,
		Age:         github.NewestRelease,
	}), nil
}

// enrichmentTTL is how long endoflife.date and OSV answers stay fresh unless the config says
// otherwise. Neither tells relgap how old its data is.
const enrichmentTTL = 24 * time.Hour

// parseTTLOverride turns cache.ttl_override into TTLs by host, on top of the 1-day default for
// endoflife.date and OSV. Keys are source names or hosts.
func parseTTLOverride(in map[string]string) (map[string]time.Duration, error) {
	sources := ttlSources()
	out := map[string]time.Duration{sources["endoflife"]: enrichmentTTL, sources["osv"]: enrichmentTTL}
	for key, val := range in {
		ttl, err := time.ParseDuration(val)
		if err != nil || ttl <= 0 {
			return nil, fmt.Errorf("cache.ttl_override.%s: want a positive Go duration such as 24h, got %q", key, val)
		}
		host, ok := sources[key]
		if !ok && !strings.Contains(key, ".") {
			names := make([]string, 0, len(sources))
			for n := range sources {
				names = append(names, n)
			}
			slices.Sort(names)
			return nil, fmt.Errorf("cache.ttl_override.%s: unknown source; want one of %s or a host name",
				key, strings.Join(names, ", "))
		}
		if !ok {
			host = key
		}
		out[host] = ttl
	}
	return out, nil
}
