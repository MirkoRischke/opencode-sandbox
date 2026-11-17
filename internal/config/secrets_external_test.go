package config_test

import (
	"testing"

	launcherconfig "opencode-sandbox/internal/config"

	"github.com/stretchr/testify/require"
)

func TestSecretSettingsRejectInvalidScopes(t *testing.T) {
	t.Parallel()
	for _, fields := range []string{
		"source.file = 'creds/token'", // missing environment name and scope
		"source.file = 'creds/token'\nguest.env = 'BAD-NAME'\nallowed_hosts = ['github.com']\nheaders = ['authorization']",
		"source.file = 'creds/token'\nguest.env = 'TOKEN'\nallowed_hosts = []\nheaders = ['authorization']",
		"source.file = 'creds/token'\nguest.env = 'TOKEN'\nallowed_hosts = ['.github.com']\nheaders = ['authorization']",
		"source.file = 'creds/token'\nguest.env = 'TOKEN'\nallowed_hosts = ['github.com']\nheaders = []",
		"source.file = 'creds/token'\nguest.env = 'TOKEN'\nallowed_hosts = ['github.com']\nheaders = ['bad header']",
		"source.file = 'creds/token'\nguest.env = 'TOKEN'\nallowed_hosts = ['github.com']\nheaders = ['authorization']\nvalue = 'inline-secrets-not-supported'",
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
		Source: launcherconfig.SecretSourceSettings{File: "unused"}, Guest: launcherconfig.SecretGuestSettings{Env: "GH_TOKEN"}, AllowedHosts: []string{"github.com"}, Headers: []string{"authorization"},
	}
	err := launcherconfig.ValidateSecrets(map[string]launcherconfig.SecretSettings{"github": secret, "other": secret})
	require.ErrorContains(t, err, "duplicate secret environment variable")
}
