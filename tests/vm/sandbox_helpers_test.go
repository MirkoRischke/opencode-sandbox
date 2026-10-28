package sandbox_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	launcherconfig "opencode-sandbox/internal/config"
	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	m "github.com/superradcompany/microsandbox/sdk/go"
)

func newWorktree(t *testing.T) string {
	t.Helper()
	return newNamedWorktree(t, "repo", "worktree")
}

func newNamedWorktree(t *testing.T, repoName, worktreeName string) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp/opencode", "worktree-test-")
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, os.RemoveAll(dir)) })
	repo, worktree := filepath.Join(dir, repoName), filepath.Join(dir, worktreeName)
	runGit(t, "init", "--initial-branch=main", repo)
	runGit(t, "-C", repo, "-c", "user.name=Sandbox Test", "-c", "user.email=sandbox@example.invalid",
		"-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-m", "Initial commit")
	runGit(t, "-C", repo, "worktree", "add", "-b", "sandbox", worktree)
	// Some tests manage VM handles directly instead of using the shared cleanup.
	// First launches now also leave a managed snapshot that outlives those VMs.
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		assert.NoError(t, sandbox.RemoveSnapshot(ctx, worktree))
	})
	return worktree
}

func prepareSandbox(t *testing.T, worktree string) *sandbox.Sandbox {
	t.Helper()
	return prepareSandboxWithSettings(t, worktree, testSettings(t))
}

func prepareSandboxWithSettings(t *testing.T, worktree string, settings launcherconfig.Settings, diagnostics ...sandbox.DiagnosticsOptions) *sandbox.Sandbox {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	s, err := sandbox.Prepare(ctx, worktree, nil, settings, diagnostics...)
	require.NoError(t, err)
	registerSandboxCleanup(t, s)
	return s
}

// Destroy through a fresh handle: a test may close, stop, or recreate its VM.
func registerSandboxCleanup(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		handle, err := m.GetSandbox(ctx, s.Name)
		if !m.IsKind(err, m.ErrSandboxNotFound) && assert.NoError(t, err) {
			assert.NoError(t, handle.Destroy(ctx))
		}
		if err := s.Close(); !m.IsKind(err, m.ErrInvalidHandle) {
			assert.NoError(t, err)
		}
		assert.NoError(t, sandbox.RemoveSnapshot(ctx, s.Worktree))
	})
}

func startSandbox(t *testing.T) *sandbox.Sandbox {
	t.Helper()
	return prepareSandbox(t, newWorktree(t))
}

func runGit(t *testing.T, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "git", args...).CombinedOutput()
	require.NoError(t, err, "prepare Git fixture: %s", output)
}

func execInSandbox(t *testing.T, s *sandbox.Sandbox, command ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var output bytes.Buffer
	err := s.Exec(ctx, nil, &output, &output, command...)
	require.NoError(t, err, "execute %v: %s", command, output.String())
	return output.String()
}
