package summarize

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/smithy-go/logging"
)

// awsAppID is sent in the AWS user agent.
const awsAppID = "relgap"

// LoadAWSConfig reads the shared AWS config and credentials for profile (empty means the
// default chain). It makes no request; credentials are fetched on the first call. A non-nil
// transport carries every request, and the SDK logs to logger.
func LoadAWSConfig(
	ctx context.Context, profile string, transport http.RoundTripper, logger *slog.Logger,
) (aws.Config, error) {
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithAppID(awsAppID),
		awsconfig.WithLogger(sdkLogger{l: logger, ctx: ctx}),
	}
	if profile != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(profile))
	}
	if transport != nil {
		opts = append(opts, awsconfig.WithHTTPClient(&http.Client{Transport: transport}))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return aws.Config{}, fmt.Errorf("load AWS config: %w", err)
	}
	if cfg.Region == "" {
		return aws.Config{}, fmt.Errorf("load AWS config: no region; set AWS_REGION or region in the profile")
	}
	return cfg, nil
}

// sdkLogger sends the AWS SDK's log lines to slog: warnings at Warn, the rest at Debug. Without
// it the SDK writes to [os.Stderr].
type sdkLogger struct {
	l   *slog.Logger
	ctx context.Context //nolint:containedctx // the SDK calls Logf without a context; WithContext sets it
}

func (s sdkLogger) Logf(c logging.Classification, format string, v ...any) {
	level := slog.LevelDebug
	if c == logging.Warn {
		level = slog.LevelWarn
	}
	s.l.Log(s.ctx, level, "aws sdk: "+fmt.Sprintf(format, v...))
}

func (s sdkLogger) WithContext(ctx context.Context) logging.Logger {
	s.ctx = ctx
	return s
}
