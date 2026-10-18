package sandbox

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSandboxNameIdentifiesRepositoryAndWorktree(t *testing.T) {
	t.Parallel()
	name := sandboxName("/projects/worktree", "/projects/repo/.git")
	require.Regexp(t, `^oc-repo-worktree-[0-9a-f]{8}$`, name)
}

func TestSandboxNameNormalizesFolderNames(t *testing.T) {
	t.Parallel()
	name := sandboxName("/projects/VM Test", "/projects/Git_Ticket/.git")
	require.Regexp(t, `^oc-git-ticket-vm-test-[0-9a-f]{8}$`, name)
}

func TestSandboxNameFitsRuntimeLimitForLongFolderNames(t *testing.T) {
	t.Parallel()
	name := sandboxName(filepath.Join("/projects", strings.Repeat("tree", 30)),
		filepath.Join("/projects", strings.Repeat("repo", 30), ".git"))
	require.LessOrEqual(t, len(name), 128)
	require.Regexp(t, `^oc-[a-z0-9-]+-[0-9a-f]{8}$`, name)
}

func TestIdenticallyNamedWorktreesHaveDistinctSandboxNames(t *testing.T) {
	t.Parallel()
	first := sandboxName("/first/worktree", "/first/repo/.git")
	second := sandboxName("/second/worktree", "/second/repo/.git")
	require.Regexp(t, `^oc-repo-worktree-[0-9a-f]{8}$`, first)
	require.Regexp(t, `^oc-repo-worktree-[0-9a-f]{8}$`, second)
	require.NotEqual(t, first, second)
}
