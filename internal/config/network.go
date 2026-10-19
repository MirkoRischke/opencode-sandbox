package config

import "strings"

// EffectiveAllowedHosts resolves the host patterns shared by the runtime policy
// and its guest-facing description. Unrestricted access ignores both host lists.
func (n NetworkSettings) EffectiveAllowedHosts() []string {
	if n.Unrestricted {
		return nil
	}
	var hosts []string
	seen := make(map[string]bool)
	for _, list := range [][]string{n.AllowedHosts, n.AdditionalAllowedHosts} {
		for _, host := range list {
			key := strings.ToLower(host)
			if !seen[key] {
				seen[key] = true
				hosts = append(hosts, host)
			}
		}
	}
	return hosts
}
