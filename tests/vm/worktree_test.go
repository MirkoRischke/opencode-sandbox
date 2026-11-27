package sandbox_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"opencode-sandbox/internal/sandbox"
	"opencode-sandbox/internal/workspace"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	m "github.com/superradcompany/microsandbox/sdk/go"
)

func TestSandboxEditsAreVisibleOnHost(t *testing.T) {
	t.Parallel()
	s := startSandbox(t)
	file := filepath.Join(s.Worktree, "guest.txt")

	execInSandbox(t, s, "bash", "-eu", "-c", `echo 'written in sandbox' > guest.txt`)

	contents, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, "written in sandbox\n", string(contents))
}

func TestSandboxCanBeSelectedByMarkerLabel(t *testing.T) {
	t.Parallel()
	s := startSandbox(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	page, err := m.ListSandboxesWith(ctx, m.WithListLabels(map[string]string{"opencode-sandbox": ""}))
	require.NoError(t, err)
	names := make([]string, 0, len(page.Sandboxes))
	for _, handle := range page.Sandboxes {
		names = append(names, handle.Name())
	}
	require.Contains(t, names, s.Name)
}

func TestGitStatusAndCommitWorkInMountedLinkedWorktree(t *testing.T) {
	t.Parallel()
	s := startSandbox(t)
	err := os.WriteFile(filepath.Join(s.Worktree, "host.txt"), []byte("written on host\n"), 0644)
	require.NoError(t, err)

	assert.Equal(t, "/workspace\n", execInSandbox(t, s, "pwd"))
	execInSandbox(t, s, "test", "!", "-e", s.Worktree)
	assert.Equal(t, "?? host.txt\n", execInSandbox(t, s, "git", "status", "--porcelain"))
	execInSandbox(t, s, "git", "add", "host.txt")
	execInSandbox(t, s, "git", "-c", "user.name=Sandbox Test", "-c", "user.email=sandbox@example.invalid",
		"-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "commit", "-m", "Commit from /workspace")

	head := execInSandbox(t, s, "git", "rev-parse", "HEAD")
	assert.Equal(t, head, runGit(t, "-C", s.Worktree, "rev-parse", "HEAD"))
	assert.Empty(t, runGit(t, "-C", s.Worktree, "status", "--porcelain"))
}

func TestPreparingAgainReusesSandboxState(t *testing.T) {
	t.Parallel()
	s := startSandbox(t)
	execInSandbox(t, s, "touch", "/root/persistent-state")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	resumed, err := sandbox.Prepare(ctx, s.Worktree, nil, testSettings(t))

	require.NoError(t, err)
	assert.Equal(t, s.Name, resumed.Name)
	execInSandbox(t, resumed, "test", "-f", "/root/persistent-state")
	require.NoError(t, resumed.Close())
	execInSandbox(t, s, "test", "-f", "/root/persistent-state")
}

func TestClosingSandboxConnectionDoesNotRestartVM(t *testing.T) {
	t.Parallel()
	worktree := newWorktree(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	first, err := sandbox.Prepare(ctx, worktree, nil, testSettings(t))
	require.NoError(t, err)
	bootID := execInSandbox(t, first, "cat", "/proc/sys/kernel/random/boot_id")
	require.NoError(t, first.Close())

	resumed := prepareSandbox(t, worktree)

	assert.Equal(t, bootID, execInSandbox(t, resumed, "cat", "/proc/sys/kernel/random/boot_id"))
}

func TestPreparingStoppedSandboxResumesState(t *testing.T) {
	t.Parallel()
	s := startSandbox(t)
	execInSandbox(t, s, "touch", "/root/persistent-state")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	w, err := workspace.Resolve(ctx, s.Worktree)
	require.NoError(t, err)
	labels := func(s *sandbox.Sandbox) map[string]string {
		var stored struct {
			Labels map[string]string `json:"labels"`
		}
		require.NoError(t, json.Unmarshal([]byte(storedSandboxConfig(t, s)), &stored))
		return stored.Labels
	}
	fresh := labels(s)
	require.Equal(t, "workspace-vm", fresh["kind"])
	require.Equal(t, w.ID(), fresh["workspace-id"])
	require.Equal(t, w.Root(), fresh["workspace-path"])
	require.Equal(t, "worktree", fresh["workspace-name"])
	require.Equal(t, w.RepositoryID(), fresh["repository-id"])
	require.Equal(t, "repo", fresh["repository-name"])
	require.Equal(t, "true", fresh["linked-worktree"])
	require.Equal(t, filepath.Join(w.RepositoryRoot(), ".git"), fresh["git-common-dir"])
	firstLaunch, err := time.Parse(time.RFC3339Nano, fresh["last-launched-at"])
	require.NoError(t, err)
	require.Equal(t, time.UTC, firstLaunch.Location())
	// Simulate the user stopping a persistent VM outside the launcher.
	require.NoError(t, s.Stop(ctx))

	resumed, err := sandbox.Prepare(ctx, s.Worktree, nil, testSettings(t))

	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, resumed.Close()) })
	assert.Equal(t, s.Name, resumed.Name)
	resumedLabels := labels(resumed)
	lastLaunch, err := time.Parse(time.RFC3339Nano, resumedLabels["last-launched-at"])
	require.NoError(t, err)
	require.Equal(t, time.UTC, lastLaunch.Location())
	require.True(t, lastLaunch.After(firstLaunch))
	fresh["last-launched-at"] = resumedLabels["last-launched-at"]
	require.Equal(t, fresh, resumedLabels)
	execInSandbox(t, resumed, "test", "-f", "/root/persistent-state")
}

func TestShutdownStopsSandboxAndPreservesGuestState(t *testing.T) {
	t.Parallel()
	s := startSandbox(t)
	execInSandbox(t, s, "touch", "/root/persistent-state")
	require.NoError(t, s.Shutdown())
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	handle, err := m.GetSandbox(ctx, s.Name)
	require.NoError(t, err)
	require.Equal(t, m.SandboxStatusStopped, handle.Status())

	resumed, err := sandbox.Prepare(ctx, s.Worktree, nil, testSettings(t))
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, resumed.Close()) })
	execInSandbox(t, resumed, "test", "-f", "/root/persistent-state")
}

func TestNewProjectToolsRequireSnapshotRebuildAndVMRecreation(t *testing.T) {
	t.Parallel()
	s := startSandbox(t)
	require.NoError(t, os.WriteFile(filepath.Join(s.Worktree, ".mise.toml"), []byte("[tools]\nbun = \"1.3.14\"\n"), 0644))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	resumed, err := sandbox.Prepare(ctx, s.Worktree, nil, testSettings(t))

	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, resumed.Close()) })
	require.Error(t, resumed.Exec(ctx, nil, nil, nil, "bun", "--version"))
	_, err = sandbox.CreateSnapshot(ctx, s.Worktree, nil, testSettings(t))
	require.NoError(t, err)
	require.NoError(t, sandbox.Remove(ctx, s.Worktree))
	recreated := prepareSandbox(t, s.Worktree)
	require.Equal(t, "1.3.14\n", execInSandbox(t, recreated, "bun", "--version"))
}

func TestPreparingInstallsProjectTools(t *testing.T) {
	t.Parallel()
	worktree := newWorktree(t)
	require.NoError(t, os.WriteFile(filepath.Join(worktree, "mise.toml"), []byte("[tools]\nbun = \"1.3.14\"\n"), 0644))
	s := prepareSandbox(t, worktree)
	require.Equal(t, "1.3.14\n", execInSandbox(t, s, "bun", "--version"))
}

func TestProjectMiseEnvironmentIsTrustedBeforeLoading(t *testing.T) {
	t.Parallel()
	worktree := newWorktree(t)
	config := "[env]\n_.path = [\"./bin\"]\n"
	require.NoError(t, os.WriteFile(filepath.Join(worktree, "mise.toml"), []byte(config), 0644))

	s := prepareSandbox(t, worktree)

	execInSandbox(t, s, "mise", "env")
}
