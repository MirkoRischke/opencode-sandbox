package config

import (
	"fmt"
	"regexp"
	"strings"
)

const DefaultKeyringService = "opencode-sandbox"

type SecretSettings struct {
	File         string           `toml:"file" description:"Host credential file; relative to the config supplying this field. Use directory mode 700 and file mode 600. Do not commit credentials."`
	Keyring      *KeyringSettings `toml:"keyring,omitempty" description:"Use the OS keyring instead of file. An empty table uses service opencode-sandbox and the secret name as account; optional service/account fields override these defaults."`
	Env          string           `toml:"env" description:"Unique guest environment variable receiving a placeholder, not the real credential."`
	AllowedHosts []string         `toml:"allowed_hosts" description:"Hosts permitted to receive this credential: example.com matches exactly; *.example.com includes the base domain and all subdomains. Passed unchanged to Microsandbox. Network access must be allowed separately."`
	Headers      []string         `toml:"headers" description:"HTTP headers in which the host proxy may substitute the credential."`
	AllowHTTP    bool             `toml:"allow_http" description:"Allow substitution without verified TLS identity. Default false. Enable only for trusted HTTP routes such as the host-local gateway; credentials are sent in plaintext."`
}

// KeyringSettings identifies an OS-keyring entry without storing its value.
type KeyringSettings struct {
	Service string `toml:"service" description:"Keyring service; defaults to opencode-sandbox."`
	Account string `toml:"account" description:"Keyring account; defaults to the secret name."`
}

// Reference supplies defaults for omitted service and account identifiers.
func (k KeyringSettings) Reference(name string) (service, account string) {
	service, account = k.Service, k.Account
	if service == "" {
		service = DefaultKeyringService
	}
	if account == "" {
		account = name
	}
	return service, account
}

var envPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var headerPattern = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")

// ValidateSecrets checks named credential definitions without accessing their sources.
func ValidateSecrets(secrets map[string]SecretSettings) error {
	envs := map[string]bool{}
	for name, secret := range secrets {
		if err := validateSecretSource(name, secret); err != nil {
			return err
		}
		if envs[secret.Env] {
			return fmt.Errorf("duplicate secret environment variable %q", secret.Env)
		}
		envs[secret.Env] = true
		if err := validateSecretDestinations(name, secret); err != nil {
			return err
		}
	}
	return nil
}

func validateSecretSource(name string, secret SecretSettings) error {
	if (secret.File != "") == (secret.Keyring != nil) {
		return fmt.Errorf("secret %q requires exactly one source: file or keyring", name)
	}
	if !envPattern.MatchString(secret.Env) {
		return fmt.Errorf("secret %q requires a valid environment variable name", name)
	}
	if secret.Keyring == nil {
		return nil
	}
	service, account := secret.Keyring.Reference(name)
	for _, value := range []string{service, account} {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf("secret %q: keyring service/account must be non-empty and contain no newlines or NUL bytes", name)
		}
	}
	return nil
}

func validateSecretDestinations(name string, secret SecretSettings) error {
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
	return nil
}
