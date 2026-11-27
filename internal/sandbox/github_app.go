package sandbox

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"time"

	"opencode-sandbox/internal/config"
	"opencode-sandbox/internal/githubapp"
	"opencode-sandbox/internal/workspace"

	m "github.com/superradcompany/microsandbox/sdk/go"
	"github.com/zalando/go-keyring"
)

const (
	githubTokenEnv     = "GH_TOKEN"
	githubReadTokenEnv = "GH_READ_TOKEN"
	githubAppLabel     = "github-app-auth"
)

var githubRepoPattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

type githubAuth struct {
	mint  func(context.Context, string) (githubapp.Token, error)
	owner string
	repo  string // Set only when the workspace has write access.
	read  githubapp.Token
	write githubapp.Token
}

func githubRepository(w workspace.Workspace) string {
	var name string
	switch w.(type) {
	case workspace.GitRepository:
		name = filepath.Base(w.Root())
	case workspace.GitWorktree:
		name = w.RepositoryName()
	default:
		return ""
	}
	if !githubRepoPattern.MatchString(name) {
		return ""
	}
	return name
}

func loadGitHubAuth(ctx context.Context, settings *config.GitHubSettings, repo string) (*githubAuth, error) {
	if settings == nil {
		return nil, nil
	}
	account := fmt.Sprintf("github-app-%d", settings.AppID)
	key, err := keyring.Get(config.DefaultKeyringService, account)
	if err != nil {
		return nil, fmt.Errorf("load GitHub App keyring entry %q (store it with 'opencode-sandbox secret set %s'): %w", account, account, err)
	}
	client, err := githubapp.New(settings.AppID, key)
	if err != nil {
		return nil, fmt.Errorf("load GitHub App keyring entry %q: %w", account, err)
	}
	installation, writable, err := client.Installation(ctx, settings.Owner, repo)
	if err != nil {
		return nil, err
	}
	mint := func(ctx context.Context, repo string) (githubapp.Token, error) {
		return client.Mint(ctx, installation, repo)
	}
	read, err := mint(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("mint read-only GitHub App token for %s: %w", settings.Owner, err)
	}
	auth := &githubAuth{mint: mint, owner: settings.Owner, read: read}
	if writable {
		write, err := mint(ctx, repo)
		if err != nil {
			return nil, fmt.Errorf("mint GitHub App write token for %s/%s: %w", settings.Owner, repo, err)
		}
		auth.repo, auth.write = repo, write
	}
	return auth, nil
}

type githubTokenBinding struct {
	env, repo string
	token     githubapp.Token
}

func (a *githubAuth) bindings(readOnly bool) []githubTokenBinding {
	if readOnly || a.repo == "" {
		return []githubTokenBinding{{env: githubTokenEnv, token: a.read}}
	}
	return []githubTokenBinding{
		{env: githubTokenEnv, repo: a.repo, token: a.write},
		{env: githubReadTokenEnv, token: a.read},
	}
}

func (a *githubAuth) secrets(base []Secret, readOnly bool) []Secret {
	if a == nil {
		return base
	}
	secrets := make([]Secret, 0, len(base)+2)
	for _, secret := range base {
		if !isGitHubToken(secret.entry.EnvVar) {
			secrets = append(secrets, secret)
		}
	}
	for _, binding := range a.bindings(readOnly) {
		requireTLS := true
		secrets = append(secrets, Secret{entry: m.Secret.Env(binding.env, binding.token.Value, m.SecretEnvOptions{
			Allow:               []string{"github.com", "api.github.com"},
			AllowPlaceholderFor: []string{"host.microsandbox.internal"},
			RequireTLSIdentity:  &requireTLS,
			Substitution:        m.SecretSubstitution{HeaderFields: []string{"authorization"}},
		})})
	}
	return secrets
}

func isGitHubToken(env string) bool {
	return env == githubTokenEnv || env == githubReadTokenEnv
}

func (s *Sandbox) startGitHubRefresh(ctx context.Context, auth *githubAuth, readOnly bool) {
	if auth == nil {
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	s.stopGitHubRefresh = func() {
		cancel()
		<-done
	}
	bindings := auth.bindings(readOnly)
	go func() {
		defer close(done)
		s.refreshGitHub(ctx, auth, bindings)
	}()
}

const (
	gitHubRefreshLead  = 5 * time.Minute
	gitHubRefreshRetry = 30 * time.Second
)

func (s *Sandbox) refreshGitHub(ctx context.Context, auth *githubAuth, bindings []githubTokenBinding) {
	next := make([]time.Time, len(bindings))
	for i, binding := range bindings {
		next[i] = binding.token.ExpiresAt.Add(-gitHubRefreshLead)
	}
	for ctx.Err() == nil {
		// Serialize proxy updates, but retry each token independently so that
		// a failure to renew write access cannot interrupt read access.
		i := 0
		for j := range next {
			if next[j].Before(next[i]) {
				i = j
			}
		}
		timer := time.NewTimer(time.Until(next[i]))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		binding := bindings[i]
		expiresAt, err := s.rotateGitHubToken(ctx, auth, binding)
		if err != nil {
			if ctx.Err() == nil {
				s.log().Warn("GitHub App token refresh failed; will retry", "env", binding.env, "error", err)
			}
			next[i] = time.Now().Add(gitHubRefreshRetry)
			continue
		}
		next[i] = expiresAt.Add(-gitHubRefreshLead)
	}
}

func (s *Sandbox) rotateGitHubToken(ctx context.Context, auth *githubAuth, binding githubTokenBinding) (time.Time, error) {
	token, err := auth.mint(ctx, binding.repo)
	if err != nil {
		return time.Time{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	plan, err := s.vm.Modify(ctx, m.ModifyOptions{
		Secrets: map[string]m.SecretModifySpec{binding.env: {Value: token.Value}},
	})
	if err != nil {
		return time.Time{}, fmt.Errorf("rotate VM GitHub credential: %w", err)
	}
	if !plan.Applied {
		return time.Time{}, fmt.Errorf("rotate VM GitHub credential: modification not applied")
	}
	return token.ExpiresAt, nil
}
