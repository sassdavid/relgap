package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/sassdavid/relgap/internal/app"
	"github.com/sassdavid/relgap/internal/vex"
)

const (
	flagVEX    = "vex"
	keyVulnVEX = "vulns.vex"
)

// addVEXFlag adds --vex, which sets vulns.vex.
func addVEXFlag(cmd *cobra.Command) {
	cmd.Flags().StringSlice(flagVEX, nil,
		"OpenVEX v0.2.0 document whose not_affected and fixed statements suppress advisories; repeatable; "+
			"replaces config vulns.vex")
	_ = cmd.MarkFlagFilename(flagVEX, "json")
}

// suppression returns the rules of the config's vulns.ignore entries and of the OpenVEX documents
// in vulns.vex or --vex. A document that can't be read is a usage error.
func (s *session) suppression() (*vex.Rules, error) {
	cfg := s.d.Config
	ignores := make([]vex.Ignore, 0, len(cfg.Config.Vulns.Ignore))
	for _, e := range cfg.Config.Vulns.Ignore {
		ignores = append(ignores, vex.Ignore{ID: e.ID, Reason: e.Reason, Until: e.Until, Tools: e.Tools})
	}
	var files []vex.File
	for _, p := range cfg.FilePaths(keyVulnVEX, cfg.Config.Vulns.VEX) {
		f, err := vex.Read(p)
		if err != nil {
			return nil, usageError(err)
		}
		files = append(files, f)
	}
	rules, err := vex.New(ignores, files, s.d.Clock())
	if err != nil {
		return nil, usageError(err)
	}
	return rules, nil
}

// warnSuppression warns about each vulns.ignore entry past its until date and, with unused, about
// each entry in effect that suppressed nothing, so stale entries show up.
func warnSuppression(ctx context.Context, d *app.Deps, r *vex.Rules, unused bool) {
	if r == nil {
		return
	}
	for _, e := range r.Expired() {
		d.Logger.WarnContext(ctx, "vulns.ignore entry has expired and suppresses nothing", "id", e.ID, "until", e.Until)
	}
	if !unused {
		return
	}
	for _, e := range r.Unused() {
		d.Logger.WarnContext(ctx, "vulns.ignore entry suppressed nothing; remove it if it is stale", "id", e.ID)
	}
}
