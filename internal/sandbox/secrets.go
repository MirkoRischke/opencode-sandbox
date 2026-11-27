package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"opencode-sandbox/internal/config"

	m "github.com/superradcompany/microsandbox/sdk/go"
	"github.com/zalando/go-keyring"
)

// Secret contains a resolved host credential. Never log it or expose it to guests.
type Secret struct {
	entry m.SecretEntry
}

func (s Secret) modifySpec() m.SecretModifySpec {
	entry := s.entry
	placeholder := entry.Placeholder
	if placeholder == "" {
		placeholder = "$MSB_" + entry.EnvVar
	}
	return m.SecretModifySpec{
		Value: entry.Value, Placeholder: placeholder, AllowedHosts: entry.Allow,
		Substitution: &entry.Substitution, AllowPlaceholderFor: entry.Passthrough,
		ViolationAction: entry.ViolationAction, RequireTLSIdentity: entry.RequireTLSIdentity,
	}
}

// AddGatewaySecret configures gateway authentication and lets prompts carry
// unchanged placeholders for other secrets.
func AddGatewaySecret(secrets []Secret, key string) []Secret {
	const gatewayHost = "host.microsandbox.internal"
	for i := range secrets {
		entry := &secrets[i].entry
		entry.Passthrough = append(entry.Passthrough, gatewayHost)
	}
	requireTLS := false
	gateway := Secret{entry: m.Secret.Env("LLM_GATEWAY_KEY", key, m.SecretEnvOptions{
		Allow:              []string{gatewayHost},
		Substitution:       m.SecretSubstitution{HeaderFields: []string{"authorization"}},
		RequireTLSIdentity: &requireTLS,
	})}
	return append(secrets, gateway)
}

// LoadSecrets reads only explicitly configured files or keyring entries. Relative paths
// are resolved by the settings loader before this function is called.
// Host environment credentials are ignored.
func LoadSecrets(settings config.Settings) ([]Secret, error) {
	return loadSecrets(settings, keyring.Get)
}

func loadSecrets(settings config.Settings, get func(string, string) (string, error)) ([]Secret, error) {
	if err := config.ValidateSecrets(settings.Secrets); err != nil {
		return nil, err
	}
	if len(settings.Secrets) == 0 {
		return nil, nil
	}
	names := make([]string, 0, len(settings.Secrets))
	for name := range settings.Secrets {
		names = append(names, name)
	}
	sort.Strings(names)
	var secrets []Secret
	for _, name := range names {
		config := settings.Secrets[name]
		if settings.GitHub != nil && isGitHubToken(config.Guest.Env) {
			continue
		}
		value, err := resolveSecretValue(name, config, get)
		if err != nil {
			return nil, err
		}
		hosts := make([]string, len(config.AllowedHosts))
		copy(hosts, config.AllowedHosts)
		headers := make([]string, len(config.Headers))
		for i, header := range config.Headers {
			headers[i] = strings.ToLower(header)
		}
		requireTLS := !config.AllowHTTP
		secrets = append(secrets, Secret{entry: m.Secret.Env(config.Guest.Env, value, m.SecretEnvOptions{
			Allow: hosts, Substitution: m.SecretSubstitution{HeaderFields: headers},
			RequireTLSIdentity: &requireTLS,
		})})
	}
	return secrets, nil
}

func resolveSecretValue(name string, settings config.SecretSettings, get func(string, string) (string, error)) (string, error) {
	if settings.Source.Keyring != nil {
		service, account := settings.Source.Keyring.Reference(name)
		value, err := get(service, account)
		if errors.Is(err, keyring.ErrNotFound) {
			return "", fmt.Errorf("secret %q: keyring entry not found; run 'opencode-sandbox secret set %s': %w", name, name, err)
		}
		if err != nil {
			return "", fmt.Errorf("secret %q: keyring access failed; check that the keyring is available and unlocked: %w", name, err)
		}
		if err := ValidateSecretValue(value); err != nil {
			return "", fmt.Errorf("load secret %q: %w", name, err)
		}
		return value, nil
	}
	if !filepath.IsAbs(settings.Source.File) {
		return "", fmt.Errorf("secret %q: credential path must be absolute", name)
	}
	value, err := readSecretFile(settings.Source.File)
	if err != nil {
		return "", fmt.Errorf("load secret %q: %w", name, err)
	}
	return value, nil
}

func readSecretFile(path string) (string, error) {
	if err := validateSecretDirectory(filepath.Dir(path)); err != nil {
		return "", err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("inspect secret file: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return "", fmt.Errorf("secret must be a regular owner-only file (chmod 600)")
	}
	if info.Size() > 16384 {
		return "", fmt.Errorf("secret file is too large")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read secret file: %w", err)
	}
	value := strings.TrimRight(string(data), "\r\n")
	if err := ValidateSecretValue(value); err != nil {
		return "", err
	}
	return value, nil
}

func validateSecretDirectory(directory string) error {
	// Reject symlinks in the credential path, not just its final component.
	for dir := directory; ; dir = filepath.Dir(dir) {
		info, err := os.Lstat(dir)
		if err != nil {
			return fmt.Errorf("inspect credential directory: %w", err)
		}
		if !info.IsDir() {
			return fmt.Errorf("credential path must contain real directories, not symlinks")
		}
		if dir == directory && info.Mode().Perm()&0077 != 0 {
			return fmt.Errorf("credential directory must be owner-only (chmod 700)")
		}
		if parent := filepath.Dir(dir); parent == dir {
			break
		}
	}
	return nil
}

// ValidateSecretValue applies the same credential constraints to file and keyring sources.
func ValidateSecretValue(value string) error {
	if len(value) > 16384 {
		return fmt.Errorf("secret is too large (maximum 16 KiB)")
	}
	if value == "" || strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("secret must contain one non-empty line without NUL bytes")
	}
	return nil
}

func connectWithSecrets(ctx context.Context, handle *m.SandboxHandle, secrets []Secret, githubApp bool) (workspaceConnection, error) {
	if handle.Status() == m.SandboxStatusStopped {
		bindings, err := storedPortBindings(handle.ConfigJSON())
		if err != nil {
			return workspaceConnection{}, err
		}
		if err := checkPortBindings(bindings); err != nil {
			return workspaceConnection{}, err
		}
	}
	stored, err := handle.Config()
	if err != nil {
		return workspaceConnection{}, fmt.Errorf("read stored VM configuration: %w", err)
	}
	_, wasGitHubApp := stored.Labels[githubAppLabel]
	if githubApp || wasGitHubApp {
		if err := reconcileGitHubSecrets(ctx, handle, secrets, githubApp); err != nil {
			return workspaceConnection{}, err
		}
	}
	vm, err := handle.ConnectOrStart(ctx, m.WithConnectOrStartDetached())
	if err != nil {
		return workspaceConnection{}, fmt.Errorf("connect sandbox: %w", err)
	}
	return workspaceConnection{vm: vm, refreshGitHubInstructions: wasGitHubApp}, nil
}

func reconcileGitHubSecrets(ctx context.Context, handle *m.SandboxHandle, secrets []Secret, appEnabled bool) error {
	patch := m.ModifyOptions{
		Policy:  m.ModificationPolicyRestart,
		Secrets: make(map[string]m.SecretModifySpec),
	}
	for _, secret := range secrets {
		if isGitHubToken(secret.entry.EnvVar) {
			patch.Secrets[secret.entry.EnvVar] = secret.modifySpec()
		}
	}
	for _, name := range []string{githubTokenEnv, githubReadTokenEnv} {
		if _, keep := patch.Secrets[name]; !keep {
			patch.SecretsRemove = append(patch.SecretsRemove, name)
		}
	}
	if appEnabled {
		patch.Labels = map[string]string{githubAppLabel: ""}
	}
	plan, err := handle.Modify(ctx, patch)
	if err != nil {
		return fmt.Errorf("update GitHub credentials before starting the VM: %w", err)
	}
	if !plan.Applied {
		return fmt.Errorf("update GitHub credentials before starting the VM: modification not applied")
	}
	return nil
}
