package summarize

import (
	"context"
	"log/slog"

	"github.com/aws/smithy-go/logging"
)

// SDKLogger exposes sdkLogger to the external tests.
func SDKLogger(ctx context.Context, l *slog.Logger) logging.Logger { return sdkLogger{l: l, ctx: ctx} }
