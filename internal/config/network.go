package config

import "strings"

// EffectiveAllowedHosts resolves the host patterns shared by the runtime policy
// and its guest-facing description. Unrestricted access ignores the allowlist.
func (n NetworkSettings) EffectiveAllowedHosts() []string {
	if n.Unrestricted {
		return nil
	}
	var hosts []string
	seen := make(map[string]bool)
	for _, host := range n.AllowedHosts {
		key := strings.ToLower(host)
		if !seen[key] {
			seen[key] = true
			hosts = append(hosts, host)
		}
	}
	return hosts
}
