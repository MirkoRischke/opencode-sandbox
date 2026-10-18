package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"opencode-sandbox/internal/workspace"

	"github.com/stretchr/testify/require"
)

func TestSandboxNameUsesOnlyWorkspaceIdentity(t *testing.T) {
	t.Parallel()
	for _, folder := range []string{"VM Test", strings.Repeat("tree", 30)} {
		root := filepath.Join(t.TempDir(), folder)
		require.NoError(t, os.Mkdir(root, 0700))
		w, err := workspace.Resolve(context.Background(), root)
		require.NoError(t, err)
		name := sandboxName(w)
		require.LessOrEqual(t, len(name), 128)
		require.Regexp(t, `^oc-[a-z0-9-]+-[0-9a-f]{8}$`, name)
		require.True(t, strings.HasSuffix(name, w.ID()[:8]))
	}
	first, err := workspace.Resolve(context.Background(), t.TempDir())
	require.NoError(t, err)
	second, err := workspace.Resolve(context.Background(), t.TempDir())
	require.NoError(t, err)
	require.NotEqual(t, sandboxName(first), sandboxName(second))
}
