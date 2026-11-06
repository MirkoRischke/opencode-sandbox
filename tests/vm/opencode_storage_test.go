package sandbox_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"opencode-sandbox/internal/sandbox"
	"opencode-sandbox/internal/workspace"

	"github.com/stretchr/testify/require"
)

// This verifies wiring and persistence, not the safety of concurrent SQLite
// writers across VMs. Stop each service before opening the DB in the other VM.
func TestOpenCodeHistorySurvivesVMAndWorktreeRemoval(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state) //nolint:forbidigo // Keep experimental shared storage isolated; Setenv prevents parallel execution.
	root := newWorktree(t)
	w, err := workspace.Resolve(t.Context(), root)
	require.NoError(t, err)
	first := prepareSandbox(t, root)
	const guestDatabase = "/var/lib/opencode-sandbox/opencode/opencode.db"
	require.Equal(t, guestDatabase, strings.TrimSpace(execInSandbox(t, first, "opencode", "debug", "paths", "db")))
	payload, err := json.Marshal(map[string]any{"title": "Persistent history", "location": map[string]string{"directory": root}})
	require.NoError(t, err)
	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(execInSandbox(t, first, "opencode", "api", "post", "/api/session", "--data", string(payload))), &created))
	require.NotEmpty(t, created.Data.ID)
	execInSandbox(t, first, "opencode", "service", "stop")
	hostDatabase := filepath.Join(state, "opencode-sandbox", "opencode", w.RepositoryID(), "opencode.db")
	require.FileExists(t, hostDatabase)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	require.NoError(t, sandbox.Remove(ctx, root))
	recreated := prepareSandbox(t, root)
	require.Contains(t, execInSandbox(t, recreated, "opencode", "api", "get", "/api/session/"+created.Data.ID), "Persistent history")
	execInSandbox(t, recreated, "opencode", "service", "stop")

	// The original baseline predates the session; restore must not roll it back.
	otherRoot := filepath.Join(filepath.Dir(root), "other-worktree")
	repo := filepath.Join(filepath.Dir(root), "repo")
	runGit(t, "-C", repo, "worktree", "add", "-b", "other", otherRoot)
	other := prepareSandbox(t, otherRoot)
	// Existing fixture cleanup resolves the original path; restore an empty
	// non-Git directory only during cleanup, after testing the missing worktree.
	t.Cleanup(func() { require.NoError(t, os.MkdirAll(root, 0700)) })
	require.NotEqual(t, recreated.Name, other.Name)
	require.Contains(t, execInSandbox(t, other, "opencode", "api", "get", "/api/session/"+created.Data.ID), "Persistent history")
	require.NoError(t, sandbox.Remove(ctx, root))
	require.NoError(t, sandbox.RemoveSnapshot(ctx, root))
	runGit(t, "-C", repo, "worktree", "remove", root)
	payload, err = json.Marshal(map[string]string{"directory": otherRoot})
	require.NoError(t, err)
	execInSandbox(t, other, "opencode", "api", "post", "/api/session/"+created.Data.ID+"/move", "--data", string(payload))
	require.Contains(t, execInSandbox(t, other, "opencode", "api", "get", "/api/session/"+created.Data.ID), otherRoot)
	execInSandbox(t, other, "opencode", "service", "stop")
	require.NoError(t, sandbox.Remove(ctx, otherRoot))
	require.NoError(t, sandbox.RemoveSnapshot(ctx, otherRoot))
	_, err = os.Stat(hostDatabase)
	require.NoError(t, err, "VM and snapshot removal must preserve host history")
}
