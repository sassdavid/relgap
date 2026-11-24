package enrich_test

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/enrich"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/version"
)

// versionsOf returns a lister for the tool's versions, given without the tag prefix.
func versionsOf(t *testing.T, tool registry.ToolSpec, vs string) enrich.VersionLister {
	t.Helper()
	rules, err := tool.Version.Rules()
	require.NoError(t, err)
	refs := make([]version.Ref, 0)
	for v := range strings.FieldsSeq(vs) {
		refs = append(refs, version.Ref{Tag: tool.Version.TagPrefixes.Newest() + v})
	}
	return func(ctx context.Context) (*version.Universe, error) {
		return version.NewUniverse(ctx, slog.New(slog.DiscardHandler), refs, rules), nil
	}
}

// TestSpec_vulnerability_audit_5: the fix targets of terraform, two npm packages and go match a
// hand-checked list. lodash's first candidate has a record of its own, request's latest release
// is still affected, and one of request's advisories has no fix. Every version below comes from
// osv.dev and the release lists checked on 2026-11-23.
func TestSpec_vulnerability_audit_5(t *testing.T) {
	t.Parallel()
	terraform := repoTool("terraform", "", "hashicorp/terraform", "v")
	terraform.OSV = []registry.OSVPackage{{Ecosystem: "Go", Name: "github.com/hashicorp/terraform"}}
	request := backendTool("npm:request", version.SchemeSemver)
	request.Version.TagPrefixes = registry.Prefixes{"v"}
	goTool := toolOf("go", version.SchemeLoose, "go")
	goTool.OSV = []registry.OSVPackage{{Ecosystem: "Go", Name: "stdlib"}, {Ecosystem: "Go", Name: "toolchain"}}
	tests := []struct {
		cassette string
		tool     registry.ToolSpec
		current  string
		versions string // the tool's releases from the current line on, as relgap lists them
		want     enrich.Fix
	}{
		{
			// GHSA-h626-pv66-hhm7 (GO-2023-2055) is fixed in 1.5.7. OSV lists nothing for 1.5.7 or
			// 1.17.2. Versions: the GitHub releases of hashicorp/terraform without the alphas.
			cassette: "terraform", tool: terraform, current: "1.5.0",
			versions: `
				1.5.0-beta1 1.5.0-beta2 1.5.0-rc1 1.5.0-rc2 1.5.0 1.5.1 1.5.2 1.5.3 1.5.4 1.5.5
				1.5.6 1.5.7 1.6.0-beta1 1.6.0-beta2 1.6.0-beta3 1.6.0-rc1 1.6.0 1.6.1 1.6.2
				1.6.3 1.6.4 1.6.5 1.6.6 1.7.0-beta1 1.7.0-beta2 1.7.0-rc1 1.7.0-rc2 1.7.0 1.7.1
				1.7.2 1.7.3 1.7.4 1.7.5 1.8.0-beta1 1.8.0-rc1 1.8.0-rc2 1.8.0 1.8.1 1.8.2 1.8.3
				1.8.4 1.8.5 1.9.0-beta1 1.9.0-rc1 1.9.0-rc2 1.9.0-rc3 1.9.0 1.9.1 1.9.2 1.9.3
				1.9.4 1.9.5 1.9.6 1.9.7 1.9.8 1.10.0-beta1 1.10.0-rc1 1.10.0-rc2 1.10.0-rc3
				1.10.0 1.10.1 1.10.2 1.10.3 1.10.4 1.10.5 1.11.0-beta1 1.11.0-beta2 1.11.0-rc1
				1.11.0-rc2 1.11.0-rc3 1.11.0 1.11.1 1.11.2 1.11.3 1.11.4 1.12.0-beta1
				1.12.0-beta2 1.12.0-beta3 1.12.0-rc1 1.12.0-rc2 1.12.0 1.12.1 1.12.2
				1.13.0-beta1 1.13.0-beta2 1.13.0-beta3 1.13.0-rc1 1.13.0 1.13.1 1.13.2 1.13.3
				1.13.4 1.13.5 1.14.0-beta1 1.14.0-beta2 1.14.0-beta3 1.14.0-rc1 1.14.0-rc2
				1.14.0 1.14.1 1.14.2 1.14.3 1.14.4 1.14.5 1.14.6 1.14.7 1.14.8 1.14.9
				1.15.0-beta1 1.15.0-beta2 1.15.0-rc1 1.15.0-rc2 1.15.0-rc3 1.15.0-rc4 1.15.0
				1.15.1 1.15.2 1.15.3 1.15.4 1.15.5 1.15.6 1.15.7 1.15.8 1.15.9 1.16.0-beta1
				1.16.0-beta2 1.16.0-rc1 1.16.0-rc2 1.16.0 1.16.1 1.16.2 1.16.3 1.16.4 1.16.5
				1.16.6 1.16.7 1.17.0-beta1 1.17.0-beta2 1.17.0-rc1 1.17.0-rc2 1.17.0 1.17.1
				1.17.2`,
			want: enrich.Fix{Lowest: "1.5.7", LowestInLine: "1.5.7", Latest: "1.17.2"},
		},
		{
			// 4.17.20 has GHSA-29mw-wpgm-hmr9 and GHSA-35jh-r3h4-6jhm (fixed 4.17.21),
			// GHSA-xxjr-mmjv-4gpg (fixed 4.17.23) and GHSA-r5fr-rjxr-66jc (fixed 4.18.0), so 4.18.0
			// is the first release past all four. 4.18.0 has a record of its own,
			// GHSA-f23m-r3pf-42rh (introduced 4.17.23, fixed 4.18.1), so the target moves to 4.18.1,
			// which OSV lists nothing for. Versions: the GitHub releases of lodash/lodash, which
			// npm:lodash lists.
			cassette: "lodash", tool: backendTool("npm:lodash", version.SchemeSemver), current: "4.17.20",
			versions: "3.0.0 4.0.0 4.17.20 4.18.0 4.18.1",
			want:     enrich.Fix{Lowest: "4.18.1", LowestInLine: "4.18.1", Latest: "4.18.1"},
		},
		{
			// GHSA-7xfp-9c55-5vqj is fixed in 2.68.0. GHSA-p8p7-x288-28g6 has no fixed version
			// (last_affected 2.88.2), so the latest tag, v2.88.1, still has it. Versions: the tags
			// of request/request, which has no GitHub releases.
			cassette: "request", tool: request, current: "2.67.0",
			versions: `
				2.67.0 2.67.1 2.68.0 2.68.1 2.69.0 2.69.1 2.70.0 2.70.1 2.71.0 2.71.1 2.72.0
				2.72.1 2.73.0 2.73.1 2.74.0 2.74.1 2.75.0 2.75.1 2.76.0 2.76.1 2.77.0 2.77.1
				2.78.0 2.78.1 2.79.0 2.79.1 2.80.0 2.80.1 2.81.0 2.81.1 2.82.0 2.82.1 2.83.0
				2.83.1 2.84.0 2.84.1 2.85.0 2.85.1 2.86.0 2.86.1 2.87.0 2.87.1 2.88.0 2.88.1`,
			want: enrich.Fix{
				Lowest: "2.68.0", LowestInLine: "2.68.0", Latest: "2.88.1", LatestAffected: true,
				LatestAffectedBy: []string{"GHSA-p8p7-x288-28g6"}, Unfixed: []string{"GHSA-p8p7-x288-28g6"},
			},
		},
		{
			// Of the stable Go releases from 1.22.0 to 1.27.4 (go.dev/dl/?mode=json&include=all),
			// only 1.26.11 and 1.27.4 have no stdlib and no toolchain record (one querybatch for
			// every stable release). The rc tags are never candidates.
			cassette: "go", tool: goTool, current: "1.22.0",
			versions: `
				1.22rc1 1.22rc2 1.22.0 1.22.1 1.22.2 1.22.3 1.22.4 1.22.5 1.22.6 1.22.7 1.22.8
				1.22.9 1.22.10 1.22.11 1.22.12 1.23rc1 1.23rc2 1.23.0 1.23.1 1.23.2 1.23.3
				1.23.4 1.23.5 1.23.6 1.23.7 1.23.8 1.23.9 1.23.10 1.23.11 1.23.12 1.24rc1
				1.24rc2 1.24rc3 1.24.0 1.24.1 1.24.2 1.24.3 1.24.4 1.24.5 1.24.6 1.24.7 1.24.8
				1.24.9 1.24.10 1.24.11 1.24.12 1.24.13 1.25rc1 1.25rc2 1.25rc3 1.25.0 1.25.1
				1.25.2 1.25.3 1.25.4 1.25.5 1.25.6 1.25.7 1.25.8 1.25.9 1.25.10 1.25.11 1.25.12
				1.25.13 1.25.14 1.26rc1 1.26rc2 1.26rc3 1.26.0 1.26.1 1.26.2 1.26.3 1.26.4
				1.26.5 1.26.6 1.26.7 1.26.8 1.26.9 1.26.10 1.26.11 1.27rc1 1.27rc2 1.27rc3
				1.27.0 1.27.1 1.27.2 1.27.3 1.27.4`,
			want: enrich.Fix{Lowest: "1.26.11", LowestInLine: "1.26.11", Latest: "1.27.4"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.cassette, func(t *testing.T) {
			t.Parallel()
			_, osv := clients(t, "fix-"+tt.cassette)
			rules, err := tt.tool.Version.Rules()
			require.NoError(t, err)
			current, err := rules.Scheme.Parse(tt.current)
			require.NoError(t, err)
			res, problems, err := enrich.NewEnricher(slog.New(slog.DiscardHandler), nil, osv, 2).
				Enrich(t.Context(), []enrich.Request{{
					Key: "k", Name: tt.tool.Name, Tool: tt.tool, Current: current,
					Versions: versionsOf(t, tt.tool, tt.versions),
				}})
			require.NoError(t, err)
			require.Empty(t, problems)
			require.NotNil(t, res["k"].Fix)
			assert.Equal(t, tt.want, *res["k"].Fix)
		})
	}
}

// TestFixTargetEdges: a current version that is the newest release has nothing to move to, and a
// failed version list gives no target.
func TestFixTargetEdges(t *testing.T) {
	t.Parallel()
	lodash := backendTool("npm:lodash", version.SchemeSemver)
	advisories := []string{"GHSA-29mw-wpgm-hmr9", "GHSA-35jh-r3h4-6jhm", "GHSA-r5fr-rjxr-66jc", "GHSA-xxjr-mmjv-4gpg"}
	failing := func(context.Context) (*version.Universe, error) { return nil, errors.New("down") }
	tests := []struct {
		name     string // also names the cassette
		versions enrich.VersionLister
		want     *enrich.Fix
	}{
		{
			name:     "newest",
			versions: versionsOf(t, lodash, "4.17.19 4.17.20"),
			want: &enrich.Fix{
				Latest:           "4.17.20",
				LatestAffected:   true,
				LatestAffectedBy: advisories,
				Unfixed:          advisories,
			},
		},
		{name: "failed", versions: failing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, osv := clients(t, "fix-"+tt.name)
			res, problems, err := enrich.NewEnricher(slog.New(slog.DiscardHandler), nil, osv, 2).
				Enrich(t.Context(), []enrich.Request{{
					Key: "k", Name: lodash.Name, Tool: lodash, Current: semver(t, "4.17.20"), Versions: tt.versions,
				}})
			require.NoError(t, err)
			assert.Empty(t, problems)
			assert.Equal(t, tt.want, res["k"].Fix)
		})
	}
}
