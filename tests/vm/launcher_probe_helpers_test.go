package sandbox_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"opencode-sandbox/internal/sandbox"
	"opencode-sandbox/internal/workspace"
	gateway "opencode-sandbox/llm-gateway"

	"github.com/stretchr/testify/require"
)

func newLauncherProbeWorktree(t *testing.T) string {
	t.Helper()
	worktree := newWorktree(t)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		require.NoError(t, sandbox.Remove(ctx, worktree))
	})
	return worktree
}

func launcherProbeVMName(t *testing.T, worktree string) string {
	t.Helper()
	w, err := workspace.Resolve(t.Context(), worktree)
	require.NoError(t, err)
	return sandbox.Name(w)
}

// Exercise the real launcher and terminal attachment without a provider or TUI.
func runLauncherProbe(t *testing.T, worktree string, env map[string]string, probe string, flags ...string) error {
	t.Helper()
	// These probes exercise launcher/guest behavior, not the shared host gateway.
	// Keep ordinary VM tests independent of a developer's Compose installation.
	bin := t.TempDir()
	docker, err := exec.LookPath("docker")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(bin, "docker"), []byte(fmt.Sprintf(`#!/bin/sh
if [ "$1" = compose ]; then printf 'litellm healthy\ndb healthy\n'; exit 0; fi
exec %q "$@"
`, docker)), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(worktree, "launcher-probe.sh"), []byte(probe), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(worktree, "launcher-startup"), []byte(`#!/bin/bash
bash -eu ./launcher-probe.sh
status=$?
printf '%s\n' "$status" > launcher-probe-status
printf 'exit %s\n' "$status" > /root/.bashrc
exit "$status"
`), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(worktree, ".opencode-sandbox.toml"), []byte(`
[startup]
command = ["./launcher-startup"]
`), 0600))
	args := append([]string{buildLauncher(t)}, flags...)
	var scriptArgs []string
	switch runtime.GOOS {
	case "linux":
		quoted := make([]string, len(args))
		for i, arg := range args {
			quoted[i] = "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
		}
		scriptArgs = []string{"-q", "-e", "-c", strings.Join(quoted, " "), "/dev/null"}
	case "darwin":
		scriptArgs = append([]string{"-q", "/dev/null"}, args...)
	default:
		t.Skip("launcher probes require a controlling terminal on Linux or macOS")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "script", scriptArgs...)
	command.Dir = worktree
	command.Env = os.Environ()
	dataDir := t.TempDir()
	// Preseed reusable credentials so ordinary launcher probes need no LiteLLM.
	w, err := workspace.Resolve(ctx, worktree)
	require.NoError(t, err)
	credentials := filepath.Join(gateway.New(dataDir).Directory(), "credentials")
	require.NoError(t, os.MkdirAll(credentials, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(credentials, "team-"+w.RepositoryID()), []byte("test-team"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(credentials, "key-"+sandbox.Name(w)), []byte("sk-test-placeholder"), 0600))
	command.Env = append(command.Env, "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "XDG_DATA_HOME="+dataDir)
	approveTestConfig(t, worktree, env["XDG_CONFIG_HOME"], env["XDG_STATE_HOME"])
	for key, value := range env {
		command.Env = append(command.Env, key+"="+value)
	}
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("launcher: %w: %s", err, output)
	}
	status, err := os.ReadFile(filepath.Join(worktree, "launcher-probe-status"))
	require.NoError(t, err, "the configured startup probe must run")
	require.Equal(t, "0\n", string(status))
	return nil
}
