package sandbox_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"opencode-sandbox/internal/config"
	"opencode-sandbox/internal/sandbox"
	"opencode-sandbox/internal/workspace"

	"github.com/stretchr/testify/require"
)

func TestOptInOpenCodeHistorySurvivesVMRemoval(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state) //nolint:forbidigo // Keep host storage isolated; Setenv prevents parallel execution.
	root := newWorktree(t)
	w, err := workspace.Resolve(t.Context(), root)
	require.NoError(t, err)
	const guestDirectory = "/var/lib/opencode-sandbox/opencode"
	const guestDatabase = "/var/lib/opencode-sandbox/opencode/opencode.db"
	settings := testSettings(t)
	settings.State = []config.StateSettings{{Name: "opencode-sessions", Target: guestDirectory}}
	first := prepareSandboxWithSettings(t, root, settings)
	runOpenCode := func(s *sandbox.Sandbox, args ...string) string {
		return execInSandbox(t, s, append([]string{"env", "OPENCODE_DB=" + guestDatabase, "opencode"}, args...)...)
	}
	require.Equal(t, guestDatabase, strings.TrimSpace(runOpenCode(first, "debug", "paths", "db")))
	payload, err := json.Marshal(map[string]any{"title": "Persistent history", "location": map[string]string{"directory": root}})
	require.NoError(t, err)
	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(runOpenCode(first, "api", "post", "/api/session", "--data", string(payload))), &created))
	require.NotEmpty(t, created.Data.ID)
	runOpenCode(first, "service", "stop")
	hostDatabase := filepath.Join(state, "opencode-sandbox", "state", w.RepositoryID(), w.ID(), "opencode-sessions", "opencode.db")
	require.FileExists(t, hostDatabase)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	require.NoError(t, sandbox.Remove(ctx, root))
	recreated := prepareSandboxWithSettings(t, root, settings)
	require.Contains(t, runOpenCode(recreated, "api", "get", "/api/session/"+created.Data.ID), "Persistent history")
	runOpenCode(recreated, "service", "stop")
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
