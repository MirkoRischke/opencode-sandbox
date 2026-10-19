package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEffectiveAllowedHosts(t *testing.T) {
	t.Parallel()
	network := NetworkSettings{
		AllowedHosts:           []string{"GitHub.com", "api.github.com", "github.com"},
		AdditionalAllowedHosts: []string{"API.GITHUB.COM", "registry.npmjs.org"},
	}
	require.Equal(t, []string{"GitHub.com", "api.github.com", "registry.npmjs.org"}, network.EffectiveAllowedHosts())
	require.Equal(t, "GitHub.com", network.AllowedHosts[0], "resolution must not mutate settings")
	network.Unrestricted = true
	require.Empty(t, network.EffectiveAllowedHosts())
	require.Empty(t, (NetworkSettings{}).EffectiveAllowedHosts())
}

func TestNetworkHostPatterns(t *testing.T) {
	t.Parallel()
	for _, host := range []string{"example.com", ".Example.com"} {
		settings := DefaultSettings()
		settings.Network.AllowedHosts = []string{host}
		settings.Network.AdditionalAllowedHosts = []string{host}
		require.NoError(t, settings.Validate(), host)
		require.Equal(t, []string{host}, settings.Network.EffectiveAllowedHosts())
	}
	for _, host := range []string{"*.example.com", "*", "..example.com", ".com", ".127.0.0.1", ".https://example.com", ".example.com:443"} {
		settings := DefaultSettings()
		settings.Network.AllowedHosts = []string{host}
		require.Error(t, settings.Validate(), host)
		settings.Network.AllowedHosts = []string{}
		settings.Network.AdditionalAllowedHosts = []string{host}
		require.Error(t, settings.Validate(), host)
	}
}

func TestSecretHostPatterns(t *testing.T) {
	t.Parallel()
	for _, host := range []string{"example.com", "*.Example.com", ".example.com", "*", "*.*.example.com", "*.127.0.0.1", "*.example.com:443", "https://example.com"} {
		secret := SecretSettings{File: "token", Env: "TOKEN", AllowedHosts: []string{host}, Headers: []string{"authorization"}}
		err := ValidateSecrets(map[string]SecretSettings{"test": secret})
		if host == "example.com" || host == "*.Example.com" {
			require.NoError(t, err, host)
		} else {
			require.Error(t, err, host)
		}
	}
}
