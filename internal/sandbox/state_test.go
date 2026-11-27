package sandbox

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStatePathNamesSurviveWorktreeRemoval(t *testing.T) {
	t.Parallel()
	f := newPruneFixture(t)

	repoPath := filepath.Join(f.state, "opencode-sandbox", "state", "repo--"+f.current.RepositoryID())
	require.Equal(t, repoPath, RepositoryStatePath(f.state, f.deleted))
	require.Equal(t, filepath.Join(repoPath, "repo--"+f.current.ID()), WorkspaceStatePath(f.state, f.current))
	require.NoDirExists(t, f.deleted.Root())
	require.FileExists(t, filepath.Join(repoPath, "tree--"+f.deleted.ID(), "history", "session"))
}

func TestStatePathsDistinguishSameNamedDirectories(t *testing.T) {
	t.Parallel()
	first, second := filepath.Join(t.TempDir(), "My Project"), filepath.Join(t.TempDir(), "My Project")
	require.NoError(t, os.Mkdir(first, 0700))
	require.NoError(t, os.Mkdir(second, 0700))
	state := t.TempDir()
	a, b := resolvePruneWorkspace(t, first), resolvePruneWorkspace(t, second)
	require.Equal(t, "my-project--"+a.RepositoryID(), filepath.Base(RepositoryStatePath(state, a)))
	require.NotEqual(t, RepositoryStatePath(state, a), RepositoryStatePath(state, b))
}
