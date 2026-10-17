package sandbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

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
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("COLORTERM", "truecolor")
	t.Setenv("SANDBOX_HOST_ONLY", "must-not-cross-into-guest")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	require.NoError(t, s.Open(ctx))

	require.Equal(t, "xterm-256color\ntruecolor\n\n", execInSandbox(t, s, "cat", s.Worktree+"/terminal-env.txt"))
}
