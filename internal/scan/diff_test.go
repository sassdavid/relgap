package scan_test

import (
	"archive/tar"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/execx"
	"github.com/sassdavid/relgap/internal/scan"
)

func pin(file string, line int, tool, value string) scan.Pin {
	p := scan.Pin{
		Tool: tool, RawName: tool, Kind: scan.KindTool, Value: value, Version: value,
		Match: scan.MatchExact, Exact: true,
		Location: scan.Location{Repo: ".", File: file, Line: line},
	}
	if value != "" && value[0] == '~' {
		p.Match, p.Exact = scan.MatchConstraint, false
	}
	return p
}

// changeView is a [scan.Change] reduced to values, so tables can compare it.
type changeView struct {
	Base, Head string
	Upgrade    bool
}

func view(cs []scan.Change) []changeView {
	out := make([]changeView, len(cs))
	for i, c := range cs {
		if c.Base != nil {
			out[i].Base = c.Base.Tool + " " + c.Base.Value
		}
		if c.Head != nil {
			out[i].Head = c.Head.Tool + " " + c.Head.Value
		}
		out[i].Upgrade = c.Upgrade()
	}
	return out
}

func TestDiff(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		base, head []scan.Pin
		want       []changeView
	}{
		{
			name: "unchanged pins drop out even when lines move",
			base: []scan.Pin{pin("mise.toml", 2, "helm", "3.21.0")},
			head: []scan.Pin{pin("mise.toml", 5, "helm", "3.21.0")},
			want: []changeView{},
		},
		{
			name: "an exact version change is an upgrade",
			base: []scan.Pin{pin("mise.toml", 2, "kubectl", "1.36.0")},
			head: []scan.Pin{pin("mise.toml", 2, "kubectl", "1.37.2")},
			want: []changeView{{Base: "kubectl 1.36.0", Head: "kubectl 1.37.2", Upgrade: true}},
		},
		{
			name: "added, removed and constraint pins aren't upgrades",
			base: []scan.Pin{
				pin("mise.toml", 2, "gh", "2.80.0"),
				pin("versions.tf", 5, "tfprovider:hashicorp/aws", "~> 6.0"),
			},
			head: []scan.Pin{
				pin("mise.toml", 2, "terraform", "1.15.8"),
				pin("versions.tf", 5, "tfprovider:hashicorp/aws", "~> 6.1"),
			},
			want: []changeView{
				{Base: "gh 2.80.0"},
				{Head: "terraform 1.15.8"},
				{Base: "tfprovider:hashicorp/aws ~> 6.0", Head: "tfprovider:hashicorp/aws ~> 6.1"},
			},
		},
		{
			name: "the same tool in another file pairs separately",
			base: []scan.Pin{pin("Dockerfile", 2, "kubectl", "1.36.0"), pin("mise.toml", 2, "kubectl", "1.36.0")},
			head: []scan.Pin{pin("Dockerfile", 2, "kubectl", "1.36.0"), pin("mise.toml", 2, "kubectl", "1.37.2")},
			want: []changeView{{Base: "kubectl 1.36.0", Head: "kubectl 1.37.2", Upgrade: true}},
		},
		{
			name: "several changes of one key pair in line order",
			base: []scan.Pin{
				pin("ci.yml", 3, "node", "20.1.0"),
				pin("ci.yml", 9, "node", "22.1.0"),
				pin("ci.yml", 12, "node", "24.0.0"),
			},
			head: []scan.Pin{
				pin("ci.yml", 3, "node", "20.2.0"),
				pin("ci.yml", 9, "node", "22.2.0"),
				pin("ci.yml", 12, "node", "24.0.0"),
			},
			want: []changeView{
				{Base: "node 20.1.0", Head: "node 20.2.0", Upgrade: true},
				{Base: "node 22.1.0", Head: "node 22.2.0", Upgrade: true},
			},
		},
		{
			name: "uneven changes of one key count as removed and added",
			base: []scan.Pin{pin("ci.yml", 3, "node", "20.1.0")},
			head: []scan.Pin{pin("ci.yml", 3, "node", "20.2.0"), pin("ci.yml", 9, "node", "22.2.0")},
			want: []changeView{{Base: "node 20.1.0"}, {Head: "node 20.2.0"}, {Head: "node 22.2.0"}},
		},
		{
			name: "an unresolved pin is never an upgrade",
			base: []scan.Pin{pin("Makefile", 1, "", "1.0.0")},
			head: []scan.Pin{pin("Makefile", 1, "", "1.1.0")},
			want: []changeView{{Base: " 1.0.0", Head: " 1.1.0"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, view(scan.Diff(tt.base, tt.head)))
		})
	}
}

func TestRenovateTopic(t *testing.T) {
	t.Parallel()
	tests := []struct {
		branch, topic string
		prefixed      bool
	}{
		{"renovate/terraform-1.x", "terraform-1.x", true},
		{"deps/hashicorp-aws-6.x", "hashicorp-aws-6.x", false},
		{"main", "main", false},
	}
	for _, tt := range tests {
		topic, prefixed := scan.RenovateTopic(tt.branch)
		assert.Equal(t, tt.topic, topic, tt.branch)
		assert.Equal(t, tt.prefixed, prefixed, tt.branch)
	}
}

func TestRenovateTopicMatches(t *testing.T) {
	t.Parallel()
	tests := []struct {
		topic string
		names []string
		want  bool
	}{
		{"terraform-1.x", []string{"terraform"}, true},
		{"kubectl-1.37.x", []string{"kubectl"}, true},
		{"hashicorp-aws-6.x", []string{"tfprovider:hashicorp/aws", "hashicorp/aws"}, true},
		{"gruntwork-io-terragrunt-1.x", []string{"terragrunt", "TG_VERSION", "gruntwork-io/terragrunt"}, true},
		{"node-digest", []string{"node"}, true},
		{"golangci-lint-2.x", []string{"go"}, false},
		{"all-minor-patch", []string{"terraform", "kubectl"}, false},
		{"Terraform-1.x", []string{"TERRAFORM"}, true},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, scan.RenovateTopicMatches(tt.topic, tt.names...), "%s %v", tt.topic, tt.names)
	}
}

func TestRenovateTargetBranch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, body, want string
		found            bool
	}{
		{
			name:  "debug comment",
			body:  "updates\n<!--renovate-debug:eyJjcmVhdGVkSW5WZXIiOiI0My4xMC4yIiwidGFyZ2V0QnJhbmNoIjoicmVsZWFzZS8xLngifQ==-->\n",
			want:  "release/1.x",
			found: true,
		},
		{name: "not base64", body: "<!--renovate-debug:%%%-->", found: false},
		{name: "not JSON", body: "<!--renovate-debug:bm90IGpzb24=-->", found: true},
		{name: "no comment", body: "Bumps kubectl from 1.36.0 to 1.37.2.", found: false},
	}
	for _, tt := range tests {
		got, found := scan.RenovateTargetBranch(tt.body)
		assert.Equal(t, tt.want, got, tt.name)
		assert.Equal(t, tt.found, found, tt.name)
	}
}

// tarGit answers `git archive` with a fixed tar stream.
type tarGit []byte

func (g tarGit) Run(_ context.Context, c execx.Cmd) (*execx.Result, error) {
	_, err := c.Stdout.Write(g)
	return &execx.Result{}, err
}

func tarOf(t *testing.T, entries ...tar.Header) tarGit {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, h := range entries {
		body := []byte("content of " + h.Name)
		if h.Typeflag == tar.TypeReg {
			h.Size = int64(len(body))
		}
		require.NoError(t, tw.WriteHeader(&h))
		if h.Typeflag == tar.TypeReg {
			_, err := tw.Write(body)
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	return buf.Bytes()
}

func TestExtractRef(t *testing.T) {
	t.Parallel()
	dst := t.TempDir()
	git := tarOf(
		t,
		tar.Header{
			Typeflag:   tar.TypeXGlobalHeader,
			Name:       "pax_global_header",
			PAXRecords: map[string]string{"comment": "abc"},
		},
		tar.Header{Typeflag: tar.TypeDir, Name: "infra/", Mode: 0o755},
		tar.Header{Typeflag: tar.TypeReg, Name: "infra/mise.toml", Mode: 0o644},
		tar.Header{Typeflag: tar.TypeReg, Name: "deep/a/b/Dockerfile", Mode: 0o644},
		tar.Header{Typeflag: tar.TypeSymlink, Name: "link", Linkname: "/etc/passwd"},
	)
	require.NoError(t, scan.ExtractRef(t.Context(), git, ".", "abc", dst))
	got, err := os.ReadFile(filepath.Join(dst, "infra", "mise.toml"))
	require.NoError(t, err)
	assert.Equal(t, "content of infra/mise.toml", string(got))
	assert.FileExists(t, filepath.Join(dst, "deep", "a", "b", "Dockerfile"))
	assert.NoFileExists(t, filepath.Join(dst, "link"))

	escape := tarOf(t, tar.Header{Typeflag: tar.TypeReg, Name: "../escape.txt", Mode: 0o644})
	require.Error(t, scan.ExtractRef(t.Context(), escape, ".", "abc", t.TempDir()))
}

func TestResolveRefRejectsOptions(t *testing.T) {
	t.Parallel()
	_, err := scan.ResolveRef(t.Context(), noGit{}, ".", "--output=/tmp/x")
	require.ErrorIs(t, err, scan.ErrRefNotFound)
}
