package sandbox_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	launcherconfig "opencode-sandbox/internal/config"
	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNetworkDiagnosticsPreservesWorktreeHistoryAcrossVMRecreation(t *testing.T) {
	// Sequential: replaces process-wide stderr and the state-directory environment.
	stderr := os.Stderr
	captured, err := os.CreateTemp(t.TempDir(), "diagnostics-stderr")
	require.NoError(t, err)
	os.Stderr = captured
	t.Cleanup(func() {
		os.Stderr = stderr
		require.NoError(t, captured.Close())
	})
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	root := newWorktree(t)
	settings := launcherconfig.DefaultSettings()
	settings.Network.Diagnostics = true
	var current *sandbox.Sandbox
	t.Cleanup(func() {
		if current != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			assert.NoError(t, current.Remove(ctx))
		}
	})
	type overview struct {
		Worktree string `json:"worktree"`
		Targets  []struct {
			Hostname     string `json:"hostname"`
			Observations uint64 `json:"observations"`
		} `json:"denied_targets"`
	}
	var previous uint64
	for iteration := range 2 {
		if iteration == 1 {
			settings.Network.AllowedHosts = []string{"github.com"}
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		s, err := sandbox.Prepare(ctx, root, nil, nil, settings)
		cancel()
		require.NoError(t, err)
		current = s
		if iteration == 1 {
			execInSandbox(t, s, "curl", "--noproxy", "*", "--fail", "--silent", "--show-error", "--max-time", "15", "https://github.com")
			requireNetworkBlocked(t, s, "http://169.254.169.254")
		}
		requireNetworkBlocked(t, s, "https://example.org")
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		err = s.Stop(ctx)
		cancel()
		require.NoError(t, err)
		messages, err := os.ReadFile(captured.Name())
		require.NoError(t, err)
		require.NotContains(t, string(messages), "warning:", "diagnostics should collect and save without warnings")

		identity := sha256.Sum256([]byte(s.Worktree))
		path := filepath.Join(state, "opencode-sandbox", "diagnostics", fmt.Sprintf("%x", identity[:16]), "network.json")
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		var stored overview
		require.NoError(t, json.Unmarshal(data, &stored))
		require.Equal(t, s.Worktree, stored.Worktree)
		var count uint64
		for _, target := range stored.Targets {
			if target.Hostname == "example.org" {
				count += target.Observations
			}
		}
		require.Greater(t, count, previous, "new VM's denials should merge with the previous VM's history")
		previous = count
		require.NoError(t, s.Close())

		// Resume the same VM without making requests. Old raw logs must not be
		// counted again, and deleting the VM must preserve the overview.
		ctx, cancel = context.WithTimeout(context.Background(), time.Minute)
		resumed, err := sandbox.Prepare(ctx, root, nil, nil, settings)
		cancel()
		require.NoError(t, err)
		current = resumed
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		err = resumed.Remove(ctx)
		cancel()
		require.NoError(t, err)
		current = nil
		after, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, string(data), string(after), "resuming must not replay historical raw logs")
	}
}
