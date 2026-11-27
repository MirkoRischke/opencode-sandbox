package sandbox_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"opencode-sandbox/internal/config"
	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/require"
)

const guestInstructions = "/etc/opencode-sandbox/agent-sandbox-instructions.md"

func TestSandboxInstructionsDescribeGuestFilesAndSurviveReconnect(t *testing.T) {
	t.Parallel()
	source := filepath.Join(t.TempDir(), "settings.toml")
	const target = "/root/.config/test-agent/settings.toml"
	const userConfig = "preference = 'keep this content'\n"
	require.NoError(t, os.WriteFile(source, []byte(userConfig), 0600))
	settings := testSettings(t)
	settings.Network.AllowedHosts = []string{"GitHub.com", "github.com", "api.github.com"}
	settings.Files = []config.FileSettings{{Source: source, Target: target}}
	s := prepareSandboxWithSettings(t, newWorktree(t), settings)
	instructions := execInSandbox(t, s, "cat", guestInstructions)
	require.Contains(t, instructions, "- GitHub.com\n")
	require.NotContains(t, instructions, "- github.com\n")
	require.Contains(t, instructions, "- api.github.com\n")
	require.Contains(t, instructions, `"`+target+`"`)
	require.NotContains(t, instructions, source)
	require.Equal(t, userConfig, execInSandbox(t, s, "cat", target))
	// A modified file proves reconnect does not even rewrite identical instructions.
	execInSandbox(t, s, "bash", "-eu", "-c", `printf '\nretained on reconnect\n' >> "$1"`, "test", guestInstructions)
	instructions = execInSandbox(t, s, "cat", guestInstructions)
	require.NoError(t, s.Shutdown())
	settings.Network.Unrestricted = true
	settings.Files = nil
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	resumed, err := sandbox.Prepare(ctx, s.Worktree, nil, settings)
	require.NoError(t, err)
	defer func() { require.NoError(t, resumed.Shutdown()) }()
	require.Equal(t, instructions, execInSandbox(t, resumed, "cat", guestInstructions))
	require.Equal(t, userConfig, execInSandbox(t, resumed, "cat", target))
}
