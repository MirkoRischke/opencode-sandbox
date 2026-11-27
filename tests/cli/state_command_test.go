package sandbox_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"opencode-sandbox/internal/sandbox"
	"opencode-sandbox/internal/workspace"

	"github.com/stretchr/testify/require"
)

func TestStatePathWorkspaceAndRepository(t *testing.T) {
	t.Parallel()
	state, config := t.TempDir(), t.TempDir()
	writeSettingsAt(t, config, "invalid = [")
	tree := newWorktree(t)
	repo := filepath.Join(filepath.Dir(tree), "repo")

	w, err := workspace.Resolve(t.Context(), tree)
	require.NoError(t, err)
	saved := sandbox.WorkspaceStatePath(state, w)
	require.NoError(t, os.MkdirAll(saved, 0700))
	output, err := runStatePath(t, tree, state, config)
	require.NoError(t, err, "%s", output)
	require.Equal(t, saved+"\n", output)

	runGit(t, "-C", repo, "worktree", "remove", tree)
	output, err = runStatePath(t, repo, state, config, "--repo")
	require.NoError(t, err, "%s", output)
	require.Equal(t, filepath.Dir(saved)+"\n", output)
	require.DirExists(t, saved)
}

func TestStatePathWithoutGit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	config := t.TempDir()
	root := sandbox.StateRoot(filepath.Join(dir, "host-state"))

	output, err := runStatePath(t, dir, "host-state", config, "--all")
	require.Error(t, err)
	require.Contains(t, output, "no such file or directory")
	require.NoDirExists(t, root)

	w, err := workspace.Resolve(t.Context(), dir)
	require.NoError(t, err)
	path := sandbox.WorkspaceStatePath(filepath.Join(dir, "host-state"), w)
	require.NoError(t, os.MkdirAll(path, 0700))
	output, err = runStatePath(t, dir, "host-state", config)
	require.NoError(t, err, "%s", output)
	require.Equal(t, path+"\n", output)

	output, err = runStatePath(t, dir, "host-state", config, "--repo")
	require.Error(t, err)
	require.Contains(t, output, "requires a Git checkout or worktree root")

	// All-state lookup works even when the current workspace cannot be resolved.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git"), []byte("invalid"), 0600))
	output, err = runStatePath(t, dir, "host-state", config, "--all")
	require.NoError(t, err, "%s", output)
	require.Equal(t, root+"\n", output)
}

func runStatePath(t *testing.T, directory, state, config string, flags ...string) (string, error) {
	t.Helper()
	command := exec.CommandContext(t.Context(), buildLauncher(t), append([]string{"state", "path"}, flags...)...)
	command.Dir = directory
	command.Env = append(os.Environ(), "XDG_STATE_HOME="+state, "XDG_CONFIG_HOME="+config)
	output, err := command.CombinedOutput()
	return string(output), err
}
