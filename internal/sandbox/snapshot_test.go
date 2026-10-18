package sandbox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"opencode-sandbox/internal/config"

	"github.com/stretchr/testify/require"
	m "github.com/superradcompany/microsandbox/sdk/go"
)

func TestSnapshotIdentityIncludesReadableRepositoryAndStableLabels(t *testing.T) {
	t.Parallel()
	repo := snapshotIdentity("/projects/My_Project/.git")
	require.Regexp(t, `^oc-repo-my-project-[0-9a-f]{16}$`, repo.group)
	require.Equal(t, "My_Project", repo.labels()["repo-name"])
	require.Equal(t, "/projects/My_Project", repo.labels()["repo-path"])
	require.Len(t, repo.id, 64)
	require.True(t, repo.owns(repo.labels()))
	require.False(t, repo.owns(map[string]string{"repo-id": repo.id}))
	require.NotEqual(t, repo.group, snapshotIdentity("/other/My_Project/.git").group)
	require.LessOrEqual(t, len(snapshotIdentity("/projects/"+strings.Repeat("long", 50)+"/.git").group), 128)
}

func TestSnapshotDiscoverySharesIdentityAcrossLinkedWorktrees(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	repo, tree := filepath.Join(dir, "repo"), filepath.Join(dir, "tree")
	git := func(args ...string) {
		t.Helper()
		out, err := exec.Command("git", args...).CombinedOutput()
		require.NoError(t, err, "%s", out)
	}
	git("init", "--initial-branch=main", repo)
	git("-C", repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-m", "initial")
	git("-C", repo, "worktree", "add", "-b", "other", tree)
	first, err := snapshotRepo(context.Background(), repo)
	require.NoError(t, err)
	second, err := snapshotRepo(context.Background(), tree)
	require.NoError(t, err)
	require.Equal(t, first, second)
	alias := filepath.Join(dir, "alias")
	require.NoError(t, os.Symlink(tree, alias))
	third, err := snapshotRepo(context.Background(), alias)
	require.NoError(t, err)
	require.Equal(t, first, third)
}

func TestRestoredSecretsKeepValuesHostSideAndRetainDestinationScope(t *testing.T) {
	t.Parallel()
	secret := Secret{entry: m.Secret.Env("GH_TOKEN", "synthetic-secret", m.SecretEnvOptions{
		Allow: []string{"github.com"}, Substitution: m.SecretSubstitution{HeaderFields: []string{"authorization"}},
	})}
	patch := restoredWorktreePatch("/worktree", []Secret{secret})
	require.Equal(t, m.ModificationPolicyNextStart, patch.Policy)
	require.Equal(t, "/worktree", patch.Workdir)
	require.Empty(t, patch.Env, "never put resolved credentials in guest environment")
	require.Equal(t, "synthetic-secret", patch.Secrets["GH_TOKEN"].Value)
	require.Equal(t, "$MSB_GH_TOKEN", patch.Secrets["GH_TOKEN"].Placeholder)
	require.Equal(t, []string{"github.com"}, patch.Secrets["GH_TOKEN"].AllowedHosts)
	require.Contains(t, patch.Labels, "opencode-sandbox")
}

func TestSnapshotCreationRequiresExplicitPreparationCommand(t *testing.T) {
	t.Parallel()
	_, err := CreateSnapshot(context.Background(), "not-a-repository", nil, config.DefaultSettings())
	require.ErrorContains(t, err, "snapshot.command is required")
}

func TestSnapshotLockSerializesOnlyTheSameRepository(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	repo := snapshotIdentity("/projects/repo/.git")
	unlock, err := repo.lockIn(dir)
	require.NoError(t, err)
	defer unlock()
	_, err = repo.lockIn(dir)
	require.ErrorContains(t, err, "another snapshot operation")
	otherUnlock, err := snapshotIdentity("/projects/other/.git").lockIn(dir)
	require.NoError(t, err)
	otherUnlock()
	unlock()
	again, err := repo.lockIn(dir)
	require.NoError(t, err)
	again()
	entries, err := os.ReadDir(filepath.Join(dir, "opencode-sandbox", "snapshot-locks"))
	require.NoError(t, err)
	for _, entry := range entries {
		require.Equal(t, ".lock", filepath.Ext(entry.Name()), "no launcher snapshot index")
	}
}
