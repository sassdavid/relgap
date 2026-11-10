package github

import (
	"context"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/config"

	"github.com/sassdavid/relgap/internal/execx"
)

// DefaultHost is the GitHub host whose token relgap looks up.
const DefaultHost = "github.com"

const ghTokenTimeout = 10 * time.Second

// Token sources reported by [Token].
const (
	SourceGHToken     = "GH_TOKEN"
	SourceGitHubToken = "GITHUB_TOKEN" //nolint:gosec // an environment variable name, not a credential
	SourceGHConfig    = "gh config"
	SourceGHKeyring   = "gh auth token"
)

// Runner starts a command; [execx.Runner] implements it.
type Runner interface {
	Run(ctx context.Context, c execx.Cmd) (*execx.Result, error)
}

// TokenOptions says where [Token] looks.
type TokenOptions struct {
	Host        string                      // default [DefaultHost]
	Lookup      func(string) (string, bool) // environment
	ConfigToken func(host string) string    // nil skips gh's config file
	Runner      Runner                      // runs `gh auth token`; nil skips it
}

// Token finds a GitHub token the way gh does: GH_TOKEN, GITHUB_TOKEN, gh's config file, then
// gh's keyring through `gh auth token`. It returns "" when there is none, and the source otherwise.
func Token(ctx context.Context, o TokenOptions) (string, string) {
	host := o.Host
	if host == "" {
		host = DefaultHost
	}
	for _, key := range []string{SourceGHToken, SourceGitHubToken} {
		if v, ok := o.Lookup(key); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v), key
		}
	}
	if o.ConfigToken != nil {
		if v := o.ConfigToken(host); v != "" {
			return v, SourceGHConfig
		}
	}
	if o.Runner != nil {
		res, err := o.Runner.Run(ctx, execx.Cmd{
			Name:    "gh",
			Args:    []string{"auth", "token", "--secure-storage", "--hostname", host},
			Timeout: ghTokenTimeout,
		})
		if err == nil {
			if v := strings.TrimSpace(string(res.Stdout)); v != "" {
				return v, SourceGHKeyring
			}
		}
	}
	return "", ""
}

// GHConfigToken returns the oauth_token for host in gh's hosts.yml, or "".
func GHConfigToken(host string) string {
	cfg, err := config.Read(nil)
	if err != nil {
		return ""
	}
	v, err := cfg.Get([]string{"hosts", host, "oauth_token"})
	if err != nil {
		return ""
	}
	return v
}
