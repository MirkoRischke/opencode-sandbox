package sandbox_test

import (
	"os"
	"path/filepath"
	"testing"

	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/require"
)

func TestHostTUIConfigIsCopiedWithoutOtherOpenCodeFiles(t *testing.T) {
	t.Parallel()
	dir, path := hostTUIConfigPath(t)
	config := []byte(`{"theme":{"name":"tokyonight","mode":"dark"},"mouse":false}`)
	require.NoError(t, os.WriteFile(path, config, 0600))
	for _, name := range []string{"opencode.json", "auth.json", "opencode.db"} {
		require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(path), name), []byte("host-only fixture"), 0600))
	}
	s := startSandbox(t)

	syncTUIConfig(t, s, dir)

	require.Equal(t, string(config), execInSandbox(t, s, "cat", "/root/.config/opencode/cli.json"))
	require.Equal(t, "600\n", execInSandbox(t, s, "stat", "-c", "%a", "/root/.config/opencode/cli.json"))
	for _, name := range []string{"opencode.json", "auth.json", "opencode.db"} {
		execInSandbox(t, s, "test", "!", "-e", "/root/.config/opencode/"+name)
	}
	execInSandbox(t, s, "bash", "-eu", "-c", `printf '{}' > /root/.config/opencode/cli.json`)
	hostConfig, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, config, hostConfig, "guest edits must not affect the host")
}

func TestTUIConfigIsRefreshedOnNextSync(t *testing.T) {
	t.Parallel()
	dir, path := hostTUIConfigPath(t)
	require.NoError(t, os.WriteFile(path, []byte(`{"mouse":false}`), 0600))
	s := startSandbox(t)
	syncTUIConfig(t, s, dir)
	updated := []byte(`{"mouse":true}`)
	require.NoError(t, os.WriteFile(path, updated, 0600))

	syncTUIConfig(t, s, dir)

	require.Equal(t, string(updated), execInSandbox(t, s, "cat", "/root/.config/opencode/cli.json"))
}

func TestMissingHostTUIConfigLeavesGuestDefaultsUnchanged(t *testing.T) {
	t.Parallel()
	dir, _ := hostTUIConfigPath(t)
	s := startSandbox(t)

	syncTUIConfig(t, s, dir)

	execInSandbox(t, s, "test", "!", "-e", "/root/.config/opencode/cli.json")
}

func TestRemovingHostTUIConfigClearsPreviousCopy(t *testing.T) {
	t.Parallel()
	dir, path := hostTUIConfigPath(t)
	require.NoError(t, os.WriteFile(path, []byte(`{"mouse":false}`), 0600))
	s := startSandbox(t)
	syncTUIConfig(t, s, dir)
	require.NoError(t, os.Remove(path))

	syncTUIConfig(t, s, dir)

	execInSandbox(t, s, "test", "!", "-e", "/root/.config/opencode/cli.json")
}

func TestMissingHostTUIConfigPreservesUnmanagedGuestConfig(t *testing.T) {
	t.Parallel()
	dir, _ := hostTUIConfigPath(t)
	s := startSandbox(t)
	execInSandbox(t, s, "bash", "-eu", "-c", `mkdir -p /root/.config/opencode; printf '{}' > /root/.config/opencode/cli.json`)

	syncTUIConfig(t, s, dir)

	require.Equal(t, "{}", execInSandbox(t, s, "cat", "/root/.config/opencode/cli.json"))
}

func TestUnreadableHostTUIConfigIsReported(t *testing.T) {
	t.Parallel()
	dir, path := hostTUIConfigPath(t)
	require.NoError(t, os.Mkdir(path, 0700))
	_, err := sandbox.LoadCLIConfig("", dir)

	require.ErrorContains(t, err, "read CLI config")
}
