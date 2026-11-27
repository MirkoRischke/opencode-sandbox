package sandbox_test

import (
	"bytes"
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
	requireMissingStatePath(t, tree, state, config, saved)
	requireMissingStatePath(t, repo, state, config, filepath.Dir(saved), "--repo")
	require.NoError(t, os.MkdirAll(saved, 0700))
	output, stderr, err := runStatePath(t, tree, state, config)
	require.NoError(t, err, "%s", stderr)
	require.Empty(t, stderr)
	require.Equal(t, saved+"\n", output)

	runGit(t, "-C", repo, "worktree", "remove", tree)
	output, stderr, err = runStatePath(t, repo, state, config, "--repo")
	require.NoError(t, err, "%s", stderr)
	require.Empty(t, stderr)
	require.Equal(t, filepath.Dir(saved)+"\n", output)
	require.DirExists(t, saved)
}

func TestStatePathWithoutGit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	config := t.TempDir()
	root := sandbox.StateRoot(filepath.Join(dir, "host-state"))

	requireMissingStatePath(t, dir, "host-state", config, root, "--all")

	w, err := workspace.Resolve(t.Context(), dir)
	require.NoError(t, err)
	path := sandbox.WorkspaceStatePath(filepath.Join(dir, "host-state"), w)
	require.NoError(t, os.MkdirAll(path, 0700))
	output, stderr, err := runStatePath(t, dir, "host-state", config)
	require.NoError(t, err, "%s", stderr)
	require.Empty(t, stderr)
	require.Equal(t, path+"\n", output)

	output, stderr, err = runStatePath(t, dir, "host-state", config, "--repo")
	require.Error(t, err)
	require.Empty(t, output)
	require.Contains(t, stderr, "requires a Git checkout or worktree root")

	// All-state lookup works even when the current workspace cannot be resolved.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git"), []byte("invalid"), 0600))
	output, stderr, err = runStatePath(t, dir, "host-state", config, "--all")
	require.NoError(t, err, "%s", stderr)
	require.Empty(t, stderr)
	require.Equal(t, root+"\n", output)
}

func requireMissingStatePath(t *testing.T, directory, state, config, expected string, flags ...string) {
	t.Helper()
	output, stderr, err := runStatePath(t, directory, state, config, flags...)
	require.NoError(t, err, "%s", stderr)
	require.Equal(t, expected+"\n", output)
	require.Contains(t, stderr, "Warning: persistent state directory does not exist yet")
	require.NoDirExists(t, expected)
}

func runStatePath(t *testing.T, directory, state, config string, flags ...string) (string, string, error) {
	t.Helper()
	command := exec.CommandContext(t.Context(), buildLauncher(t), append([]string{"state", "path"}, flags...)...)
	command.Dir = directory
	command.Env = append(os.Environ(), "XDG_STATE_HOME="+state, "XDG_CONFIG_HOME="+config)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	return string(output), stderr.String(), err
}
