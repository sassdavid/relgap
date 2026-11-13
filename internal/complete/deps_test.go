package complete_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNoNetworkReachable proves that completion can't reach GitHub or any other host: no HTTP
// package is linked into this package.
func TestNoNetworkReachable(t *testing.T) {
	t.Parallel()
	out, err := exec.CommandContext(t.Context(), "go", "list", "-deps", ".").Output()
	require.NoError(t, err)
	deps := strings.Fields(string(out))
	require.Contains(t, deps, "github.com/spf13/cobra")
	for _, dep := range deps {
		for _, banned := range []string{
			"github.com/sassdavid/relgap/internal/httpx",
			"github.com/sassdavid/relgap/internal/github",
			"net/http",
		} {
			assert.False(t, dep == banned || strings.HasPrefix(dep, banned+"/"), "completion links %s", dep)
		}
	}
}
