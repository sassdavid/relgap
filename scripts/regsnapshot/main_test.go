package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/registry"
)

// aquaRegistry has the version fields of real packages, cut down: lychee, oxlint and oxfmt switch
// prefix in their overrides, kustomize goes back to unprefixed tags, and the last two cover a
// package without overrides and one whose own fields apply to no version.
const aquaRegistry = `packages:
  - repo_owner: lycheeverse
    repo_name: lychee
    version_filter: not (Version startsWith "lychee-lib-")
    version_constraint: "false"
    version_overrides:
      - version_constraint: semver("<= 0.15.1")
      - version_constraint: semver("< 0.24.0")
        version_prefix: lychee-v
      - version_constraint: "true"
        version_prefix: lychee-v
  - name: oxc-project/oxc/oxlint
    repo_owner: oxc-project
    repo_name: oxc
    version_filter: 'Version matches "^apps_v" || Version matches "^oxlint_v"'
    version_constraint: "false"
    version_overrides:
      - version_constraint: Version in ["apps_v1.47.0", "oxlint_v0.1.1"]
      - version_constraint: semver("< 1.17.0")
        version_prefix: oxlint_v
      - version_constraint: semver("< 1.85.0")
        version_prefix: apps_v
      - version_constraint: "true"
        version_prefix: oxlint_v
  - name: oxc-project/oxc/oxfmt
    repo_owner: oxc-project
    repo_name: oxc
    version_prefix: oxfmt_v
    version_filter: Version matches "^oxfmt_v"
    version_constraint: "false"
    version_overrides:
      - version_constraint: semver("<= 1.84.0")
        version_prefix: apps_v
      - version_constraint: "true"
  - repo_owner: kubernetes-sigs
    repo_name: kustomize
    version_prefix: kustomize/
    version_overrides:
      - version_constraint: semver("< 3.0.0")
        version_prefix: ""
  - repo_owner: hashicorp
    repo_name: terraform
    version_prefix: v
  - repo_owner: example
    repo_name: switched
    version_constraint: "false"
    version_overrides:
      - version_constraint: semver("< 2.0.0")
        version_prefix: old-
      - version_constraint: "true"
        version_prefix: new-
`

func TestWriteAquaVersionPrefixes(t *testing.T) {
	t.Parallel()
	in, out := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(in, "registry.yaml"), []byte(aquaRegistry), 0o600))
	meta := registry.SnapshotMeta{Source: "test", Commit: "abc", CommitDate: "2026-11-09T16:23:13Z"}
	require.NoError(t, writeAqua(in, meta, out))

	b, err := os.ReadFile(filepath.Join(out, registry.AquaSnapshotFile))
	require.NoError(t, err)
	var f struct {
		Entries []registry.AquaEntry `json:"entries"`
	}
	require.NoError(t, json.Unmarshal(b, &f))
	got := map[string]registry.AquaEntry{}
	for _, e := range f.Entries {
		got[e.Name] = e
	}
	tests := []struct {
		name     string
		prefix   string
		prefixes []string
	}{
		{name: "lycheeverse/lychee", prefixes: []string{"lychee-v", ""}},
		{name: "oxc-project/oxc/oxlint", prefixes: []string{"oxlint_v", "apps_v", ""}},
		{name: "oxc-project/oxc/oxfmt", prefix: "oxfmt_v", prefixes: []string{"oxfmt_v", "apps_v"}},
		{name: "kubernetes-sigs/kustomize", prefix: "kustomize/", prefixes: []string{"kustomize/", ""}},
		{name: "hashicorp/terraform", prefix: "v"},
		{name: "example/switched", prefixes: []string{"new-", "old-"}},
	}
	for _, tt := range tests {
		e, ok := got[tt.name]
		require.True(t, ok, tt.name)
		assert.Equal(t, tt.prefix, e.VersionPrefix, tt.name)
		assert.Equal(t, tt.prefixes, e.VersionPrefixes, tt.name)
	}
	assert.Equal(t, `not (Version startsWith "lychee-lib-")`, got["lycheeverse/lychee"].VersionFilter)
}
