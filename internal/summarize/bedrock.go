package summarize

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

// bedrockAppID is sent in the AWS user agent.
const bedrockAppID = "relgap"

// Bedrock calls the Amazon Bedrock Converse API.
type Bedrock struct {
	client *bedrockruntime.Client
	model  string // a model ID, inference profile ID or ARN
}

// NewBedrock returns a provider for cfg; optFns change the client, such as its endpoint in tests.
func NewBedrock(cfg aws.Config, model string, optFns ...func(*bedrockruntime.Options)) *Bedrock {
	return &Bedrock{client: bedrockruntime.NewFromConfig(cfg, optFns...), model: model}
}

// LoadAWSConfig reads the shared AWS config and credentials for profile (empty means the
// default chain). It makes no request; credentials are fetched on the first call. A non-nil
// transport carries every request.
func LoadAWSConfig(ctx context.Context, profile string, transport http.RoundTripper) (aws.Config, error) {
	opts := []func(*awsconfig.LoadOptions) error{awsconfig.WithAppID(bedrockAppID)}
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

// Complete sends p with Converse and joins the text blocks of the answer.
func (b *Bedrock) Complete(ctx context.Context, p Prompt) (string, error) {
	out, err := b.client.Converse(ctx, &bedrockruntime.ConverseInput{
		ModelId: aws.String(b.model),
		System:  []types.SystemContentBlock{&types.SystemContentBlockMemberText{Value: p.System}},
		Messages: []types.Message{{
			Role:    types.ConversationRoleUser,
			Content: []types.ContentBlock{&types.ContentBlockMemberText{Value: p.User}},
		}},
	})
	if err != nil {
		return "", fmt.Errorf("bedrock converse: %w", err)
	}
	if out.StopReason == types.StopReasonGuardrailIntervened || out.StopReason == types.StopReasonContentFiltered {
		return "", fmt.Errorf("bedrock converse: the answer was blocked (stopReason %s)", out.StopReason)
	}
	msg, ok := out.Output.(*types.ConverseOutputMemberMessage)
	if !ok {
		return "", fmt.Errorf("bedrock converse: no message in the answer")
	}
	var s strings.Builder
	for _, c := range msg.Value.Content {
		if t, isText := c.(*types.ContentBlockMemberText); isText {
			s.WriteString(t.Value)
		}
	}
	return s.String(), nil
}
