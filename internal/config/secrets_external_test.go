package config_test

import (
	"testing"

	launcherconfig "opencode-sandbox/internal/config"

	"github.com/stretchr/testify/require"
)

func TestSecretSettingsRejectInvalidScopes(t *testing.T) {
	t.Parallel()
	for _, fields := range []string{
		"file = 'creds/token'", // missing environment name and scope
		"file = 'creds/token'\nenv = 'BAD-NAME'\nallowed_hosts = ['github.com']\nheaders = ['authorization']",
		"file = 'creds/token'\nenv = 'TOKEN'\nallowed_hosts = []\nheaders = ['authorization']",
		"file = 'creds/token'\nenv = 'TOKEN'\nallowed_hosts = ['.github.com']\nheaders = ['authorization']",
		"file = 'creds/token'\nenv = 'TOKEN'\nallowed_hosts = ['github.com']\nheaders = []",
		"file = 'creds/token'\nenv = 'TOKEN'\nallowed_hosts = ['github.com']\nheaders = ['bad header']",
		"file = 'creds/token'\nenv = 'TOKEN'\nallowed_hosts = ['github.com']\nheaders = ['authorization']\nvalue = 'inline-secrets-not-supported'",
	} {
		t.Run(fields, func(t *testing.T) {
			dir := t.TempDir()
			writeSettingsAt(t, dir, "[secrets.test]\n"+fields+"\n")
			_, err := launcherconfig.LoadSettings(dir)
			require.Error(t, err)
		})
	}
}

func TestSecretSettingsRejectDuplicateEnvironmentNames(t *testing.T) {
	t.Parallel()
	secret := launcherconfig.SecretSettings{
		File: "unused", Env: "GH_TOKEN", AllowedHosts: []string{"github.com"}, Headers: []string{"authorization"},
	}
	err := launcherconfig.ValidateSecrets(map[string]launcherconfig.SecretSettings{"github": secret, "other": secret})
	require.ErrorContains(t, err, "duplicate secret environment variable")
}
