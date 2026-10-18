package sandbox_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

//nolint:forbidigo // runInTerminal parallelizes the parent; only its isolated terminal child mutates the environment consumed by the real attachment path.
func TestGuestTUIReceivesOnlySelectedHostTerminalVariables(t *testing.T) {
	if runInTerminal(t) {
		return
	}
	s := startSandbox(t)
	// Replace OpenCode only inside this disposable VM, observing the same
	// interactive attachment used by the launcher without starting a real TUI.
	execInSandbox(t, s, "bash", "-eu", "-c", `
cat > /mise/data/shims/opencode <<'PROBE'
#!/bin/bash
printf '%s\n' "${TERM-}" "${COLORTERM-}" "${SANDBOX_HOST_ONLY-}" > "$PWD/terminal-env.txt"
PROBE
chmod 755 /mise/data/shims/opencode`)
	// Exit the fallback shell automatically in this terminal-environment probe.
	execInSandbox(t, s, "bash", "-c", `printf 'exit 0\n' > /root/.bashrc`)
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("COLORTERM", "truecolor")
	t.Setenv("SANDBOX_HOST_ONLY", "must-not-cross-into-guest")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	require.NoError(t, s.Open(ctx))

	require.Equal(t, "xterm-256color\ntruecolor\n\n", execInSandbox(t, s, "cat", s.Worktree+"/terminal-env.txt"))
}

//nolint:paralleltest // runInTerminal schedules the parent in parallel; its isolated child must not call Parallel again when exercising the controlling terminal.
func TestOpenCodeExitFallsBackToInteractiveGuestBash(t *testing.T) {
	if runInTerminal(t) {
		return
	}
	s := startSandbox(t)
	for _, test := range []struct {
		name         string
		opencodeExit string
		shellExit    string
	}{
		{"success", "0", "0"},
		{"opencode failure", "7", "0"},
		{"shell failure", "0", "9"},
	} {
		t.Run(test.name, func(t *testing.T) {
			execInSandbox(t, s, "bash", "-eu", "-c", `
printf '#!/bin/bash\nexit %s\n' "$1" > /mise/data/shims/opencode
chmod 755 /mise/data/shims/opencode
cat > /root/.bashrc <<'PROBE'
printf '%s\n' "$PWD" "$-" "${BASH_VERSION-}" > "$PWD/shell-state.txt"
PROBE
printf 'exit %s\n' "$2" >> /root/.bashrc`, "probe", test.opencodeExit, test.shellExit)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			err := s.Open(ctx)
			if test.shellExit == "0" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, "guest command exited with code 9")
			}
			state := execInSandbox(t, s, "cat", s.Worktree+"/shell-state.txt")
			lines := strings.Split(strings.TrimSpace(state), "\n")
			require.Len(t, lines, 3)
			require.Equal(t, s.Worktree, lines[0])
			require.Contains(t, lines[1], "i", "fallback Bash must be interactive")
			require.NotEmpty(t, lines[2], "fallback shell must be Bash")
		})
	}
}
