package sandbox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"opencode-sandbox/internal/workspace"

	"github.com/stretchr/testify/require"
	m "github.com/superradcompany/microsandbox/sdk/go"
)

func TestOpenCodeStorageUsesXDGAndPreservesExistingData(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state) //nolint:forbidigo // Exercise host storage discovery; Setenv prevents parallel execution.
	w, err := workspace.Resolve(context.Background(), t.TempDir())
	require.NoError(t, err)
	dir, err := prepareOpenCodeStorage(w)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(state, "opencode-sandbox", "opencode", w.ID()), dir)
	info, err := os.Stat(dir)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0700), info.Mode().Perm())
	database := filepath.Join(dir, "opencode.db")
	require.NoError(t, os.WriteFile(database, []byte("existing history"), 0600))
	again, err := prepareOpenCodeStorage(w)
	require.NoError(t, err)
	require.Equal(t, dir, again)
	contents, err := os.ReadFile(database)
	require.NoError(t, err)
	require.Equal(t, "existing history", string(contents))
}

func TestOpenCodeStorageIsolatedBetweenLinkedWorktrees(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir()) //nolint:forbidigo // Isolate host storage; Setenv prevents parallel execution.
	parent := t.TempDir()
	repo, tree := filepath.Join(parent, "repo"), filepath.Join(parent, "tree")
	for _, args := range [][]string{
		{"init", "--initial-branch=main", repo},
		{"-C", repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-m", "initial"},
		{"-C", repo, "worktree", "add", "-b", "other", tree},
	} {
		out, err := exec.Command("git", args...).CombinedOutput()
		require.NoError(t, err, "%s", out)
	}
	checkout, err := workspace.Resolve(t.Context(), repo)
	require.NoError(t, err)
	linked, err := workspace.Resolve(t.Context(), tree)
	require.NoError(t, err)
	require.Equal(t, checkout.RepositoryID(), linked.RepositoryID())
	first, err := prepareOpenCodeStorage(checkout)
	require.NoError(t, err)
	second, err := prepareOpenCodeStorage(linked)
	require.NoError(t, err)
	require.NotEqual(t, first, second)
	require.NoError(t, os.WriteFile(filepath.Join(first, "opencode.db"), []byte("checkout history"), 0600))
	require.NoFileExists(t, filepath.Join(second, "opencode.db"))
}

func TestOpenCodeStorageFallsBackToHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "") //nolint:forbidigo // Exercise host storage discovery; Setenv prevents parallel execution.
	home := t.TempDir()
	t.Setenv("HOME", home) //nolint:forbidigo // Keep host state inside the test directory.
	w, err := workspace.Resolve(context.Background(), t.TempDir())
	require.NoError(t, err)
	dir, err := prepareOpenCodeStorage(w)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, ".local", "state", "opencode-sandbox", "opencode", w.ID()), dir)
}

func TestOpenCodeStoragePreservesCreationErrors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0600))
	t.Setenv("XDG_STATE_HOME", file) //nolint:forbidigo // Exercise host storage errors; Setenv prevents parallel execution.
	w, err := workspace.Resolve(context.Background(), t.TempDir())
	require.NoError(t, err)
	_, err = prepareOpenCodeStorage(w)
	var pathError *os.PathError
	require.ErrorAs(t, err, &pathError)
	require.ErrorContains(t, err, "create OpenCode host storage")
}

func TestOpenCodeMountIsWritableAndIncludesSQLiteSidecars(t *testing.T) {
	t.Parallel()
	source := t.TempDir()
	mounts := map[string]m.MountConfig{}
	addOpenCodeMount(mounts, source)
	require.Len(t, mounts, 1)
	mount := mounts[openCodeDatabaseDir]
	require.Equal(t, source, mount.Bind)
	require.False(t, mount.Readonly)
	require.Equal(t, &m.MountOwner{UID: 0, GID: 0}, mount.Owner)
	require.Equal(t, openCodeDatabaseDir, filepath.Dir(openCodeDatabase))
}
