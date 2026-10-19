package sandbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"opencode-sandbox/internal/config"

	m "github.com/superradcompany/microsandbox/sdk/go"
)

// Secret contains a resolved host credential. Never log it or expose it to guests.
type Secret struct {
	entry m.SecretEntry
}

// LoadSecrets reads only explicitly configured files. Remaining relative paths
// are resolved by the settings loader before this function is called.
// Host environment credentials are ignored.
func LoadSecrets(settings config.Settings) ([]Secret, error) {
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
		path := config.File
		if !filepath.IsAbs(path) {
			return nil, fmt.Errorf("secret %q: credential path must be absolute", name)
		}
		value, err := readSecretFile(path)
		if err != nil {
			return nil, fmt.Errorf("load secret %q: %w", name, err)
		}
		hosts := make([]string, len(config.AllowedHosts))
		copy(hosts, config.AllowedHosts)
		headers := make([]string, len(config.Headers))
		for i, header := range config.Headers {
			headers[i] = strings.ToLower(header)
		}
		secrets = append(secrets, Secret{entry: m.Secret.Env(config.Env, value, m.SecretEnvOptions{
			Allow: hosts, Substitution: m.SecretSubstitution{HeaderFields: headers},
		})})
	}
	return secrets, nil
}

func readSecretFile(path string) (string, error) {
	// Reject symlinks in the credential path, not just its final component.
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		info, err := os.Lstat(dir)
		if err != nil {
			return "", fmt.Errorf("inspect credential directory: %w", err)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("credential path must contain real directories, not symlinks")
		}
		if dir == filepath.Dir(path) && info.Mode().Perm()&0077 != 0 {
			return "", fmt.Errorf("credential directory must be owner-only (chmod 700)")
		}
		if parent := filepath.Dir(dir); parent == dir {
			break
		}
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
	if value == "" || strings.ContainsAny(value, "\r\n\x00") {
		return "", fmt.Errorf("secret must contain one non-empty line without NUL bytes")
	}
	return value, nil
}

func connectWithSecrets(ctx context.Context, name string, secrets []Secret, opts []m.SandboxOption) (*m.Sandbox, error) {
	for _, secret := range secrets {
		opts = append(opts, m.WithSecrets(secret.entry))
	}
	vm, err := m.ConnectOrCreateSandbox(ctx, name, opts...)
	if err != nil {
		return nil, fmt.Errorf("connect sandbox (runtime details suppressed)")
	}
	return vm, nil
}
