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

	"github.com/stretchr/testify/require"
)

// Run the real CLI without live auth/inference. The guest probe stands in for
// OpenCode and carries its exit status through the fallback interactive shell.
func prepareLauncherProbe(t *testing.T) *sandbox.Sandbox {
	t.Helper()
	s := startSandboxWithSettings(t, testSettings(t))
	execInSandbox(t, s, "bash", "-eu", "-c", `
cat > /mise/data/shims/opencode <<'PROBE'
#!/bin/bash
# mise inspects tool versions during Prepare, before the interactive launch.
if [ "$#" -gt 0 ]; then exit 0; fi
bash -eu "$PWD/launcher-probe.sh"
status=$?
printf '%s\n' "$status" > "$PWD/launcher-probe-status"
exit "$status"
PROBE
chmod 755 /mise/data/shims/opencode
printf 'exit "$(cat "$PWD/launcher-probe-status")"\n' > /root/.bashrc`)
	return s
}

func runLauncherProbe(t *testing.T, s *sandbox.Sandbox, env map[string]string, probe string, flags ...string) error {
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
	require.NoError(t, os.WriteFile(filepath.Join(s.Worktree, "launcher-probe.sh"), []byte(probe), 0600))
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
	command.Dir = s.Worktree
	command.Env = os.Environ()
	command.Env = append(command.Env, "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "XDG_DATA_HOME="+t.TempDir())
	for key, value := range env {
		command.Env = append(command.Env, key+"="+value)
	}
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("launcher: %w: %s", err, output)
	}
	return nil
}
