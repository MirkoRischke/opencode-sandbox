package config

import (
	"fmt"
	"regexp"
	"strings"
)

type SecretSettings struct {
	File         string   `toml:"file" description:"Host credential file; relative to the config supplying this field. Use directory mode 700 and file mode 600. Do not commit credentials."`
	Env          string   `toml:"env" description:"Unique guest environment variable receiving a placeholder, not the real credential."`
	AllowedHosts []string `toml:"allowed_hosts" description:"HTTPS hosts permitted to receive this credential: example.com matches exactly; *.example.com includes the base domain and all subdomains. Passed unchanged to Microsandbox. Network access must be allowed separately."`
	Headers      []string `toml:"headers" description:"HTTP headers in which the host proxy may substitute the credential."`
}

var envPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var headerPattern = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")

// ValidateSecrets checks named credential definitions without reading their files.
func ValidateSecrets(secrets map[string]SecretSettings) error {
	envs := map[string]bool{}
	for name, secret := range secrets {
		if secret.File == "" || !envPattern.MatchString(secret.Env) {
			return fmt.Errorf("secret %q requires a file and a valid environment variable name", name)
		}
		if envs[secret.Env] {
			return fmt.Errorf("duplicate secret environment variable %q", secret.Env)
		}
		envs[secret.Env] = true
		if len(secret.AllowedHosts) == 0 || len(secret.Headers) == 0 {
			return fmt.Errorf("secret %q requires non-empty allowed_hosts and headers", name)
		}
		for _, host := range secret.AllowedHosts {
			hostname := strings.TrimPrefix(host, "*.")
			if len(hostname) > 253 || !hostnamePattern.MatchString(hostname) {
				return fmt.Errorf("secret %q: allowed_hosts: %q must be a DNS hostname or *.prefixed wildcard", name, host)
			}
		}
		for _, header := range secret.Headers {
			if !headerPattern.MatchString(header) {
				return fmt.Errorf("secret %q has an invalid header name", name)
			}
		}
	}
	return nil
}
