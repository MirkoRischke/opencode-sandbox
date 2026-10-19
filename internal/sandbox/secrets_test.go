package sandbox

import (
	"os"
	"path/filepath"
	"testing"

	"opencode-sandbox/internal/config"

	"github.com/stretchr/testify/require"
)

func TestLoadSecretsPreservesHostPatterns(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0700))
	path := filepath.Join(dir, "token")
	require.NoError(t, os.WriteFile(path, []byte("fake-token"), 0600))
	hosts := []string{"Example.com", "*.Example.com"}
	settings := config.DefaultSettings()
	settings.Secrets = map[string]config.SecretSettings{
		"test": {File: path, Env: "TOKEN", AllowedHosts: hosts, Headers: []string{"authorization"}},
	}
	secrets, err := LoadSecrets(settings)
	require.NoError(t, err)
	require.Len(t, secrets, 1)
	require.Equal(t, hosts, secrets[0].entry.Allow)
}
