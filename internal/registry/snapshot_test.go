package registry_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/registry"
)

func TestSnapshotMeta_CheckAge(t *testing.T) {
	t.Parallel()
	meta := registry.SnapshotMeta{
		Source:     "https://github.com/jdx/mise",
		Commit:     "abc",
		CommitDate: "2026-11-09T10:34:37-06:00",
	}
	commit := time.Date(2026, 11, 9, 16, 34, 37, 0, time.UTC)
	tests := []struct {
		name    string
		meta    registry.SnapshotMeta
		now     time.Time
		wantErr string
	}{
		{"fresh", meta, commit.Add(7 * 24 * time.Hour), ""},
		{"at the limit", meta, commit.Add(registry.MaxSnapshotAge), ""},
		{
			"stale",
			meta,
			commit.Add(registry.MaxSnapshotAge + time.Second),
			"snapshot is stale: https://github.com/jdx/mise at abc is from 2026-11-09T10:34:37-06:00, 30 days old, over the 30-day limit",
		},
		{"bad date", registry.SnapshotMeta{Source: "x", CommitDate: "2026-11-09"}, commit, "parse commit_date of x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.meta.CheckAge(tt.now, registry.MaxSnapshotAge)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

// The embedded snapshots carry a commit date the registry workflow can read.
func TestSnapshotMeta_embedded(t *testing.T) {
	t.Parallel()
	mise, _, err := registry.MiseSnapshot()
	require.NoError(t, err)
	aqua, _, err := registry.AquaSnapshot()
	require.NoError(t, err)
	for _, m := range []registry.SnapshotMeta{mise, aqua} {
		date, parseErr := time.Parse(time.RFC3339, m.CommitDate)
		require.NoError(t, parseErr, m.Source)
		assert.NoError(t, m.CheckAge(date, registry.MaxSnapshotAge), m.Source)
	}
}
