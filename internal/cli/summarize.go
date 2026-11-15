package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sassdavid/relgap/internal/app"
	"github.com/sassdavid/relgap/internal/buildinfo"
	"github.com/sassdavid/relgap/internal/report"
	"github.com/sassdavid/relgap/internal/summarize"
)

const flagSummarize = "summarize"

// summarizeDigest adds an LLM summary to data when --summarize is set. Every problem is a
// warning, and the summary never changes the entries, the --fail-on gate or the exit code.
func summarizeDigest(cmd *cobra.Command, d *app.Deps, g *globals, f digestFlags, data *report.Digest) {
	if !f.summarize {
		return
	}
	ctx := cmd.Context()
	warn := func(msg string) { d.Logger.WarnContext(ctx, "--summarize: "+msg) }
	if g.offline {
		warn("skipped with --offline")
		return
	}
	c := d.Config.Config.LLM
	timeout, err := summarize.Timeout(c)
	if err != nil {
		warn(err.Error() + "; no summary")
		return
	}
	p, err := summarize.New(ctx, c, summarize.Env{
		Lookup:    d.Env.Lookup,
		Transport: d.HTTPTransport,
		Logger:    d.Logger,
		UserAgent: userAgent(),
	})
	if err != nil {
		warn(err.Error() + "; no summary")
		return
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	res, err := summarize.Summarize(callCtx, p, data, summarize.Options{
		Provider:       c.Provider,
		Model:          c.Model,
		MaxInputTokens: c.MaxInputTokens,
	})
	switch {
	case err != nil && ctx.Err() == nil && errors.Is(callCtx.Err(), context.DeadlineExceeded):
		warn(fmt.Sprintf("the model didn't answer within llm.timeout (%s); no summary", timeout))
		return
	case err != nil:
		warn(err.Error() + "; no summary")
		return
	}
	for _, w := range res.Warnings {
		warn(w)
	}
	data.Summary = res.Summary
}

func userAgent() string {
	return fmt.Sprintf("relgap/%s (+https://github.com/sassdavid/relgap)", buildinfo.Get().Version)
}
