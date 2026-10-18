package sandbox_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/require"
)

func TestSandboxCLIConfigReplacesHostCLIConfig(t *testing.T) {
	t.Parallel()
	dir, hostPath := hostTUIConfigPath(t)
	require.NoError(t, os.WriteFile(hostPath, []byte(`{"mouse":false}`), 0600))
	path := filepath.Join(dir, "opencode-sandbox", "cli.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	custom := []byte(`{"theme":{"name":"tokyonight","mode":"dark"}}`)
	require.NoError(t, os.WriteFile(path, custom, 0600))
	s := startSandbox(t)

	syncTUIConfig(t, s, dir)

	require.Equal(t, string(custom), execInSandbox(t, s, "cat", "/root/.config/opencode/cli.json"))
}

func TestExplicitCLIConfigReplacesSandboxAndHostSettings(t *testing.T) {
	t.Parallel()
	dir, hostPath := hostTUIConfigPath(t)
	require.NoError(t, os.WriteFile(hostPath, []byte(`{"mouse":false}`), 0600))
	sandboxPath := filepath.Join(dir, "opencode-sandbox", "cli.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(sandboxPath), 0700))
	require.NoError(t, os.WriteFile(sandboxPath, []byte(`{"animations":false}`), 0600))
	path := filepath.Join(t.TempDir(), "custom-cli.json")
	custom := []byte(`{"theme":{"name":"tokyonight","mode":"dark"}}`)
	require.NoError(t, os.WriteFile(path, custom, 0600))
	s := startSandbox(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	config, err := sandbox.LoadCLIConfig(path, dir)
	require.NoError(t, err)
	require.NoError(t, s.SyncTUIConfig(ctx, config))

	require.Equal(t, string(custom), execInSandbox(t, s, "cat", "/root/.config/opencode/cli.json"))
}

func TestInvalidCLIConfigDoesNotFallBackToHostSettings(t *testing.T) {
	t.Parallel()
	dir, hostPath := hostTUIConfigPath(t)
	require.NoError(t, os.WriteFile(hostPath, []byte(`{}`), 0600))
	path := filepath.Join(dir, "opencode-sandbox", "cli.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	for _, tc := range []struct {
		name     string
		contents string
	}{
		{name: "empty", contents: ""},
		{name: "malformed", contents: "{broken"},
		{name: "array", contents: "[]"},
		{name: "null", contents: "null"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, os.WriteFile(path, []byte(tc.contents), 0600))
			_, err := sandbox.LoadCLIConfig("", dir)
			require.ErrorContains(t, err, "CLI config must be a JSON object")
		})
	}
}

func TestMissingExplicitCLIConfigDoesNotFallBackToHostSettings(t *testing.T) {
	t.Parallel()
	dir, hostPath := hostTUIConfigPath(t)
	require.NoError(t, os.WriteFile(hostPath, []byte(`{}`), 0600))

	_, err := sandbox.LoadCLIConfig(filepath.Join(t.TempDir(), "missing.json"), dir)

	require.ErrorContains(t, err, "read CLI config")
}
