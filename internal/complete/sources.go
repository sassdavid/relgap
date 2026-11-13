package complete

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/sassdavid/relgap/internal/registry"
)

// ConfigTools returns the tools and aliases of the user's config files.
type ConfigTools func(ctx context.Context, cmd *cobra.Command) (registry.ConfigLayer, error)

// Tools lists the tool names of the embedded relgap registry, the config and the embedded mise
// registry snapshot, with their repositories as descriptions. It never runs mise or calls GitHub.
func Tools(cfg ConfigTools) Source {
	return func(ctx context.Context, cmd *cobra.Command, _ string) ([]Choice, error) {
		layer, err := cfg(ctx, cmd)
		if err != nil {
			return nil, err
		}
		// A nil mise lister keeps registry.Load on the embedded snapshot.
		r, err := registry.Load(ctx, slog.New(slog.DiscardHandler), nil, layer)
		if err != nil {
			return nil, fmt.Errorf("load registry: %w", err)
		}
		names := r.Names()
		out := make([]Choice, len(names))
		for i, n := range names {
			out[i] = Choice{Value: n.Name, Desc: n.Repo}
		}
		return out, nil
	}
}
