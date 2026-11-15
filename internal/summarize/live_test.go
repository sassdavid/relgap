//go:build live

package summarize_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/summarize"
)

// bedrockModelEnv names the model or inference profile the smoke test calls. live.yml sets it
// from the RELGAP_BEDROCK_MODEL repository variable.
const bedrockModelEnv = "RELGAP_BEDROCK_MODEL"

// TestSpec_mcp_and_llm_4 summarizes a small digest with Bedrock Converse. It skips without a
// model or AWS credentials, so it costs nothing until both are configured.
func TestSpec_mcp_and_llm_4(t *testing.T) {
	model := os.Getenv(bedrockModelEnv)
	if model == "" {
		t.Skip(bedrockModelEnv + " is not set")
	}
	cfg, err := summarize.LoadAWSConfig(t.Context(), "", nil)
	if err != nil {
		t.Skipf("no AWS config: %v", err)
	}
	if _, err = cfg.Credentials.Retrieve(t.Context()); err != nil {
		t.Skipf("no AWS credentials: %v", err)
	}
	res, err := summarize.Summarize(t.Context(), summarize.NewBedrock(cfg, model), testDigest(), summarize.Options{
		Provider: summarize.ProviderBedrock, Model: model, MaxInputTokens: 30000,
	})
	require.NoError(t, err)
	require.NotNil(t, res.Summary, "warnings: %v", res.Warnings)
	assert.NotEmpty(t, res.Summary.Risks)
	t.Logf("risks %v, actions %v, dropped %d", res.Summary.Risks, res.Summary.Actions, res.Summary.Dropped)
}
