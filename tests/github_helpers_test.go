package sandbox_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	launcherconfig "opencode-sandbox/internal/config"
	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	m "github.com/superradcompany/microsandbox/sdk/go"
)

func githubTokenPath(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "opencode-sandbox", "creds")
	require.NoError(t, os.MkdirAll(dir, 0700))
	return filepath.Join(dir, "github-token")
}

func prepareWithGitHub(t *testing.T, worktree string, token []byte) *sandbox.Sandbox {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	s, err := sandbox.Prepare(ctx, worktree, nil, githubSecrets(t, token), testSettings(t))
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		assert.NoError(t, s.Remove(ctx))
	})
	return s
}

func githubSecrets(t *testing.T, token []byte) []sandbox.Secret {
	t.Helper()
	if token == nil {
		return nil
	}
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0700))
	path := filepath.Join(dir, "token")
	require.NoError(t, os.WriteFile(path, token, 0600))
	return loadGitHubSecrets(t, path)
}

func githubSecretSettings(path string) launcherconfig.Settings {
	settings := launcherconfig.DefaultSettings()
	settings.Secrets = map[string]launcherconfig.SecretSettings{"github": {
		File: path, Env: "GH_TOKEN", AllowedHosts: []string{"api.github.com", "github.com"}, Headers: []string{"authorization"},
	}}
	return settings
}

func loadGitHubSecrets(t *testing.T, path string) []sandbox.Secret {
	t.Helper()
	secrets, err := sandbox.LoadSecrets(githubSecretSettings(path))
	require.NoError(t, err)
	return secrets
}

func storedSandboxConfig(t *testing.T, s *sandbox.Sandbox) string {
	t.Helper()
	handle, err := m.GetSandbox(context.Background(), s.Name)
	require.NoError(t, err)
	return handle.ConfigJSON()
}

// Echo only fake credentials, never a developer token. Production credentials
// remain restricted to GitHub; temporarily change the test VM's destination to
// observe actual TLS substitution and restore it before launcher reuse.
func echoedGitHubAuthorization(t *testing.T, s *sandbox.Sandbox) string {
	t.Helper()
	handle, err := m.GetSandbox(context.Background(), s.Name)
	require.NoError(t, err)
	_, err = handle.Modify(context.Background(), m.ModifyOptions{Secrets: map[string]m.SecretModifySpec{
		"GH_TOKEN": {AllowedHosts: []string{"httpbingo.org"}},
	}})
	require.NoError(t, err)
	defer func() {
		_, err := handle.Modify(context.Background(), m.ModifyOptions{Secrets: map[string]m.SecretModifySpec{
			"GH_TOKEN": {AllowedHosts: []string{"api.github.com", "github.com"}},
		}})
		require.NoError(t, err)
	}()
	return execInSandbox(t, s, "bash", "-eu", "-c", `
curl --fail --silent --show-error --max-time 15 https://httpbingo.org/headers \
  -H "Authorization: Bearer $GH_TOKEN" -H "X-Untrusted: $GH_TOKEN"`)
}
