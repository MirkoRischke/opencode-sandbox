package sandbox_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/require"
)

func TestSandboxNameIdentifiesRepositoryAndWorktree(t *testing.T) {
	s := startSandbox(t)

	require.Regexp(t, `^oc-repo-worktree-[0-9a-f]{8}$`, s.Name)
}

func TestSandboxNameNormalizesFolderNames(t *testing.T) {
	worktree := newNamedWorktree(t, "Git_Ticket", "VM Test")

	s := prepareSandbox(t, worktree)

	require.Regexp(t, `^oc-git-ticket-vm-test-[0-9a-f]{8}$`, s.Name)
}

func TestSandboxNameFitsRuntimeLimitForLongFolderNames(t *testing.T) {
	worktree := newNamedWorktree(t, strings.Repeat("repo", 30), strings.Repeat("tree", 30))

	s := prepareSandbox(t, worktree)

	require.LessOrEqual(t, len(s.Name), 128)
	require.Regexp(t, `^oc-[a-z0-9-]+-[0-9a-f]{8}$`, s.Name)
}

func TestSandboxNameRemainsStableWhenBranchChanges(t *testing.T) {
	s := startSandbox(t)
	runGit(t, "-C", s.Worktree, "switch", "-c", "another-branch")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	resumed, err := sandbox.Prepare(ctx, s.Worktree, nil, nil)

	require.NoError(t, err)
	defer resumed.Close()
	require.Equal(t, s.Name, resumed.Name)
}

func TestIdenticallyNamedWorktreesHaveDistinctSandboxNames(t *testing.T) {
	first := startSandbox(t)
	second := startSandbox(t)

	require.Regexp(t, `^oc-repo-worktree-[0-9a-f]{8}$`, first.Name)
	require.Regexp(t, `^oc-repo-worktree-[0-9a-f]{8}$`, second.Name)
	require.NotEqual(t, first.Name, second.Name)
}
