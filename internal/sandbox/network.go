package sandbox

import (
	"strings"

	"opencode-sandbox/internal/config"

	m "github.com/superradcompany/microsandbox/sdk/go"
)

func networkPolicy(n config.NetworkSettings) *m.NetworkConfig {
	policy := &m.NetworkConfig{
		DefaultEgress:  m.PolicyActionDeny,
		DefaultIngress: m.PolicyActionDeny,
		Rules:          []m.PolicyRule{m.Rule.AllowDNS()},
	}
	if n.Diagnostics {
		// Default-deny SYN resets are silent. The responder extracts TLS SNI
		// and emits a denial diagnostic without connecting to the destination.
		policy.HTTP = &m.HTTPConfig{DenyResponse: true}
	}
	// Keep local/protected destinations blocked even for an allowed hostname.
	// The gateway DNS exception must precede the host deny.
	for _, destination := range []string{"host", "private", "loopback", "link-local", "metadata", "multicast", "0.0.0.0", "::"} {
		policy.Rules = append(policy.Rules, m.PolicyRule{
			Action: m.PolicyActionDeny, Direction: m.PolicyDirectionEgress, Destination: destination,
		})
	}
	seen := make(map[string]bool)
	for _, hosts := range [][]string{n.AllowedHosts, n.AdditionalAllowedHosts} {
		for _, host := range hosts {
			host = strings.ToLower(host)
			if seen[host] {
				continue
			}
			seen[host] = true
			policy.Rules = append(policy.Rules, m.PolicyRule{
				Action: m.PolicyActionAllow, Direction: m.PolicyDirectionEgress,
				Destination: host, Protocol: m.PolicyProtocolTCP, Port: "443",
			})
		}
	}
	return policy
}
