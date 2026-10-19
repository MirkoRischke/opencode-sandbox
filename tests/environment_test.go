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

func TestSandboxInstructionsPreserveUserConfigAndSurviveReconnect(t *testing.T) {
	t.Parallel()
	source := filepath.Join(t.TempDir(), "opencode.json")
	const userConfig = `{"instructions":["/root/user-instructions.md"],"model":"openai/user-model"}`
	require.NoError(t, os.WriteFile(source, []byte(userConfig), 0600))
	settings := testSettings(t)
	settings.Network.AllowedHosts = []string{"GitHub.com"}
	settings.Network.AdditionalAllowedHosts = []string{"github.com", "api.github.com"}
	settings.Mounts = []config.MountSettings{{Source: source, Target: "/root/.config/opencode/opencode.json"}}
	s := prepareSandboxWithSettings(t, newWorktree(t), settings)
	instructions := execInSandbox(t, s, "cat", guestInstructions)
	require.Contains(t, instructions, "- GitHub.com\n")
	require.NotContains(t, instructions, "- github.com\n")
	require.Contains(t, instructions, "- api.github.com\n")
	require.Contains(t, instructions, `"/root/.config/opencode/opencode.json"`)
	require.NotContains(t, instructions, source)
	require.JSONEq(t, userConfig, execInSandbox(t, s, "cat", "/root/.config/opencode/opencode.json"))
	require.JSONEq(t, `{"$schema":"https://opencode.ai/config.json","instructions":["`+guestInstructions+`"]}`,
		execInSandbox(t, s, "cat", "/etc/opencode/opencode.json"))
	loadedConfig := execInSandbox(t, s, "opencode", "debug", "config")
	require.Contains(t, loadedConfig, "/etc/opencode/opencode.json")
	require.Contains(t, loadedConfig, "/root/.config/opencode/opencode.json")
	// A modified file proves reconnect does not even rewrite identical instructions.
	execInSandbox(t, s, "bash", "-eu", "-c", `printf '\nretained on reconnect\n' >> "$1"`, "test", guestInstructions)
	instructions = execInSandbox(t, s, "cat", guestInstructions)
	require.NoError(t, s.Shutdown())
	settings.Network.Unrestricted = true
	settings.Mounts = nil
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	resumed, err := sandbox.Prepare(ctx, s.Worktree, nil, settings)
	require.NoError(t, err)
	defer func() { require.NoError(t, resumed.Shutdown()) }()
	require.Equal(t, instructions, execInSandbox(t, resumed, "cat", guestInstructions))
	require.JSONEq(t, userConfig, execInSandbox(t, resumed, "cat", "/root/.config/opencode/opencode.json"))
}
