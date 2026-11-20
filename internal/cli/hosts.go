package cli

import (
	"cmp"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"

	"github.com/sassdavid/relgap/internal/app"
	"github.com/sassdavid/relgap/internal/bitbucket"
	"github.com/sassdavid/relgap/internal/config"
	"github.com/sassdavid/relgap/internal/forge"
	"github.com/sassdavid/relgap/internal/gitlab"
	"github.com/sassdavid/relgap/internal/oci"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/report"
)

// GitLab token variables, read the way glab reads them.
const (
	envGitLabToken       = "GITLAB_TOKEN"
	envGitLabAccessToken = "GITLAB_ACCESS_TOKEN"
	envGitLabHost        = "GITLAB_HOST"
	envCIJobToken        = "CI_JOB_TOKEN" //nolint:gosec // an environment variable name, not a credential
	envCIServerHost      = "CI_SERVER_HOST"
)

// hostClients builds the Bitbucket Cloud and GitLab clients on first use, each with the token of
// its own host only.
type hostClients struct {
	ctx context.Context //nolint:containedctx // the logs of a client built on first use need the session's context
	d   *app.Deps
	hc  *http.Client

	bbOnce sync.Once
	bb     *bitbucket.Client

	mu sync.Mutex
	gl map[string]*gitlab.Client
}

func newHostClients(ctx context.Context, d *app.Deps, hc *http.Client) *hostClients {
	return &hostClients{ctx: ctx, d: d, hc: hc, gl: map[string]*gitlab.Client{}}
}

// client returns the client of host: bitbucket.org, gitlab.com or a host in gitlab.hosts.
func (h *hostClients) client(host string) (report.GitHub, error) {
	host = strings.ToLower(host)
	if forge.KindOf(host) == forge.KindBitbucket {
		h.bbOnce.Do(func() {
			token, _ := h.d.Env.Lookup(bitbucket.TokenEnv)
			token = strings.TrimSpace(token)
			if token == "" {
				h.d.Logger.WarnContext(h.ctx, "no Bitbucket token found; anonymous requests are limited to 60 an hour "+
					"per IP. Set "+bitbucket.TokenEnv)
			}
			h.d.Logger.DebugContext(h.ctx, "bitbucket client", "authenticated", token != "")
			h.bb = bitbucket.New(h.hc, bitbucket.Options{Token: token})
		})
		return h.bb, nil
	}
	cfg := h.d.Config.Config.GitLab
	if host != forge.GitLab && !slices.ContainsFunc(cfg.Hosts, func(g config.GitLabHost) bool {
		return strings.EqualFold(g.Host, host)
	}) {
		return nil, fmt.Errorf("%s is not a GitLab host relgap knows; add it to gitlab.hosts", host)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if c, ok := h.gl[host]; ok {
		return c, nil
	}
	token, header, source := gitlabToken(h.d.Env.Lookup, cfg, host)
	h.d.Logger.DebugContext(h.ctx, "gitlab client", "host", host, "authenticated", token != "", "token_source", source)
	c := gitlab.New(h.hc, gitlab.Options{Host: host, Token: token, TokenHeader: header})
	h.gl[host] = c
	return c, nil
}

// gitlabToken returns the token for a GitLab host, its header and the variable it came from: a
// gitlab.hosts entry's token_env, then GITLAB_TOKEN or GITLAB_ACCESS_TOKEN when GITLAB_HOST
// (default gitlab.com) is this host, then CI_JOB_TOKEN when CI_SERVER_HOST is. A token never goes
// to another host.
func gitlabToken(lookup func(string) (string, bool), cfg config.GitLab, host string) (string, string, string) {
	get := func(name string) string {
		v, _ := lookup(name)
		return strings.TrimSpace(v)
	}
	for _, g := range cfg.Hosts {
		if strings.EqualFold(g.Host, host) && g.TokenEnv != "" {
			if v := get(g.TokenEnv); v != "" {
				return v, gitlab.HeaderPrivateToken, g.TokenEnv
			}
		}
	}
	if strings.EqualFold(hostOf(cmp.Or(get(envGitLabHost), forge.GitLab)), host) {
		for _, name := range []string{envGitLabToken, envGitLabAccessToken} {
			if v := get(name); v != "" {
				return v, gitlab.HeaderPrivateToken, name
			}
		}
	}
	if ci := get(envCIServerHost); ci != "" && strings.EqualFold(hostOf(ci), host) {
		if v := get(envCIJobToken); v != "" {
			return v, gitlab.HeaderJobToken, envCIJobToken
		}
	}
	return "", "", ""
}

// hostOf returns the host of a GITLAB_HOST value, which glab accepts with or without a scheme.
func hostOf(v string) string {
	if u, err := url.Parse(v); err == nil && u.Host != "" {
		return u.Host
	}
	return strings.TrimRight(v, "/")
}

// imageClient returns the image registry client for the configured registries.
func imageClient(d *app.Deps, hc *http.Client) *oci.Client {
	return oci.New(hc, oci.Options{Registries: append(oci.DefaultRegistries(), d.Config.Config.Images.Registries...)})
}

// configLayer is the part of the config that takes part in resolution.
func configLayer(c config.Config) registry.ConfigLayer {
	layer := registry.ConfigLayer{Tools: map[string]string{}, Hosts: map[string]string{}, Aliases: c.Aliases}
	for tool, t := range c.Tools {
		layer.Tools[tool] = t.Repo
		if t.Host != "" {
			layer.Hosts[tool] = t.Host
		}
	}
	for _, g := range c.GitLab.Hosts {
		layer.GitLabHosts = append(layer.GitLabHosts, strings.ToLower(g.Host))
	}
	return layer
}

// gitLabHosts lists the hosts of gitlab.hosts.
func gitLabHosts(c config.Config) []string {
	return configLayer(c).GitLabHosts
}
