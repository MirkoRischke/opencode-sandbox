package sandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"opencode-sandbox/internal/workspace"

	"github.com/stretchr/testify/require"
)

func TestMissingLinkedWorktree(t *testing.T) {
	t.Parallel()
	parent, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	repo, tree := filepath.Join(parent, "repo"), filepath.Join(parent, "tree with spaces\nand newline")
	git := func(args ...string) {
		t.Helper()
		out, err := exec.CommandContext(t.Context(), "git", args...).CombinedOutput()
		require.NoError(t, err, "%s", out)
	}
	git("init", repo)
	git("-C", repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-m", "initial")
	git("-C", repo, "worktree", "add", "--detach", tree)

	// Scope can be resolved from inside an active linked worktree.
	nested := filepath.Join(tree, "nested")
	require.NoError(t, os.Mkdir(nested, 0700))
	id, err := workspace.CurrentGitRepositoryID(t.Context(), nested)
	require.NoError(t, err)
	w, err := workspace.Resolve(t.Context(), tree)
	require.NoError(t, err)
	require.Equal(t, w.RepositoryID(), id)

	common := filepath.Join(repo, ".git")
	labels := map[string]string{"kind": "workspace-vm", "workspace-path": tree, "repository-id": id, "git-common-dir": common}
	check := func(scope, want string) {
		t.Helper()
		path, err := missingLinkedWorktree(t.Context(), labels, scope)
		require.NoError(t, err)
		require.Equal(t, want, path)
	}
	check(id, "")
	require.NoError(t, os.RemoveAll(tree))
	check(id, "") // Git still lists the prunable worktree.
	git("-C", repo, "worktree", "prune", "--expire", "now")
	check(id, tree)
	check("", tree)
	check("another repository", "")

	require.NoError(t, os.Symlink(filepath.Join(parent, "missing-target"), tree))
	check(id, "") // Even a broken symlink occupies the former path.
	require.NoError(t, os.Remove(tree))

	labels["kind"] = "workspace-preparation"
	check("", "")
	labels["kind"] = "workspace-vm"
	delete(labels, "git-common-dir")
	check("", "")
	labels["git-common-dir"] = common
	require.NoError(t, os.Rename(repo, filepath.Join(parent, "moved")))
	path, err := missingLinkedWorktree(t.Context(), labels, "")
	require.Error(t, err)
	require.Empty(t, path)
}
