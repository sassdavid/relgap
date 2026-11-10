package config_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/invopop/jsonschema"

	"github.com/sassdavid/relgap/internal/config"
	"github.com/sassdavid/relgap/internal/testutil"
	"github.com/sassdavid/relgap/schemas"
)

// generateSchema reflects the Config struct into schemas/config-v1.json.
func generateSchema(t *testing.T) []byte {
	t.Helper()
	r := &jsonschema.Reflector{
		DoNotReference:             true,
		RequiredFromJSONSchemaTags: true,
		Anonymous:                  true,
	}
	s := r.Reflect(&config.Config{})
	s.ID = "https://github.com/sassdavid/relgap/schemas/" + schemas.ConfigV1
	s.Title = "relgap configuration"
	s.Description = "Schema for $XDG_CONFIG_HOME/relgap/config.yaml and .relgap.yaml. " +
		"Generated from internal/config by `mise run test:golden -- -update`; do not edit."
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	return append(b, '\n')
}

// TestSpec_config_3 checks that the checked-in schema matches the Go struct.
func TestSpec_config_3(t *testing.T) {
	t.Parallel()
	testutil.Golden(t, filepath.Join("..", "..", "schemas", schemas.ConfigV1), generateSchema(t))
}
