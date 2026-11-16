package summarize

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/aws/smithy-go/auth/bearer"
)

// Bedrock calls the Amazon Bedrock Converse API.
type Bedrock struct {
	client *bedrockruntime.Client
	model  string // a model ID, inference profile ID or ARN
}

// NewBedrock returns a provider for cfg; optFns change the client, such as its endpoint in tests.
func NewBedrock(cfg aws.Config, model string, optFns ...func(*bedrockruntime.Options)) *Bedrock {
	return &Bedrock{client: bedrockruntime.NewFromConfig(cfg, optFns...), model: model}
}

// WithBedrockAPIKey makes the client send key, an Amazon Bedrock API key, as a bearer token
// instead of signing with SigV4.
func WithBedrockAPIKey(key string) func(*bedrockruntime.Options) {
	return func(o *bedrockruntime.Options) {
		o.BearerAuthTokenProvider = bearer.StaticTokenProvider{Token: bearer.Token{Value: key}}
		o.AuthSchemePreference = []string{"httpBearerAuth"}
	}
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
