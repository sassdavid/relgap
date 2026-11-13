package complete_test

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/complete"
	"github.com/sassdavid/relgap/internal/registry"
)

func TestTools(t *testing.T) {
	t.Parallel()
	cfg := func(context.Context, *cobra.Command) (registry.ConfigLayer, error) {
		return registry.ConfigLayer{Tools: map[string]string{"zz-inhouse": "acme/zz-inhouse"}}, nil
	}
	cs, err := complete.Tools(cfg)(t.Context(), &cobra.Command{}, "")
	require.NoError(t, err)
	assert.Contains(t, cs, complete.Choice{Value: "terraform", Desc: "hashicorp/terraform"})
	assert.Contains(t, cs, complete.Choice{Value: "zz-inhouse", Desc: "acme/zz-inhouse"})
	seen := map[string]bool{}
	for _, c := range cs {
		assert.False(t, seen[c.Value], "duplicate %s", c.Value)
		seen[c.Value] = true
	}

	broken := func(context.Context, *cobra.Command) (registry.ConfigLayer, error) {
		return registry.ConfigLayer{}, errors.New("bad config")
	}
	_, err = complete.Tools(broken)(t.Context(), &cobra.Command{}, "")
	require.Error(t, err)
}
