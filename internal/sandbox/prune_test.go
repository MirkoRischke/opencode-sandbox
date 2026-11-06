package sandbox

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"opencode-sandbox/internal/logging"
	"opencode-sandbox/internal/workspace"

	"github.com/stretchr/testify/require"
	m "github.com/superradcompany/microsandbox/sdk/go"
)

type pruneFixture struct {
	state     string
	current   workspace.Workspace
	deleted   workspace.Workspace
	unrelated workspace.Workspace
	resources pruneResources
	removed   []string
}

func newPruneFixture(t *testing.T) *pruneFixture {
	t.Helper()
	f := &pruneFixture{state: t.TempDir()}
	repo, tree := filepath.Join(t.TempDir(), "repo"), filepath.Join(t.TempDir(), "tree")
	runPruneGit(t, "init", "--initial-branch=main", repo)
	runPruneGit(t, "-C", repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-m", "initial")
	runPruneGit(t, "-C", repo, "worktree", "add", "-b", "other", tree)
	f.current = resolvePruneWorkspace(t, repo)
	f.deleted = resolvePruneWorkspace(t, tree)
	f.unrelated = resolvePruneWorkspace(t, t.TempDir())
	for _, w := range []workspace.Workspace{f.current, f.deleted, f.unrelated} {
		require.NoError(t, workspace.Track(f.state, w))
		for _, path := range f.dataPaths(w) {
			writePruneFile(t, path)
		}
		f.resources.vms = append(f.resources.vms, pruneVM{
			name: Name(w), workspaceID: w.ID(), remove: f.remover("VM " + w.ID()),
		})
		f.resources.snapshots = append(f.resources.snapshots, pruneSnapshot{
			workspace: snapshotIdentity(w.Root()), remove: f.remover("snapshot " + w.ID()),
		})
	}
	// A preparation VM is selected by its workspace label, not its random name.
	f.resources.vms = append(f.resources.vms, pruneVM{
		name: "oc-prepare-random", workspaceID: f.deleted.ID(), remove: f.remover("preparation"),
	})
	runPruneGit(t, "-C", repo, "worktree", "remove", tree)
	return f
}

func (f *pruneFixture) remover(name string) func(context.Context) error {
	return func(context.Context) error {
		f.removed = append(f.removed, name)
		return nil
	}
}

func (f *pruneFixture) dataPaths(w workspace.Workspace) []string {
	return []string{
		filepath.Join(f.state, "opencode-sandbox", "opencode", w.ID(), "opencode.db"),
		filepath.Join(logging.Directory(f.state, w.Root()), "launch.log"),
		diagnosticsPath(f.state, w.Root()),
	}
}

func (f *pruneFixture) assertDeleted(t *testing.T, w workspace.Workspace, deleted bool) {
	t.Helper()
	paths := append(f.dataPaths(w), workspace.RecordPath(f.state, w.ID()))
	for _, path := range paths {
		if deleted {
			require.NoFileExists(t, path)
		} else {
			require.FileExists(t, path)
		}
	}
	if deleted {
		require.Contains(t, f.removed, "VM "+w.ID())
		require.Contains(t, f.removed, "snapshot "+w.ID())
	} else {
		require.NotContains(t, f.removed, "VM "+w.ID())
		require.NotContains(t, f.removed, "snapshot "+w.ID())
	}
}

func TestPruneWorkspaceScopes(t *testing.T) {
	t.Parallel()
	for _, scopeName := range []string{"current", "repo", "all"} {
		t.Run(scopeName, func(t *testing.T) {
			t.Parallel()
			f := newPruneFixture(t)
			options := PruneOptions{Directory: f.current.Root(), StateDir: f.state}
			options.Repository = scopeName == "repo"
			options.All = scopeName == "all"
			scope, err := resolvePruneScope(context.Background(), options)
			require.NoError(t, err)
			plan, err := planWorkspacePrune(scope, f.resources)
			require.NoError(t, err)
			require.NotEmpty(t, plan.Targets())
			f.assertDeleted(t, f.current, false) // Preview has no side effects.
			trust := filepath.Join(f.state, "opencode-sandbox", "trust.json")
			source := filepath.Join(f.current.Root(), "source.go")
			writePruneFile(t, trust)
			writePruneFile(t, source)
			require.NoError(t, plan.Execute(context.Background()))
			f.assertDeleted(t, f.current, true)
			f.assertDeleted(t, f.deleted, scopeName != "current")
			f.assertDeleted(t, f.unrelated, scopeName == "all")
			require.FileExists(t, trust)
			require.FileExists(t, source)
			if scopeName != "current" {
				require.Contains(t, f.removed, "preparation")
			}
		})
	}
}

func TestPruneAttemptsTargetsAndPreservesDataOnRuntimeFailure(t *testing.T) {
	t.Parallel()
	failure := errors.New("destroy failed")
	var calls []string
	step := func(name string, err error) pruneStep {
		return pruneStep{name: name, run: func(context.Context) error {
			calls = append(calls, name)
			return err
		}}
	}
	plan := PrunePlan{
		runtime:  []pruneStep{step("VM1", failure), step("VM2", nil), step("snapshot", nil)},
		local:    []pruneStep{step("database", nil)},
		metadata: []pruneStep{step("ownership", nil)},
		shared:   []pruneStep{step("image", nil), step("cache", nil)},
	}
	require.ErrorIs(t, plan.Execute(context.Background()), failure)
	require.Equal(t, []string{"VM1", "VM2", "snapshot", "image", "cache"}, calls)
}

func TestPruneLocalFailurePreservesOwnership(t *testing.T) {
	t.Parallel()
	failure := errors.New("permission denied")
	plan := PrunePlan{
		local: []pruneStep{{name: "database", run: func(context.Context) error { return failure }}},
		metadata: []pruneStep{{name: "ownership", run: func(context.Context) error {
			t.Fatal("ownership must survive failed deletion")
			return nil
		}}},
	}
	require.ErrorIs(t, plan.Execute(context.Background()), failure)
}

func TestPruneScopeValidation(t *testing.T) {
	t.Parallel()
	_, err := resolvePruneScope(context.Background(), PruneOptions{All: true, Repository: true})
	require.ErrorContains(t, err, "mutually exclusive")
	_, err = resolvePruneScope(context.Background(), PruneOptions{Directory: t.TempDir(), Repository: true})
	require.ErrorContains(t, err, "requires a Git")
	scope, err := resolvePruneScope(context.Background(), PruneOptions{Directory: "/missing", All: true})
	require.NoError(t, err)
	require.True(t, scope.all, "global cleanup must not require a valid current directory")
}

func TestPruneDoesNotFollowDataSymlinks(t *testing.T) {
	t.Parallel()
	state, outside := t.TempDir(), t.TempDir()
	sentinel := filepath.Join(outside, "keep")
	writePruneFile(t, sentinel)
	link := filepath.Join(state, "logs")
	require.NoError(t, os.Symlink(outside, link))
	steps, err := prunePaths([]string{link})
	require.NoError(t, err)
	require.NoError(t, runPruneSteps(context.Background(), steps))
	require.FileExists(t, sentinel)
	_, err = os.Lstat(link)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestPruneCacheRetentionIsReported(t *testing.T) {
	t.Parallel()
	require.NoError(t, cachePruneErrors(&m.StoragePruneReport{Entries: []m.MemoryCacheEntry{{State: "removed"}}}))
	err := cachePruneErrors(&m.StoragePruneReport{
		Entries: []m.MemoryCacheEntry{{Path: "active.ram", State: "in_use"}}, Truncated: true,
	})
	require.ErrorContains(t, err, "active.ram")
	require.ErrorContains(t, err, "in_use")
	require.ErrorContains(t, err, "truncated")
}

func resolvePruneWorkspace(t *testing.T, path string) workspace.Workspace {
	t.Helper()
	w, err := workspace.Resolve(context.Background(), path)
	require.NoError(t, err)
	return w
}

func runPruneGit(t *testing.T, args ...string) {
	t.Helper()
	output, err := exec.Command("git", args...).CombinedOutput()
	require.NoError(t, err, "%s", output)
}

func writePruneFile(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	require.NoError(t, os.WriteFile(path, []byte("data"), 0600))
}
