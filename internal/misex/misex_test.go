package misex_test

import (
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/execx"
	"github.com/sassdavid/relgap/internal/misex"
	"github.com/sassdavid/relgap/internal/testutil/fakebin"
)

func TestMain(m *testing.M) { fakebin.Main(m) }

func client(t *testing.T, pathDir string) *misex.Client {
	t.Helper()
	environ := []string{"PATH=" + pathDir}
	logger := slog.New(slog.DiscardHandler)
	return misex.New(execx.NewRunner(logger, execx.WithEnviron(func() []string { return environ })))
}

func TestRegistry(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		rule    fakebin.Rule
		want    []misex.RegistryEntry
		wantErr string
	}{
		{
			name: "decodes and ignores unknown fields",
			rule: fakebin.Rule{
				Stdout: `[{"short":"terraform","backends":["aqua:hashicorp/terraform"],"bins":["terraform"],"description":"x"},` +
					`{"short":"aws-cli","backends":["aqua:aws/aws-cli"],"aliases":["aws"]}]`,
			},
			want: []misex.RegistryEntry{
				{Short: "terraform", Backends: []string{"aqua:hashicorp/terraform"}},
				{Short: "aws-cli", Backends: []string{"aqua:aws/aws-cli"}, Aliases: []string{"aws"}},
			},
		},
		{
			name:    "old mise",
			rule:    fakebin.Rule{Stderr: "unexpected argument '--json'", ExitCode: 2},
			wantErr: "mise exited with code 2",
		},
		{
			name:    "not json",
			rule:    fakebin.Rule{Stdout: "terraform aqua:hashicorp/terraform\n"},
			wantErr: "decode mise registry --json",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.rule.Args = []string{"registry", "--json"}
			bin := fakebin.Install(t, "mise", fakebin.Spec{Rules: []fakebin.Rule{tt.rule}})
			got, err := client(t, filepath.Dir(bin)).Registry(t.Context())
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRegistryNotInstalled(t *testing.T) {
	t.Parallel()
	_, err := client(t, t.TempDir()).Registry(t.Context())
	require.ErrorIs(t, err, misex.ErrNotInstalled)
}
