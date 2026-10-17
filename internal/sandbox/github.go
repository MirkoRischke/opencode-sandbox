package sandbox

import (
	"context"
	"fmt"

	m "github.com/superradcompany/microsandbox/sdk/go"
)

func connectWithGitHub(ctx context.Context, name string, token []byte, opts []m.SandboxOption) (*m.Sandbox, error) {
	if token != nil {
		// The real value stays on the host (including its persisted VM config).
		// The guest receives only $MSB_GH_TOKEN.
		opts = append(opts, m.WithSecrets(m.Secret.Env("GH_TOKEN", string(token), m.SecretEnvOptions{
			Allow:        []string{"api.github.com", "github.com"},
			Substitution: m.SecretSubstitution{HeaderFields: []string{"authorization"}},
		})))
	}
	vm, err := m.ConnectOrCreateSandbox(ctx, name, opts...)
	if err != nil {
		return nil, fmt.Errorf("connect sandbox (runtime details suppressed)")
	}
	return vm, nil
}

func (s *Sandbox) configureGitHub(ctx context.Context, enabled bool) error {
	if !enabled {
		return nil
	}
	if err := s.Exec(ctx, nil, nil, nil, "gh", "auth", "setup-git", "--hostname", "github.com"); err != nil {
		return fmt.Errorf("configure guest GitHub CLI (rebuild the image and recreate this VM if GitHub support is missing): %w", err)
	}
	return nil
}
