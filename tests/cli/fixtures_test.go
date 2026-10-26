package sandbox_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	launcherconfig "opencode-sandbox/internal/config"

	"github.com/stretchr/testify/require"
)

func runGit(t *testing.T, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "git", args...).CombinedOutput()
	require.NoError(t, err, "prepare Git fixture: %s", output)
}

func newWorktree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	repo, worktree := filepath.Join(root, "repo"), filepath.Join(root, "worktree")
	runGit(t, "init", "--initial-branch=main", repo)
	runGit(t, "-C", repo, "-c", "user.name=Sandbox Test", "-c", "user.email=sandbox@example.invalid",
		"-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-m", "Initial commit")
	runGit(t, "-C", repo, "worktree", "add", "-b", "sandbox", worktree)
	return worktree
}

func writeSettingsAt(t *testing.T, configDir, contents string) {
	t.Helper()
	dir := filepath.Join(configDir, "opencode-sandbox")
	require.NoError(t, os.MkdirAll(dir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"), []byte(contents), 0600))
}

func repoSettingsFixture(t *testing.T, config string) (string, string, launcherconfig.Paths) {
	t.Helper()
	paths := launcherconfig.Paths{ConfigDir: t.TempDir(), StateDir: t.TempDir()}
	root := filepath.Join(t.TempDir(), "repo")
	runGit(t, "init", root)
	path := filepath.Join(root, ".opencode-sandbox.toml")
	require.NoError(t, os.WriteFile(path, []byte(config), 0600))
	return root, path, paths
}
