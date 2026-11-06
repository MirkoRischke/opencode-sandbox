package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegistryRetainsDeletedWorktreeOwnership(t *testing.T) {
	t.Parallel()
	state, parent := t.TempDir(), t.TempDir()
	repo, tree := filepath.Join(parent, "repo"), filepath.Join(parent, "tree")
	git(t, "init", "--initial-branch=main", repo)
	git(t, "-C", repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-m", "initial")
	git(t, "-C", repo, "worktree", "add", "-b", "other", tree)
	checkout, linked := resolve(t, repo), resolve(t, tree)
	for _, w := range []Workspace{checkout, linked, linked} {
		require.NoError(t, Track(state, w))
	}
	git(t, "-C", repo, "worktree", "remove", tree)
	records, err := Records(state)
	require.NoError(t, err)
	require.Len(t, records, 2)
	for _, record := range records {
		require.Equal(t, checkout.RepositoryID(), record.RepositoryID)
		require.Contains(t, []string{checkout.ID(), linked.ID()}, record.ID())
	}
	info, err := os.Stat(RecordPath(state, linked.ID()))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestRegistryRejectsInvalidIdentity(t *testing.T) {
	t.Parallel()
	state := t.TempDir()
	w := resolve(t, t.TempDir())
	require.NoError(t, Track(state, w))
	require.NoError(t, os.WriteFile(RecordPath(state, w.ID()), []byte(`{"root":"/different","repository_id":"repo"}`), 0600))
	_, err := Records(state)
	require.ErrorContains(t, err, "invalid workspace ownership")
}
