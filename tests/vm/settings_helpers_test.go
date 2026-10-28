package sandbox_test

import (
	"os"
	"path/filepath"
	"testing"

	launcherconfig "opencode-sandbox/internal/config"

	"github.com/stretchr/testify/require"
)

func testSettings(t *testing.T, hosts ...string) launcherconfig.Settings {
	t.Helper()
	settings := launcherconfig.DefaultSettings()
	// Tests opt in to their own destinations, independently of production defaults.
	settings.Network = launcherconfig.NetworkSettings{AllowedHosts: append([]string{}, hosts...)}
	return settings
}

func writeSettingsAt(t *testing.T, configDir, contents string) {
	t.Helper()
	dir := filepath.Join(configDir, "opencode-sandbox")
	require.NoError(t, os.MkdirAll(dir, 0700))
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0600))
}
