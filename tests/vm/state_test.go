package sandbox_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"opencode-sandbox/internal/config"
	"opencode-sandbox/internal/sandbox"
	"opencode-sandbox/internal/workspace"

	"github.com/stretchr/testify/require"
)

func TestWorkspaceStateSurvivesVMRemoval(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state) //nolint:forbidigo // Keep host storage isolated; Setenv prevents parallel execution.
	root := newWorktree(t)
	w, err := workspace.Resolve(t.Context(), root)
	require.NoError(t, err)
	const target = "/var/lib/test-state"
	const name = "test-state"
	const marker = "persistent workspace state"
	settings := testSettings(t)
	settings.State = []config.StateSettings{{Name: name, Target: target}}
	first := prepareSandboxWithSettings(t, root, settings)
	execInSandbox(t, first, "sh", "-ec", `printf '%s' "$1" > "$2"`, "test", marker, target+"/marker")
	content, err := os.ReadFile(filepath.Join(sandbox.WorkspaceStatePath(state, w), name, "marker"))
	require.NoError(t, err)
	require.Equal(t, marker, string(content))
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	require.NoError(t, sandbox.Remove(ctx, root))
	recreated := prepareSandboxWithSettings(t, root, settings)
	require.Equal(t, marker, execInSandbox(t, recreated, "cat", target+"/marker"))
}

func TestWorkspaceStateIsNotSharedWithLinkedWorktrees(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir()) //nolint:forbidigo // Isolate host state.
	const target = "/var/lib/test-state"
	settings := testSettings(t)
	settings.State = []config.StateSettings{{Name: "sessions", Target: target}}
	root := newWorktree(t)
	first := prepareSandboxWithSettings(t, root, settings)
	execInSandbox(t, first, "touch", target+"/marker")
	otherRoot := filepath.Join(filepath.Dir(root), "other-worktree")
	repo := filepath.Join(filepath.Dir(root), "repo")
	runGit(t, "-C", repo, "worktree", "add", "-b", "other", otherRoot)
	other := prepareSandboxWithSettings(t, otherRoot, settings)
	execInSandbox(t, other, "test", "!", "-e", target+"/marker")
}

func TestWorkspaceStateIsNotMountedDuringSnapshotPreparation(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir()) //nolint:forbidigo // Isolate host state.
	const target = "/var/lib/test-state"
	settings := testSettings(t)
	settings.State = []config.StateSettings{{Name: "sessions", Target: target}}
	settings.Snapshot.Command = "test ! -e " + target
	s := prepareSandboxWithSettings(t, newWorktree(t), settings)
	execInSandbox(t, s, "test", "-d", target)
}

func TestOpenCodeStateIsNotEnabledByDefault(t *testing.T) {
	t.Parallel()
	s := startSandbox(t)
	execInSandbox(t, s, "bash", "-eu", "-c", `test -z "${OPENCODE_DB+x}"; test ! -e /var/lib/opencode-sandbox/opencode`)
}
