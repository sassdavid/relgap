package scan_test

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/scan"
)

func TestGitRemote(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in, host, repo string
		ok             bool
	}{
		{"https://bitbucket.org/ws/repo.git", "bitbucket.org", "ws/repo", true},
		{"git::git@bitbucket.org:ws/repo.git//.", "bitbucket.org", "ws/repo", true},
		{"bitbucket.org/ws/repo", "bitbucket.org", "ws/repo", true},
		{"git::https://gitlab.com/group/sub/repo.git//modules/zone", "gitlab.com", "group/sub/repo", true},
		{"ssh://git@gitlab.internal.test:2222/infra/tf-vpc.git", "gitlab.internal.test", "infra/tf-vpc", true},
		{"git@GitHub.com:o/r.git", "github.com", "o/r", true},
		{"https://user@git.other.test/team/x", "git.other.test", "team/x", true},
		{"https://gitlab.com/repo", "", "", false},
		{"./modules/vpc", "", "", false},
		{"localhost/a/b", "", "", false},
	}
	for _, tt := range tests {
		host, repo, ok := scan.GitRemote(tt.in)
		assert.Equal(t, []any{tt.host, tt.repo, tt.ok}, []any{host, repo, ok}, tt.in)
	}
}

func TestExtractAffixes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		extract, prefix, suffix string
		ok                      bool
	}{
		{"^(?<version>.+)-alpine$", "", "-alpine", true},
		{"^v?(?<version>.+)-al2023$", "", "-al2023", true},
		{"^release-(?<version>.*)$", "release-", "", true},
		{"^v(?<version>.*)", "", "", true},
		{`^(?<version>\d+\.\d+)-slim\-bookworm$`, "", "-slim-bookworm", true},
		{"^(?<version>.+)-alpine", "", "", false},
		{"(?<version>.+)-alpine$", "", "", false},
		{"^(?<version>.+)-alpine.*$", "", "", false},
	}
	for _, tt := range tests {
		prefix, suffix, ok := scan.ExtractAffixes(tt.extract)
		assert.Equal(t, []any{tt.prefix, tt.suffix, tt.ok}, []any{prefix, suffix, ok}, tt.extract)
	}
}

// echo knows every github:, bitbucket: and gitlab: name, as the resolver does.
type echo struct{}

func (echo) Identify(input string) (string, bool) {
	for _, p := range []string{"github:", "bitbucket:", "gitlab:"} {
		if strings.HasPrefix(input, p) {
			return input, true
		}
	}
	return "", false
}

// TestForgeAndImagePins names git sources on Bitbucket, gitlab.com and a configured GitLab host,
// leaves the source on an unlisted host unresolved, reads the Renovate datasources of those hosts,
// and keeps an image's variant in its name.
func TestForgeAndImagePins(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write(t, root, map[string]string{
		"main.tf": `module "a" { source = "git::git@bitbucket.org:ws/net.git//.?ref=2.1.0" }
module "b" { source = "git::https://gitlab.com/g/sub/dns.git?ref=v1.2.0" }
module "c" { source = "git::ssh://git@gitlab.acme.internal/infra/vpc.git?ref=0.4.0" }
module "d" { source = "git::https://git.other.test/team/misc.git?ref=1.0.0" }
`,
		"versions.tf": `locals {
  # renovate: datasource=bitbucket-tags depName=ws/lib extractVersion=^release-(?<version>.+)$
  lib = "1.0.0"
  # renovate: datasource=gitlab-tags depName=infra/net registryUrl=https://gitlab.acme.internal
  net = "2.0.0"
  # renovate: datasource=git-tags depName=https://github.com/o/r.git
  r = "3.0.0"
  # renovate: datasource=gitlab-releases depName=g/app extractVersion=^(?<version>.+)-x$
  app = "4.0.0"
  # renovate: datasource=docker depName=nginx extractVersion=^(?<version>.+)-alpine$
  nginx = "1.27.0-alpine"
  # renovate: datasource=docker depName=redis extractVersion=^(?<version>.+)-alpine.*$
  redis = "7.4.0-alpine"
}
`,
		"Dockerfile": "FROM python:3.14-slim\nFROM ghcr.io/o/app:v2.1.0\nFROM busybox:latest\n",
	})
	s := scan.New(
		slog.New(slog.DiscardHandler),
		noGit{},
		echo{},
		scan.Options{GitLabHosts: []string{"gitlab.acme.internal"}},
	)
	res, err := s.Scan(t.Context(), []string{root})
	require.NoError(t, err)
	got := map[string]string{}
	for _, p := range res.Pins {
		got[p.Location.File+" "+p.Value] = p.Tool + " " + string(p.Match)
	}
	assert.Equal(t, map[string]string{
		"main.tf 2.1.0":             "bitbucket:ws/net exact",
		"main.tf v1.2.0":            "gitlab:g/sub/dns exact",
		"main.tf 0.4.0":             "gitlab:gitlab.acme.internal/infra/vpc exact",
		"main.tf 1.0.0":             " exact",
		"versions.tf 1.0.0":         "bitbucket:ws/lib@release- exact",
		"versions.tf 2.0.0":         "gitlab:gitlab.acme.internal/infra/net exact",
		"versions.tf 3.0.0":         "github:o/r exact",
		"versions.tf 4.0.0":         " exact",
		"versions.tf 1.27.0-alpine": "image:nginx:*-alpine exact",
		"versions.tf 7.4.0-alpine":  " exact",
		"Dockerfile 3.14-slim":      "image:python:*-slim exact",
		"Dockerfile v2.1.0":         "image:ghcr.io/o/app exact",
		"Dockerfile latest":         "image:busybox channel",
	}, got, "a host that isn't configured and an unreadable extractVersion leave the pin unresolved")
}
