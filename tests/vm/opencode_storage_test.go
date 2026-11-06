package sandbox_test

import (
	"bytes"
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

func TestOpenCodeHistorySurvivesVMRemovalAndIsolatesWorktrees(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state) //nolint:forbidigo // Keep host storage isolated; Setenv prevents parallel execution.
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
	hostDatabase := filepath.Join(state, "opencode-sandbox", "opencode", w.ID(), "opencode.db")
	require.FileExists(t, hostDatabase)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	require.NoError(t, sandbox.Remove(ctx, root))
	recreated := prepareSandbox(t, root)
	require.Contains(t, execInSandbox(t, recreated, "opencode", "api", "get", "/api/session/"+created.Data.ID), "Persistent history")
	execInSandbox(t, recreated, "opencode", "service", "stop")

	// A linked worktree gets a different database, even in the same repository.
	otherRoot := filepath.Join(filepath.Dir(root), "other-worktree")
	repo := filepath.Join(filepath.Dir(root), "repo")
	runGit(t, "-C", repo, "worktree", "add", "-b", "other", otherRoot)
	other := prepareSandbox(t, otherRoot)
	require.NotEqual(t, recreated.Name, other.Name)
	var output bytes.Buffer
	err = other.Exec(ctx, nil, &output, &output, "opencode", "api", "get", "/api/session/"+created.Data.ID)
	require.Error(t, err)
	require.Contains(t, output.String(), "Session not found: "+created.Data.ID)
	otherWorkspace, err := workspace.Resolve(t.Context(), otherRoot)
	require.NoError(t, err)
	require.Equal(t, w.RepositoryID(), otherWorkspace.RepositoryID())
	require.NotEqual(t, w.ID(), otherWorkspace.ID())
	require.FileExists(t, filepath.Join(state, "opencode-sandbox", "opencode", otherWorkspace.ID(), "opencode.db"))
	execInSandbox(t, other, "opencode", "service", "stop")
	require.NoError(t, sandbox.Remove(ctx, otherRoot))
	require.NoError(t, sandbox.RemoveSnapshot(ctx, otherRoot))
	require.NoError(t, sandbox.Remove(ctx, root))
	require.NoError(t, sandbox.RemoveSnapshot(ctx, root))
	_, err = os.Stat(hostDatabase)
	require.NoError(t, err, "VM and snapshot removal must preserve host history")
}
