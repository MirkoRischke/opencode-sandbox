package sandbox_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

//nolint:paralleltest // runInTerminal parallelizes the parent only.
func TestGuestTerminalReceivesOnlySelectedHostVariables(t *testing.T) {
	if runInTerminal(t, "TERM=xterm-256color", "COLORTERM=truecolor", "SANDBOX_HOST_ONLY=must-not-cross-into-guest") {
		return
	}
	s := startSandbox(t)
	execInSandbox(t, s, "bash", "-eu", "-c", `
cat > ./terminal-probe <<'PROBE'
#!/bin/bash
printf '%s\n' "${TERM-}" "${COLORTERM-}" "${SANDBOX_HOST_ONLY-}" > "$PWD/terminal-env.txt"
PROBE
chmod 755 ./terminal-probe
printf 'exit 0\n' > /root/.bashrc`)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	require.NoError(t, s.Open(ctx, []string{"./terminal-probe"}))

	require.Equal(t, "xterm-256color\ntruecolor\n\n", execInSandbox(t, s, "cat", s.Worktree+"/terminal-env.txt"))
}

//nolint:paralleltest // runInTerminal schedules the parent in parallel; its isolated child must not call Parallel again when exercising the controlling terminal.
func TestStartupExitFallsBackToInteractiveGuestBash(t *testing.T) {
	if runInTerminal(t) {
		return
	}
	s := startSandbox(t)
	for _, test := range []struct {
		name        string
		startupExit string
		shellExit   string
	}{
		{"success", "0", "0"},
		{"opencode failure", "7", "0"},
		{"shell failure", "0", "9"},
	} {
		t.Run(test.name, func(t *testing.T) {
			execInSandbox(t, s, "bash", "-eu", "-c", `
rm -f startup-ran shell-state.txt
printf '#!/bin/bash\ntouch startup-ran\nexit %s\n' "$1" > ./startup-probe
chmod 755 ./startup-probe
cat > /root/.bashrc <<'PROBE'
test -f startup-ran || exit 99
printf '%s\n' "$PWD" "$-" "${BASH_VERSION-}" > "$PWD/shell-state.txt"
PROBE
printf 'exit %s\n' "$2" >> /root/.bashrc`, "probe", test.startupExit, test.shellExit)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			err := s.Open(ctx, []string{"./startup-probe"})
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

//nolint:paralleltest // runInTerminal parallelizes the parent only.
func TestCustomStartupPassesLiteralArgumentsThenOpensBash(t *testing.T) {
	if runInTerminal(t) {
		return
	}
	s := startSandbox(t)
	execInSandbox(t, s, "bash", "-eu", "-c", `
cat > ./startup-probe <<'PROBE'
#!/bin/bash
printf '<%s>\n' "$@" > startup-args.txt
exit 7
PROBE
chmod +x ./startup-probe
cat > /root/.bashrc <<'PROBE'
[[ $- == *i* && -f startup-args.txt ]] || exit 1
exit 0
PROBE`)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	require.NoError(t, s.Open(ctx, []string{"./startup-probe", "--flag", "$HOME; *", ""}))
	require.Equal(t, "<--flag>\n<$HOME; *>\n<>\n", execInSandbox(t, s, "cat", "startup-args.txt"))
}
