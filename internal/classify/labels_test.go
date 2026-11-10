package classify_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v4"

	"github.com/sassdavid/relgap/internal/classify"
	"github.com/sassdavid/relgap/internal/registry"
	registryfiles "github.com/sassdavid/relgap/registry"
)

const (
	labelTools      = 15
	labelEntries    = 300
	minPrecision    = 0.9
	labelConfidence = 0.5 // the default --min-confidence
)

type labelFile struct {
	Tool    string `yaml:"tool"`
	Entries []struct {
		Version  string   `yaml:"version"`
		Heading  []string `yaml:"heading"`
		Text     string   `yaml:"text"`
		Breaking *bool    `yaml:"breaking"`
	} `yaml:"entries"`
}

// TestSpec_classification_1: breaking precision over the hand-labelled set is at least 0.9 at the
// default --min-confidence. Recall is logged. Tools with a registry entry use its markers and the
// ignore_sections of its first source.
func TestSpec_classification_1(t *testing.T) {
	t.Parallel()
	l, err := registry.NewLinter()
	require.NoError(t, err)
	entries, err := l.LoadFS(registryfiles.FS(), ".")
	require.NoError(t, err)
	byName := map[string]*registry.Entry{}
	for _, e := range entries {
		byName[e.Name] = e
	}
	paths, err := filepath.Glob(filepath.Join(testdata, "labels", "*.yaml"))
	require.NoError(t, err)
	require.Len(t, paths, labelTools)

	var tp, fp, fn, total int
	for _, p := range paths {
		raw, readErr := os.ReadFile(p)
		require.NoError(t, readErr)
		var f labelFile
		require.NoError(t, yaml.Unmarshal(raw, &f), p)
		var o classify.Options
		if e := byName[f.Tool]; e != nil {
			o.BreakingMarkers = e.BreakingMarkers
			if len(e.Sources) > 0 {
				o.IgnoreSections = e.Sources[0].IgnoreSections
			}
		}
		c := newClassifier(t, o)
		var ftp, ffp, ffn int
		for i, e := range f.Entries {
			require.NotNil(t, e.Breaking, "%s entry %d has no label", f.Tool, i)
			kind, conf := c.Classify(e.Heading, e.Text)
			predicted := kind == classify.KindBreaking && conf >= labelConfidence
			switch {
			case predicted && *e.Breaking:
				ftp++
			case predicted:
				ffp++
				t.Logf("false positive: %s %s (%.2f): %.100s", f.Tool, e.Version, conf, e.Text)
			case *e.Breaking:
				ffn++
				t.Logf("false negative: %s %s %s %.2f %v: %.160s", f.Tool, e.Version, kind, conf, e.Heading, e.Text)
			}
		}
		t.Logf("%-14s entries %2d  tp %2d  fp %2d  fn %2d", f.Tool, len(f.Entries), ftp, ffp, ffn)
		tp, fp, fn, total = tp+ftp, fp+ffp, fn+ffn, total+len(f.Entries)
	}
	require.Equal(t, labelEntries, total)
	require.Positive(t, tp+fp, "the set must contain predicted breaking entries")
	precision := float64(tp) / float64(tp+fp)
	recall := float64(tp) / float64(max(tp+fn, 1))
	t.Logf("breaking precision %.3f (%d/%d), recall %.3f (%d/%d)", precision, tp, tp+fp, recall, tp, tp+fn)
	require.GreaterOrEqual(t, precision, minPrecision)
}
