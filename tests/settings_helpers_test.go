package sandbox_test

import (
	"os"
	"path/filepath"
	"testing"

	"opencode-sandbox/internal/sandbox"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/require"
)

func testSettings(t *testing.T) sandbox.Settings {
	t.Helper()
	var settings sandbox.Settings
	_, err := toml.DecodeFile("../docs/config.example.toml", &settings)
	require.NoError(t, err)
	// The credential substitution test sends fake values to this echo service.
	settings.Network.AllowedHosts = append(settings.Network.AllowedHosts, "httpbingo.org")
	return settings
}

func writeSettings(t *testing.T, contents string) string {
	t.Helper()
	dir := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "opencode-sandbox")
	require.NoError(t, os.MkdirAll(dir, 0700))
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0600))
	return path
}
