package summarize

import (
	"context"
	"log/slog"

	"github.com/aws/smithy-go/logging"

	"github.com/sassdavid/relgap/internal/report"
)

// SDKLogger exposes sdkLogger to the external tests.
func SDKLogger(ctx context.Context, l *slog.Logger) logging.Logger { return sdkLogger{l: l, ctx: ctx} }

// UserPrompt returns the user message that Summarize sends for d.
func UserPrompt(d *report.Digest) (string, error) {
	upgrades, entries, err := encode(d)
	if err != nil {
		return "", err
	}
	return userPrompt(upgrades, entries), nil
}
