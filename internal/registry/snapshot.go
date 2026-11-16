package registry

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/sassdavid/relgap/internal/misex"
)

// Snapshot file names in internal/registry/snapshot.
const (
	MiseSnapshotFile = "mise.json"
	AquaSnapshotFile = "aqua.json"
)

// MaxSnapshotAge is how old a snapshot's commit may be before the registry workflow fails.
const MaxSnapshotAge = 30 * day

// ErrSnapshotStale reports a snapshot whose commit is older than the allowed age.
var ErrSnapshotStale = errors.New("snapshot is stale")

//go:embed snapshot/*.json
var snapshots embed.FS

// SnapshotMeta records where a snapshot came from.
type SnapshotMeta struct {
	Source     string `json:"source"`
	Commit     string `json:"commit"`
	CommitDate string `json:"commit_date"` // RFC 3339
}

// Age returns how long before now the snapshot's commit was made.
func (m SnapshotMeta) Age(now time.Time) (time.Duration, error) {
	t, err := time.Parse(time.RFC3339, m.CommitDate)
	if err != nil {
		return 0, fmt.Errorf("parse commit_date of %s: %w", m.Source, err)
	}
	return now.Sub(t), nil
}

// CheckAge returns [ErrSnapshotStale] when the snapshot's commit is older than maxAge at now.
func (m SnapshotMeta) CheckAge(now time.Time, maxAge time.Duration) error {
	age, err := m.Age(now)
	if err != nil {
		return err
	}
	if age > maxAge {
		return fmt.Errorf("%w: %s at %s is from %s, %d days old, over the %d-day limit",
			ErrSnapshotStale, m.Source, m.Commit, m.CommitDate, days(age), days(maxAge))
	}
	return nil
}

const day = 24 * time.Hour

func days(d time.Duration) int { return int(d / day) }

// AquaEntry is one package of the aqua registry, reduced to what resolution needs.
type AquaEntry struct {
	Name          string   `json:"name"`           // package name, such as hashicorp/terraform
	Repo          string   `json:"repo,omitempty"` // empty for packages not hosted on GitHub
	Aliases       []string `json:"aliases,omitempty"`
	VersionPrefix string   `json:"version_prefix,omitempty"`
	VersionFilter string   `json:"version_filter,omitempty"` // an expr-lang expression
	VersionSource string   `json:"version_source,omitempty"` // github_tag or github_release
}

type snapshotFile[E any] struct {
	SnapshotMeta

	Entries []E `json:"entries"`
}

func readSnapshot[E any](name string) (SnapshotMeta, []E, error) {
	b, err := snapshots.ReadFile("snapshot/" + name)
	if err != nil {
		return SnapshotMeta{}, nil, fmt.Errorf("read snapshot %s: %w", name, err)
	}
	var f snapshotFile[E]
	if err = json.Unmarshal(b, &f); err != nil {
		return SnapshotMeta{}, nil, fmt.Errorf("decode snapshot %s: %w", name, err)
	}
	return f.SnapshotMeta, f.Entries, nil
}

// MiseSnapshot returns the embedded copy of mise's registry.
func MiseSnapshot() (SnapshotMeta, []misex.RegistryEntry, error) {
	return readSnapshot[misex.RegistryEntry](MiseSnapshotFile)
}

// AquaSnapshot returns the embedded index of the aqua registry.
func AquaSnapshot() (SnapshotMeta, []AquaEntry, error) {
	return readSnapshot[AquaEntry](AquaSnapshotFile)
}
