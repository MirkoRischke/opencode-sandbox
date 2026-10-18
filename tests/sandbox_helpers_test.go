package sandbox_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

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
	return worktree
}

func prepareSandbox(t *testing.T, worktree string) *sandbox.Sandbox {
	t.Helper()
	return prepareSandboxWithConfig(t, worktree, nil)
}

func prepareSandboxWithConfig(t *testing.T, worktree string, config []byte) *sandbox.Sandbox {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	s, err := sandbox.Prepare(ctx, worktree, config, nil, testSettings(t))
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		// A test may deliberately close/shut down its original SDK connection.
		// Destroy through a fresh management handle instead of that connection.
		handle, err := m.GetSandbox(ctx, s.Name)
		if assert.NoError(t, err) {
			assert.NoError(t, handle.Destroy(ctx))
		}
		if err := s.Close(); !m.IsKind(err, m.ErrInvalidHandle) {
			assert.NoError(t, err)
		}
	})
	return s
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
