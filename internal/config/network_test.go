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
	require.Equal(t, []string{"github.com", "api.github.com", "registry.npmjs.org"}, network.EffectiveAllowedHosts())
	require.Equal(t, "GitHub.com", network.AllowedHosts[0], "resolution must not mutate settings")
	network.Unrestricted = true
	require.Empty(t, network.EffectiveAllowedHosts())
	require.Empty(t, (NetworkSettings{}).EffectiveAllowedHosts())
}
