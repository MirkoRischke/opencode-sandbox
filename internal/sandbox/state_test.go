package sandbox

import (
	"os"
	"path/filepath"
	"testing"

	"opencode-sandbox/internal/workspace"

	"github.com/stretchr/testify/require"
)

func TestStateLocationCannotOverlapWorkspace(t *testing.T) {
	t.Parallel()
	w, err := workspace.Resolve(t.Context(), t.TempDir())
	require.NoError(t, err)
	link := filepath.Join(t.TempDir(), "state-home")
	require.NoError(t, os.Symlink(w.Root(), link))

	for _, stateDir := range []string{w.Root(), link} {
		require.ErrorContains(t, validateStateLocation(w, stateDir), "overlaps workspace mount source")
	}
	require.NoError(t, validateStateLocation(w, t.TempDir()))
}
