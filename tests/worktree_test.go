package sandbox_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	m "github.com/superradcompany/microsandbox/sdk/go"
)

func TestSandboxEditsAreVisibleOnHost(t *testing.T) {
	s := startSandbox(t)
	file := filepath.Join(s.Worktree, "guest.txt")

	execInSandbox(t, s, "bash", "-eu", "-c", `echo 'written in sandbox' > "$1"`, "--", file)

	contents, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, "written in sandbox\n", string(contents))
}

func TestSandboxCanBeSelectedByAppLabel(t *testing.T) {
	s := startSandbox(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	page, err := m.ListSandboxesWith(ctx, m.WithListLabels(map[string]string{"app": "opencode-sandbox"}))
	require.NoError(t, err)
	names := make([]string, 0, len(page.Sandboxes))
	for _, handle := range page.Sandboxes {
		names = append(names, handle.Name())
	}
	require.Contains(t, names, s.Name)
}

func TestGitStatusWorksInMountedWorktree(t *testing.T) {
	s := startSandbox(t)
	err := os.WriteFile(filepath.Join(s.Worktree, "host.txt"), []byte("written on host\n"), 0644)
	require.NoError(t, err)

	output := execInSandbox(t, s, "git", "status", "--porcelain")

	assert.Equal(t, "?? host.txt\n", output)
}

func TestPreparingAgainReusesSandboxState(t *testing.T) {
	s := startSandbox(t)
	execInSandbox(t, s, "touch", "/root/persistent-state")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	resumed, err := sandbox.Prepare(ctx, s.Worktree, nil, nil, testSettings(t))

	require.NoError(t, err)
	assert.Equal(t, s.Name, resumed.Name)
	execInSandbox(t, resumed, "test", "-f", "/root/persistent-state")
	require.NoError(t, resumed.Close())
	execInSandbox(t, s, "test", "-f", "/root/persistent-state")
}

func TestClosingSandboxConnectionDoesNotRestartVM(t *testing.T) {
	worktree := newWorktree(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	first, err := sandbox.Prepare(ctx, worktree, nil, nil, testSettings(t))
	require.NoError(t, err)
	bootID := execInSandbox(t, first, "cat", "/proc/sys/kernel/random/boot_id")
	require.NoError(t, first.Close())

	resumed := prepareSandbox(t, worktree)

	assert.Equal(t, bootID, execInSandbox(t, resumed, "cat", "/proc/sys/kernel/random/boot_id"))
}

func TestPreparingStoppedSandboxResumesState(t *testing.T) {
	s := startSandbox(t)
	execInSandbox(t, s, "touch", "/root/persistent-state")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	// Simulate the user stopping a persistent VM outside the launcher.
	require.NoError(t, s.Stop(ctx))

	resumed, err := sandbox.Prepare(ctx, s.Worktree, nil, nil, testSettings(t))

	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, resumed.Close()) })
	assert.Equal(t, s.Name, resumed.Name)
	execInSandbox(t, resumed, "test", "-f", "/root/persistent-state")
}

func TestShutdownStopsSandboxAndPreservesGuestState(t *testing.T) {
	s := startSandbox(t)
	execInSandbox(t, s, "touch", "/root/persistent-state")
	require.NoError(t, s.Shutdown())
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	handle, err := m.GetSandbox(ctx, s.Name)
	require.NoError(t, err)
	require.Equal(t, m.SandboxStatusStopped, handle.Status())

	resumed, err := sandbox.Prepare(ctx, s.Worktree, nil, nil, testSettings(t))
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, resumed.Close()) })
	execInSandbox(t, resumed, "test", "-f", "/root/persistent-state")
}

func TestPreparingAgainInstallsNewProjectTools(t *testing.T) {
	s := startSandbox(t)
	require.NoError(t, os.WriteFile(filepath.Join(s.Worktree, ".mise.toml"), []byte("[tools]\nbun = \"1.3.14\"\n"), 0644))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	resumed, err := sandbox.Prepare(ctx, s.Worktree, nil, nil, testSettings(t))

	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, resumed.Close()) })
	execInSandbox(t, resumed, "bun", "--version")
}

func TestProjectMiseEnvironmentIsTrustedBeforeLoading(t *testing.T) {
	worktree := newWorktree(t)
	config := "[env]\n_.path = [\"./bin\"]\n"
	require.NoError(t, os.WriteFile(filepath.Join(worktree, "mise.toml"), []byte(config), 0644))

	s := prepareSandbox(t, worktree)

	execInSandbox(t, s, "mise", "env")
}
