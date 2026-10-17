package sandbox

import (
	"strings"

	m "github.com/superradcompany/microsandbox/sdk/go"
)

func (n NetworkSettings) policy() *m.NetworkConfig {
	policy := &m.NetworkConfig{
		DefaultEgress:  m.PolicyActionDeny,
		DefaultIngress: m.PolicyActionDeny,
		Rules:          []m.PolicyRule{m.Rule.AllowDNS()},
	}
	// Keep local/protected destinations blocked even for an allowed hostname.
	// The gateway DNS exception must precede the host deny.
	for _, destination := range []string{"host", "private", "loopback", "link-local", "metadata", "multicast", "0.0.0.0", "::"} {
		policy.Rules = append(policy.Rules, m.PolicyRule{
			Action: m.PolicyActionDeny, Direction: m.PolicyDirectionEgress, Destination: destination,
		})
	}
	for _, host := range n.AllowedHosts {
		policy.Rules = append(policy.Rules, m.PolicyRule{
			Action: m.PolicyActionAllow, Direction: m.PolicyDirectionEgress,
			Destination: strings.ToLower(host), Protocol: m.PolicyProtocolTCP, Port: "443",
		})
	}
	return policy
}
