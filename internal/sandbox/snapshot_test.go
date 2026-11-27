package sandbox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"opencode-sandbox/internal/workspace"

	"github.com/stretchr/testify/require"
	m "github.com/superradcompany/microsandbox/sdk/go"
)

func TestSnapshotIdentityIncludesReadableRepositoryAndStableLabels(t *testing.T) {
	t.Parallel()
	repo := snapshotIdentity("/projects/My_Project")
	require.Regexp(t, `^oc-workspace-my-project-[0-9a-f]{16}$`, repo.group)
	require.Equal(t, "My_Project", repo.labels()["workspace-name"])
	require.Equal(t, "/projects/My_Project", repo.labels()["workspace-path"])
	require.Equal(t, "My_Project", repo.labels()["repository-name"])
	require.Equal(t, repo.id, repo.labels()["repository-id"])
	require.Len(t, repo.id, 64)
	require.True(t, repo.owns(repo.labels()))
	legacy := repo.labels()
	delete(legacy, "repository-id")
	delete(legacy, "repository-name")
	require.True(t, repo.owns(legacy))
	_, managed := managedSnapshotWorkspace(repo.group, legacy)
	require.True(t, managed)
	require.False(t, repo.owns(map[string]string{"workspace-id": repo.id}))
	require.NotEqual(t, repo.group, snapshotIdentity("/other/My_Project").group)
	require.LessOrEqual(t, len(snapshotIdentity("/projects/"+strings.Repeat("long", 50)).group), 128)
}

func TestSnapshotDiscoverySharesLinkedWorktrees(t *testing.T) {
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
	first, err := resolveSnapshotWorkspace(context.Background(), repo)
	require.NoError(t, err)
	second, err := resolveSnapshotWorkspace(context.Background(), tree)
	require.NoError(t, err)
	require.Equal(t, first, second, "linked worktrees must select the same baseline and lock")
	repository, err := workspace.Resolve(context.Background(), repo)
	require.NoError(t, err)
	linked, err := workspace.Resolve(context.Background(), tree)
	require.NoError(t, err)
	require.NotEqual(t, repository.ID(), linked.ID(), "VM identities must remain worktree-specific")
	require.NotEqual(t, Name(repository), Name(linked))
	alias := filepath.Join(dir, "alias")
	require.NoError(t, os.Symlink(tree, alias))
	third, err := resolveSnapshotWorkspace(context.Background(), alias)
	require.NoError(t, err)
	require.Equal(t, second, third)
	aliasedWorktree, err := workspace.Resolve(t.Context(), alias)
	require.NoError(t, err)
	linkedLabels := workspaceLabels(aliasedWorktree, workspaceVMKind)
	require.Equal(t, map[string]string{
		"opencode-sandbox": "",
		"kind":             "workspace-vm",
		"repository-id":    repository.RepositoryID(),
		"repository-name":  "repo",
		"workspace-id":     linked.ID(),
		"workspace-name":   "tree",
		"workspace-path":   linked.Root(),
		"linked-worktree":  "true",
		"git-common-dir":   filepath.Join(repository.Root(), ".git"),
	}, linkedLabels)
	restored := restoredWorktreePatch(aliasedWorktree, nil)
	require.Equal(t, linkedLabels, restored.Labels)
	require.Empty(t, restored.LabelsRemove)
	linkedLabels["kind"] = "workspace-preparation"
	require.Equal(t, linkedLabels, workspaceLabels(aliasedWorktree, preparationVMKind))
	mainCheckout := restoredWorktreePatch(repository, nil)
	require.NotContains(t, mainCheckout.Labels, "linked-worktree")
	require.NotContains(t, mainCheckout.Labels, "git-common-dir")
	require.ElementsMatch(t, []string{"linked-worktree", "git-common-dir"}, mainCheckout.LabelsRemove)
	unrelated, err := resolveSnapshotWorkspace(context.Background(), t.TempDir())
	require.NoError(t, err)
	require.NotEqual(t, first.id, unrelated.id)
	clone := filepath.Join(dir, "clone")
	git("clone", repo, clone)
	independent, err := resolveSnapshotWorkspace(context.Background(), clone)
	require.NoError(t, err)
	require.NotEqual(t, first.id, independent.id, "independent clones must not share baselines")
}

func TestRestoredSecretsKeepValuesHostSideAndRetainDestinationScope(t *testing.T) {
	t.Parallel()
	requireTLS := true
	secret := Secret{entry: m.Secret.Env("GH_TOKEN", "synthetic-secret", m.SecretEnvOptions{
		Allow: []string{"github.com"}, Substitution: m.SecretSubstitution{HeaderFields: []string{"authorization"}},
		AllowPlaceholderFor: []string{"example.org"}, RequireTLSIdentity: &requireTLS,
	})}
	w, err := workspace.Resolve(t.Context(), t.TempDir())
	require.NoError(t, err)
	patch := restoredWorktreePatch(w, []Secret{secret})
	require.Equal(t, m.ModificationPolicyNextStart, patch.Policy)
	require.Equal(t, "/workspace", patch.Workdir)
	require.Equal(t, w.ID(), patch.Labels["workspace-id"])
	require.Empty(t, patch.Env, "never put resolved credentials in guest environment")
	require.Equal(t, "synthetic-secret", patch.Secrets["GH_TOKEN"].Value)
	require.Equal(t, "$MSB_GH_TOKEN", patch.Secrets["GH_TOKEN"].Placeholder)
	require.Equal(t, []string{"github.com"}, patch.Secrets["GH_TOKEN"].AllowedHosts)
	require.Equal(t, &secret.entry.Substitution, patch.Secrets["GH_TOKEN"].Substitution)
	require.Equal(t, secret.entry.Passthrough, patch.Secrets["GH_TOKEN"].AllowPlaceholderFor)
	require.Equal(t, secret.entry.RequireTLSIdentity, patch.Secrets["GH_TOKEN"].RequireTLSIdentity)
	require.Equal(t, secret.entry.ViolationAction, patch.Secrets["GH_TOKEN"].ViolationAction)
	require.Contains(t, patch.Labels, "opencode-sandbox")
}

func TestSnapshotLockSerializesOnlyTheSameRepository(t *testing.T) { //nolint:paralleltest // Concurrent subprocess tests can briefly inherit flock descriptors.
	dir := t.TempDir()
	repo := snapshotIdentity("/projects/repo/.git")
	unlock, err := repo.lockIn(dir)
	require.NoError(t, err)
	defer unlock()
	_, err = repo.lockIn(dir)
	require.ErrorContains(t, err, "another snapshot operation")
	require.ErrorIs(t, err, syscall.EWOULDBLOCK)
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

func TestManagedSnapshotRepositoryUsesStoredIdentity(t *testing.T) {
	t.Parallel()
	original := snapshotIdentity("/deleted/repository/.git")
	repo, ok := managedSnapshotWorkspace(original.group, original.labels())
	require.True(t, ok)
	require.Equal(t, original.id, repo.id)
	require.Equal(t, original.name, repo.name)
	require.Equal(t, original.group, repo.group)
	require.True(t, repo.owns(original.labels()))

	for _, key := range []string{"opencode-sandbox", "kind", "workspace-id"} {
		labels := original.labels()
		delete(labels, key)
		_, ok := managedSnapshotWorkspace(original.group, labels)
		require.False(t, ok, "must not delete snapshots missing %s", key)
	}
	labels := original.labels()
	labels["kind"] = "unrelated"
	_, ok = managedSnapshotWorkspace(original.group, labels)
	require.False(t, ok)
	_, ok = managedSnapshotWorkspace("unrelated-group", original.labels())
	require.False(t, ok)
}

func TestBulkSnapshotRemovalRespectsRepositoryLock(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	repo := snapshotIdentity("/deleted/repository/.git")
	unlock, err := repo.lockIn(dir)
	require.NoError(t, err)
	defer unlock()
	stored, ok := managedSnapshotWorkspace(repo.group, repo.labels())
	require.True(t, ok)
	require.ErrorContains(t, stored.removeLockedIn(t.Context(), dir), "another snapshot operation")
}
