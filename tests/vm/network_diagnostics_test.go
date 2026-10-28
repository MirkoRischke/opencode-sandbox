package sandbox_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNetworkDiagnosticsPreservesWorktreeHistoryAcrossVMRecreation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	state, root := t.TempDir(), newWorktree(t)
	diagnostics := sandbox.DiagnosticsOptions{StateDir: state}
	settings := testSettings(t)
	settings.Network.Diagnostics = true
	first := prepareSandboxWithSettings(t, root, settings, diagnostics)
	requireNetworkBlocked(t, first, "https://example.org")
	require.NoError(t, first.Stop(ctx))
	_, before := readSavedDiagnostics(t, root, state)
	require.Equal(t, root, before.Worktree)
	previous := before.exampleOrgObservations()
	require.Positive(t, previous)
	require.NoError(t, first.Remove(ctx))

	settings.Network.AllowedHosts = []string{"github.com"}
	recreated := prepareSandboxWithSettings(t, root, settings, diagnostics)
	execInSandbox(t, recreated, "curl", "--noproxy", "*", "--fail", "--silent", "--show-error", "--max-time", "15", "https://github.com")
	requireNetworkBlocked(t, recreated, "http://169.254.169.254")
	requireNetworkBlocked(t, recreated, "https://example.org")
	require.NoError(t, recreated.Stop(ctx))

	_, after := readSavedDiagnostics(t, root, state)
	require.Equal(t, root, after.Worktree)
	require.Greater(t, after.exampleOrgObservations(), previous, "new denials must merge with the previous VM's history")
}

func TestNetworkDiagnosticsDoesNotReplayLogsWhenResumingOrDeletingVM(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	state, root := t.TempDir(), newWorktree(t)
	settings := testSettings(t)
	settings.Network.Diagnostics = true
	diagnostics := sandbox.DiagnosticsOptions{StateDir: state}
	original := prepareSandboxWithSettings(t, root, settings, diagnostics)
	requireNetworkBlocked(t, original, "https://example.org")
	require.NoError(t, original.Stop(ctx))
	before, overview := readSavedDiagnostics(t, root, state)
	require.Positive(t, overview.exampleOrgObservations())
	require.NoError(t, original.Close())

	resumed := prepareSandboxWithSettings(t, root, settings, diagnostics)
	require.NoError(t, resumed.Remove(ctx))

	after, _ := readSavedDiagnostics(t, root, state)
	require.Equal(t, string(before), string(after), "resuming must not replay historical raw logs; removal must preserve the saved overview")
}

type diagnosticOverview struct {
	Worktree string `json:"worktree"`
	Targets  []struct {
		Hostname     string `json:"hostname"`
		Observations uint64 `json:"observations"`
	} `json:"denied_targets"`
}

func (o diagnosticOverview) exampleOrgObservations() uint64 {
	var count uint64
	for _, target := range o.Targets {
		if target.Hostname == "example.org" {
			count += target.Observations
		}
	}
	return count
}

func readSavedDiagnostics(t *testing.T, root, state string) ([]byte, diagnosticOverview) {
	t.Helper()
	var output bytes.Buffer
	require.NoError(t, sandbox.WriteNetworkDiagnostics(t.Context(), root, state, &output))
	var stored diagnosticOverview
	require.NoError(t, json.Unmarshal(output.Bytes(), &stored))
	return output.Bytes(), stored
}

func TestSnapshotRestoreBlocksHTTPSWithDenialDiagnostics(t *testing.T) {
	t.Parallel()
	state := t.TempDir()
	settings := testSettings(t)
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
	require.True(t, restoredConfig.Network.HTTP.DenyResponse)
	// Bypass DNS so this exercises the HTTPS responder rather than DNS denial.
	err = s.Exec(ctx, nil, nil, nil, "curl", "--noproxy", "*", "--fail", "--silent", "--show-error",
		"--max-time", "5", "--resolve", "example.org:443:1.1.1.1", "https://example.org")
	require.Error(t, err)
	require.NoError(t, ctx.Err(), "guest curl must fail, not the SDK deadline")
	require.NoError(t, s.Stop(ctx), "flush diagnostics before inspecting the report")
	_, stored := readSavedDiagnostics(t, root, state)
	require.Positive(t, stored.exampleOrgObservations(), "restored VMs must diagnose blocked HTTPS without a DNS query")
}
