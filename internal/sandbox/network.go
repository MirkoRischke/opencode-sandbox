package sandbox

import (
	"strconv"

	"opencode-sandbox/internal/config"

	m "github.com/superradcompany/microsandbox/sdk/go"
)

func networkPolicy(n config.NetworkSettings) *m.NetworkConfig {
	policy := &m.NetworkConfig{
		DefaultEgress:  m.PolicyActionDeny,
		DefaultIngress: m.PolicyActionDeny,
	}
	for _, port := range n.HostToVM {
		policy.Rules = append(policy.Rules, m.PolicyRule{
			Action: m.PolicyActionAllow, Direction: m.PolicyDirectionIngress,
			Destination: "loopback", Protocol: m.PolicyProtocolTCP, Port: strconv.Itoa(int(port.VMPort)),
		})
	}
	// Explicit host-service permissions precede protected-destination denies.
	for _, port := range n.VMToHost.Ports {
		policy.Rules = append(policy.Rules, m.PolicyRule{
			Action: m.PolicyActionAllow, Direction: m.PolicyDirectionEgress,
			Destination: "host", Protocol: m.PolicyProtocolTCP, Port: strconv.Itoa(int(port)),
		})
	}
	if n.Unrestricted {
		policy.Rules = append(policy.Rules, m.Rule.AllowDNS())
	}
	if n.Diagnostics {
		// Default-deny SYN resets are silent. The responder extracts TLS SNI
		// and emits a denial diagnostic without connecting to the destination.
		policy.HTTP = &m.HTTPConfig{DenyResponse: true}
	}
	// Keep local/protected destinations blocked even for an allowed hostname.
	// In restricted mode, only TCP/443 can be allowed by the rules below.
	// Scope the host deny accordingly: a blanket host rule also matches every
	// DNS query, preventing the hostname rules from filtering query names.
	for _, destination := range []string{"host", "private", "loopback", "link-local", "metadata", "multicast", "0.0.0.0", "::"} {
		rule := m.PolicyRule{
			Action: m.PolicyActionDeny, Direction: m.PolicyDirectionEgress, Destination: destination,
		}
		if destination == "host" && !n.Unrestricted {
			rule.Protocol = m.PolicyProtocolTCP
			rule.Port = "443"
		}
		policy.Rules = append(policy.Rules, rule)
	}
	if n.Unrestricted {
		policy.Rules = append(policy.Rules, m.PolicyRule{
			Action: m.PolicyActionAllow, Direction: m.PolicyDirectionEgress, Destination: "public",
		})
		return policy
	}
	for _, host := range n.EffectiveAllowedHosts() {
		policy.Rules = append(policy.Rules, m.PolicyRule{
			Action: m.PolicyActionAllow, Direction: m.PolicyDirectionEgress,
			Destination: host, Protocol: m.PolicyProtocolTCP, Port: "443",
		})
	}
	return policy
}
