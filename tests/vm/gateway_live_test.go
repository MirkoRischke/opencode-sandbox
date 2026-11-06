//go:build live

package sandbox_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"opencode-sandbox/internal/config"
	"opencode-sandbox/internal/sandbox"
	"opencode-sandbox/internal/workspace"
	gateway "opencode-sandbox/llm-gateway"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/require"
)

// An authenticated installed gateway is the only live prerequisite. Test-only
// provider configuration is isolated from the user's launcher settings.
// The real launcher must also work with a stopped gateway; this test never stops
// the shared project during setup or cleanup.
func TestOpenCodePromptsThroughHostGateway(t *testing.T) {
	t.Parallel()
	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		home, err := os.UserHomeDir()
		require.NoError(t, err)
		dataDir = filepath.Join(home, ".local", "share")
	}
	// Require persisted provider setup; the launcher provisions its own virtual key.
	env, err := os.ReadFile(filepath.Join(gateway.New(dataDir).Directory(), ".env"))
	require.NoError(t, err, "authenticate once with 'opencode-sandbox gateway authenticate' first")
	model := os.Getenv("OPENCODE_TEST_MODEL")
	if model == "" {
		model = "chatgpt/gpt-6-luna"
	}
	worktree := newWorktree(t)
	w, err := workspace.Resolve(context.Background(), worktree)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, sandbox.Remove(context.Background(), worktree))
		require.NoError(t, gateway.New(dataDir).DeleteKey(context.Background(), sandbox.Name(w)))
	})
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0700))
	provider := map[string]any{
		"$schema": "https://opencode.ai/config.json",
		"model":   "gateway/smoke",
		"providers": map[string]any{"gateway": map[string]any{
			"name": "Host gateway", "env": []string{"LLM_GATEWAY_KEY"},
			"package": "@opencode/ai/providers/openai-compatible/responses",
			"settings": map[string]any{
				"baseURL": "http://host.microsandbox.internal:4000/v1",
				"apiKey":  "{env:LLM_GATEWAY_KEY}", "transport": "http",
			},
			"models": map[string]any{"smoke": map[string]any{"modelID": model}},
		}},
	}
	data, err := json.Marshal(provider)
	require.NoError(t, err)
	providerFile := filepath.Join(dir, "opencode.json")
	require.NoError(t, os.WriteFile(providerFile, data, 0600))
	settings := testSettings(t, "models.dev")
	settings.Network.VMToHost.Ports = []uint16{4000}
	settings.Files = []config.FileSettings{{Source: providerFile, Target: "/root/.config/opencode/opencode.json"}}
	// Automate only the terminal entry point. The wrapper invokes the actual
	// OpenCode binary with a prompt; provider transport and auth are not mocked.
	settings.Snapshot.Command = `
cat > /mise/data/shims/opencode <<'SMOKE'
#!/bin/bash
if [ "$#" -gt 0 ]; then exec /root/.opencode/bin/opencode "$@"; fi
/root/.opencode/bin/opencode run --format json --model gateway/smoke 'Reply exactly with gateway ok' > "$PWD/gateway-reply.json" 2> "$PWD/gateway-error.log"
status=$?
printf '%s\n' "$status" > "$PWD/gateway-status"
exit "$status"
SMOKE
chmod 755 /mise/data/shims/opencode
printf 'exit "$(cat "$PWD/gateway-status")"\n' > /root/.bashrc
`
	var encoded bytes.Buffer
	require.NoError(t, toml.NewEncoder(&encoded).Encode(settings))
	writeSettingsAt(t, dir, encoded.String())
	binary := buildLauncher(t)
	var args []string
	switch runtime.GOOS {
	case "linux":
		args = []string{"-q", "-e", "-c", "'" + strings.ReplaceAll(binary, "'", "'\"'\"'") + "'", "/dev/null"}
	case "darwin":
		args = []string{"-q", "/dev/null", binary}
	default:
		t.Fatal("live launcher test requires Linux or macOS")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "script", args...)
	command.Dir = worktree
	command.Env = append(os.Environ(), "XDG_CONFIG_HOME="+dir, "XDG_STATE_HOME="+t.TempDir(), "HERDR_ENV=")
	// Do not include raw provider output in failures; it can contain credentials.
	output, err := command.CombinedOutput()
	if err != nil {
		// Launcher diagnostics do not include the redirected provider response.
		// Still redact every known gateway credential before showing them.
		diagnostic := string(output)
		for _, line := range strings.Split(string(env), "\n") {
			if _, value, ok := strings.Cut(line, "="); ok && value != "" {
				diagnostic = strings.ReplaceAll(diagnostic, value, "<REDACTED>")
			}
		}
		t.Logf("launcher diagnostics: %s", diagnostic)
	}
	require.NoError(t, err, "live launcher failed (output suppressed; %d bytes); check gateway authentication/configuration", len(output))
	response, err := os.ReadFile(filepath.Join(worktree, "gateway-reply.json"))
	require.NoError(t, err)
	require.Equal(t, "gateway ok", gatewayReply(t, response))
}

func gatewayReply(t *testing.T, data []byte) string {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	var reply strings.Builder
	for {
		var event struct {
			Type string `json:"type"`
			Part struct {
				Text string `json:"text"`
			} `json:"part"`
		}
		err := decoder.Decode(&event)
		if err == io.EOF {
			break
		}
		require.NoError(t, err, "decode OpenCode reply")
		require.NotEqual(t, "error", event.Type, "OpenCode reported a provider error")
		if event.Type == "text" {
			reply.WriteString(event.Part.Text)
		}
	}
	return strings.TrimSpace(reply.String())
}
