package sandbox

import (
	"testing"

	"opencode-sandbox/internal/config"

	"github.com/stretchr/testify/require"
	m "github.com/superradcompany/microsandbox/sdk/go"
)

func TestNetworkPolicyCombinesAndDeduplicatesHostLists(t *testing.T) {
	t.Parallel()
	settings := config.NetworkSettings{
		AllowedHosts:           []string{"GitHub.com", "api.github.com", "github.com"},
		AdditionalAllowedHosts: []string{"API.GITHUB.COM", "registry.npmjs.org", "registry.npmjs.org"},
	}
	policy := networkPolicy(settings)
	require.Equal(t, m.PolicyActionDeny, policy.DefaultEgress)
	require.Equal(t, m.PolicyActionDeny, policy.DefaultIngress)
	// DNS and protected-destination rules remain unchanged and precede host rules.
	baseline := networkPolicy(config.NetworkSettings{})
	require.Equal(t, baseline.Rules, policy.Rules[:len(baseline.Rules)])
	var destinations []string
	for _, rule := range policy.Rules[len(baseline.Rules):] {
		require.Equal(t, m.PolicyActionAllow, rule.Action)
		require.Equal(t, m.PolicyDirectionEgress, rule.Direction)
		require.Equal(t, m.PolicyProtocolTCP, rule.Protocol)
		require.Equal(t, "443", rule.Port)
		destinations = append(destinations, rule.Destination)
	}
	require.Equal(t, []string{"github.com", "api.github.com", "registry.npmjs.org"}, destinations)
	require.Equal(t, "GitHub.com", settings.AllowedHosts[0], "policy must not mutate settings")
}

func TestNetworkPolicyAdditionalHostsWithoutBaseline(t *testing.T) {
	t.Parallel()
	settings := config.DefaultSettings()
	settings.Network.AdditionalAllowedHosts = []string{"registry.npmjs.org"}
	require.NoError(t, settings.Validate())
	policy := networkPolicy(settings.Network)
	require.Equal(t, "registry.npmjs.org", policy.Rules[len(policy.Rules)-1].Destination)
}

func TestUnrestrictedNetworkAllowsOnlyPublicDestinationsAndIgnoresHostLists(t *testing.T) {
	t.Parallel()
	settings := config.DefaultSettings().Network
	settings.Unrestricted = true
	policy := networkPolicy(settings)
	baseline := networkPolicy(config.DefaultSettings().Network)
	require.Equal(t, m.PolicyActionDeny, policy.DefaultEgress)
	require.Equal(t, m.PolicyActionDeny, policy.DefaultIngress)
	require.False(t, policy.DisableStrict)
	require.Equal(t, baseline.Rules, policy.Rules[:len(baseline.Rules)])
	require.Equal(t, append(baseline.Rules, m.PolicyRule{
		Action: m.PolicyActionAllow, Direction: m.PolicyDirectionEgress, Destination: "public",
	}), policy.Rules)
	settings.AllowedHosts = []string{"github.com"}
	settings.AdditionalAllowedHosts = []string{"registry.npmjs.org"}
	require.Equal(t, policy, networkPolicy(settings))
	settings.Unrestricted = false
	restricted := networkPolicy(settings)
	require.Equal(t, "github.com", restricted.Rules[len(baseline.Rules)].Destination)
	require.Equal(t, "registry.npmjs.org", restricted.Rules[len(baseline.Rules)+1].Destination)
}

func TestNetworkDiagnosticsOnlyChangesDenialResponsesNotRules(t *testing.T) {
	t.Parallel()
	settings := config.DefaultSettings().Network
	settings.AllowedHosts = []string{"github.com"}
	baseline := networkPolicy(settings)
	require.Nil(t, baseline.HTTP)
	settings.Diagnostics = true
	policy := networkPolicy(settings)
	require.Equal(t, baseline.Rules, policy.Rules)
	require.Equal(t, baseline.DefaultEgress, policy.DefaultEgress)
	require.Equal(t, baseline.DefaultIngress, policy.DefaultIngress)
	require.NotNil(t, policy.HTTP)
	require.True(t, policy.HTTP.DenyResponse)
}
