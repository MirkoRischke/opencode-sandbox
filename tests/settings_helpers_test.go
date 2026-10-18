package sandbox_test

import (
	"os"
	"path/filepath"
	"testing"

	launcherconfig "opencode-sandbox/internal/config"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/require"
)

func testSettings(t *testing.T) launcherconfig.Settings {
	t.Helper()
	var settings launcherconfig.Settings
	_, err := toml.DecodeFile("../docs/config.example.toml", &settings)
	require.NoError(t, err)
	// The credential substitution test sends fake values to this echo service.
	settings.Network.AllowedHosts = append(settings.Network.AllowedHosts, "httpbingo.org")
	return settings
}

func writeSettingsAt(t *testing.T, configDir, contents string) {
	t.Helper()
	dir := filepath.Join(configDir, "opencode-sandbox")
	require.NoError(t, os.MkdirAll(dir, 0700))
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0600))
}
