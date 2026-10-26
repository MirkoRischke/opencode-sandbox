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
	// Protected-destination rules precede hostname allows.
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
	require.Equal(t, []string{"GitHub.com", "api.github.com", "registry.npmjs.org"}, destinations)
	require.Equal(t, "GitHub.com", settings.AllowedHosts[0], "policy must not mutate settings")
}

func TestNetworkPolicyAdditionalHostsWithoutBaseline(t *testing.T) {
	t.Parallel()
	settings := config.DefaultSettings()
	settings.Network.AllowedHosts = []string{}
	settings.Network.AdditionalAllowedHosts = []string{"registry.npmjs.org"}
	require.NoError(t, settings.Validate())
	policy := networkPolicy(settings.Network)
	require.Equal(t, "registry.npmjs.org", policy.Rules[len(policy.Rules)-1].Destination)
}

func TestNetworkPolicyPreservesDomainSuffix(t *testing.T) {
	t.Parallel()
	settings := config.DefaultSettings()
	settings.Network.AllowedHosts = []string{"Example.com", ".Example.com"}
	require.NoError(t, settings.Validate())
	policy := networkPolicy(settings.Network)
	require.Equal(t, "Example.com", policy.Rules[len(policy.Rules)-2].Destination)
	require.Equal(t, ".Example.com", policy.Rules[len(policy.Rules)-1].Destination)
}

func TestUnrestrictedNetworkAllowsOnlyPublicDestinationsAndIgnoresHostLists(t *testing.T) {
	t.Parallel()
	settings := config.DefaultSettings().Network
	settings.Unrestricted = true
	policy := networkPolicy(settings)
	baseline := []m.PolicyRule{m.Rule.AllowDNS()}
	for _, destination := range []string{"host", "private", "loopback", "link-local", "metadata", "multicast", "0.0.0.0", "::"} {
		baseline = append(baseline, m.PolicyRule{
			Action: m.PolicyActionDeny, Direction: m.PolicyDirectionEgress, Destination: destination,
		})
	}
	require.Equal(t, m.PolicyActionDeny, policy.DefaultEgress)
	require.Equal(t, m.PolicyActionDeny, policy.DefaultIngress)
	require.False(t, policy.DisableStrict)
	require.Equal(t, append(baseline, m.PolicyRule{
		Action: m.PolicyActionAllow, Direction: m.PolicyDirectionEgress, Destination: "public",
	}), policy.Rules)
	settings.AllowedHosts = []string{"github.com"}
	settings.AdditionalAllowedHosts = []string{"registry.npmjs.org"}
	require.Equal(t, policy, networkPolicy(settings))
	settings.Unrestricted = false
	restricted := networkPolicy(settings)
	restrictedBaseline := networkPolicy(config.NetworkSettings{})
	require.Equal(t, "github.com", restricted.Rules[len(restrictedBaseline.Rules)].Destination)
	require.Equal(t, "registry.npmjs.org", restricted.Rules[len(restrictedBaseline.Rules)+1].Destination)
}

func TestRestrictedNetworkFiltersDNSWithoutWeakeningProtectedDestinations(t *testing.T) {
	t.Parallel()
	policy := networkPolicy(config.NetworkSettings{AllowedHosts: []string{"github.com"}})
	require.Equal(t, m.PolicyActionDeny, policy.DefaultEgress)
	require.Equal(t, m.PolicyRule{
		Action: m.PolicyActionDeny, Direction: m.PolicyDirectionEgress, Destination: "host",
		Protocol: m.PolicyProtocolTCP, Port: "443",
	}, policy.Rules[0])
	for _, rule := range policy.Rules[1 : len(policy.Rules)-1] {
		require.Equal(t, m.PolicyActionDeny, rule.Action)
		require.Empty(t, rule.Protocol)
		require.Empty(t, rule.Port)
	}
	require.Equal(t, "github.com", policy.Rules[len(policy.Rules)-1].Destination)
	for _, rule := range networkPolicy(config.NetworkSettings{}).Rules {
		require.Equal(t, m.PolicyActionDeny, rule.Action, "empty allowlist must not grant DNS access")
	}
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

func TestPortMappingsAllowOnlyLoopbackIngressWithoutChangingEgress(t *testing.T) {
	t.Parallel()
	for _, unrestricted := range []bool{false, true} {
		settings := config.DefaultSettings().Network
		settings.Unrestricted = unrestricted
		baseline := networkPolicy(settings)
		policy := networkPolicy(settings, config.PortSettings{Host: 3000, Guest: 8080})
		require.Equal(t, m.PolicyActionDeny, policy.DefaultIngress)
		require.Equal(t, baseline.DefaultEgress, policy.DefaultEgress)
		require.Equal(t, baseline.Rules, policy.Rules[1:])
		require.Equal(t, m.PolicyRule{
			Action: m.PolicyActionAllow, Direction: m.PolicyDirectionIngress,
			Destination: "loopback", Protocol: m.PolicyProtocolTCP, Port: "8080",
		}, policy.Rules[0])
	}
}
