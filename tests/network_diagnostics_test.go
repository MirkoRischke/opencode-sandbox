package sandbox_test

import (
	"bytes"
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
	t.Parallel()
	var captured bytes.Buffer
	state := t.TempDir()
	diagnostics := sandbox.DiagnosticsOptions{StateDir: state, Output: &captured}
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
		s, err := sandbox.Prepare(ctx, root, nil, settings, diagnostics)
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
		require.NotContains(t, captured.String(), "warning:", "diagnostics should collect and save without warnings")

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
		resumed, err := sandbox.Prepare(ctx, root, nil, settings, diagnostics)
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

func TestSnapshotRestoreBlocksHTTPSWithoutDenialDiagnostics(t *testing.T) {
	t.Parallel()
	state := t.TempDir()
	settings := launcherconfig.DefaultSettings()
	settings.Network.Diagnostics = true
	settings.Snapshot.Command = "true"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	root := newWorktree(t)
	t.Cleanup(func() { assert.NoError(t, sandbox.RemoveSnapshot(context.Background(), root)) })
	_, err := sandbox.CreateSnapshot(ctx, root, nil, settings, sandbox.DiagnosticsOptions{StateDir: state})
	require.NoError(t, err)
	s, err := sandbox.Prepare(ctx, root, nil, settings, sandbox.DiagnosticsOptions{StateDir: state})
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		assert.NoError(t, s.Remove(ctx))
	})
	var restoredConfig struct {
		Network struct {
			HTTP struct {
				DenyResponse bool `json:"deny_response"`
			} `json:"http"`
		} `json:"network"`
	}
	require.NoError(t, json.Unmarshal([]byte(storedSandboxConfig(t, s)), &restoredConfig))
	// Microsandbox v0.7.8 restore loses the denial responder, not enforcement.
	// Keep this expectation aligned with the documented snapshot limitation.
	require.False(t, restoredConfig.Network.HTTP.DenyResponse)
	requireNetworkBlocked(t, s, "https://example.org")
	require.NoError(t, s.Stop(ctx), "flush diagnostics before inspecting the report")
	var output bytes.Buffer
	require.NoError(t, sandbox.WriteNetworkDiagnostics(ctx, s.Worktree, state, &output))
	var stored struct {
		Targets []struct {
			Hostname string `json:"hostname"`
		} `json:"denied_targets"`
	}
	require.NoError(t, json.Unmarshal(output.Bytes(), &stored))
	for _, target := range stored.Targets {
		require.NotEqual(t, "example.org", target.Hostname, "restored VMs cannot diagnose blocked HTTPS without the denial responder")
	}
}
