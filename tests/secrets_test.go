package sandbox_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/require"
)

func TestUnconfiguredSecretsAreNotLoadedOrInjected(t *testing.T) {
	path := githubTokenPath(t)
	// Invalid contents and permissions would fail if the implicit loader ran.
	require.NoError(t, os.WriteFile(path, nil, 0644))
	t.Setenv("GH_TOKEN", "host-credential-must-not-be-used")
	settings, err := sandbox.LoadSettings()
	require.NoError(t, err)
	secrets, err := sandbox.LoadSecrets(settings)
	require.NoError(t, err)
	require.Empty(t, secrets)
	s := startSandboxWithSettings(t, settings)
	execInSandbox(t, s, "bash", "-eu", "-c", `test -z "${GH_TOKEN+x}"`)
	require.NotContains(t, storedSandboxConfig(t, s), "host-credential-must-not-be-used")
}

func TestSecretSettingsRejectInvalidScopes(t *testing.T) {
	for _, fields := range []string{
		"file = 'creds/token'", // missing environment name and scope
		"file = 'creds/token'\nenv = 'BAD-NAME'\nallowed_hosts = ['github.com']\nheaders = ['authorization']",
		"file = 'creds/token'\nenv = 'TOKEN'\nallowed_hosts = []\nheaders = ['authorization']",
		"file = 'creds/token'\nenv = 'TOKEN'\nallowed_hosts = ['*.github.com']\nheaders = ['authorization']",
		"file = 'creds/token'\nenv = 'TOKEN'\nallowed_hosts = ['github.com']\nheaders = []",
		"file = 'creds/token'\nenv = 'TOKEN'\nallowed_hosts = ['github.com']\nheaders = ['bad header']",
		"file = 'creds/token'\nenv = 'TOKEN'\nallowed_hosts = ['github.com']\nheaders = ['authorization']\nvalue = 'inline-secrets-not-supported'",
	} {
		t.Run(fields, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			writeSettings(t, "[secrets.test]\n"+fields+"\n")
			_, err := sandbox.LoadSettings()
			require.Error(t, err)
		})
	}
}

func TestSecretSettingsRejectDuplicateEnvironmentNames(t *testing.T) {
	settings := githubSecretSettings("unused")
	settings.Secrets["other"] = settings.Secrets["github"]
	_, err := sandbox.LoadSecrets(settings)
	require.ErrorContains(t, err, "duplicate secret environment variable")
}

func TestConfiguredSecretsUseRelativeFilesAndSubstituteOnlySelectedHeaders(t *testing.T) {
	path := githubTokenPath(t)
	// Fake credentials only. Never send a developer credential to an echo service.
	require.NoError(t, os.WriteFile(path, []byte("FAKE_FIRST_SECRET\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(path), "second"), []byte("FAKE_SECOND_SECRET\n"), 0600))
	writeSettings(t, `
[network]
allowed_hosts = ['httpbingo.org']
[secrets.first]
file = 'creds/github-token'
env = 'FIRST_TOKEN'
allowed_hosts = ['httpbingo.org']
headers = ['Authorization']
[secrets.second]
file = 'creds/second'
env = 'SECOND_TOKEN'
allowed_hosts = ['httpbingo.org']
headers = ['X-Api-Key']
`)
	settings, err := sandbox.LoadSettings()
	require.NoError(t, err)
	secrets, err := sandbox.LoadSecrets(settings)
	require.NoError(t, err)
	require.Len(t, secrets, 2)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	worktree := newWorktree(t)
	s, err := sandbox.Prepare(ctx, worktree, nil, secrets, settings)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Remove(context.Background())) })
	guestEnv := execInSandbox(t, s, "env")
	require.Contains(t, guestEnv, "FIRST_TOKEN=$MSB_FIRST_TOKEN")
	require.Contains(t, guestEnv, "SECOND_TOKEN=$MSB_SECOND_TOKEN")
	require.NotContains(t, guestEnv, "FAKE_FIRST_SECRET")
	require.NotContains(t, guestEnv, "FAKE_SECOND_SECRET")
	response := execInSandbox(t, s, "curl", "--noproxy", "*", "--fail", "--silent", "--show-error", "--max-time", "15",
		"-H", "Authorization: Bearer $MSB_FIRST_TOKEN", "-H", "X-Api-Key: $MSB_SECOND_TOKEN",
		"-H", "X-Untrusted: $MSB_FIRST_TOKEN", "https://httpbingo.org/headers")
	var echoed struct {
		Headers map[string][]string `json:"headers"`
	}
	require.NoError(t, json.Unmarshal([]byte(response), &echoed))
	require.Equal(t, []string{"Bearer FAKE_FIRST_SECRET"}, echoed.Headers["Authorization"])
	require.Equal(t, []string{"FAKE_SECOND_SECRET"}, echoed.Headers["X-Api-Key"])
	require.Equal(t, []string{"$MSB_FIRST_TOKEN"}, echoed.Headers["X-Untrusted"])
	requireNetworkBlocked(t, s, "https://github.com")
}

func TestSecretHostDoesNotExpandNetworkAllowlist(t *testing.T) {
	settings := githubSecretSettings("unused")
	secrets := githubSecrets(t, []byte("github_pat_FAKE_NETWORK_SCOPE"))
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	s, err := sandbox.Prepare(ctx, newWorktree(t), nil, secrets, settings)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Remove(context.Background())) })
	requireNetworkBlocked(t, s, "https://github.com")
}

func TestSecretIsNotSubstitutedOnAnUnlistedHost(t *testing.T) {
	settings := sandbox.DefaultSettings()
	settings.Network.AllowedHosts = []string{"httpbingo.org"}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	s, err := sandbox.Prepare(ctx, newWorktree(t), nil, githubSecrets(t, []byte("FAKE_HOST_RESTRICTED_SECRET")), settings)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Remove(context.Background())) })
	// Prove the echo destination itself is reachable; the secret permits only GitHub.
	execInSandbox(t, s, "curl", "--fail", "--silent", "--max-time", "15", "https://httpbingo.org/headers")
	var output bytes.Buffer
	err = s.Exec(ctx, nil, &output, nil, "curl", "--fail", "--silent", "--max-time", "15",
		"-H", "Authorization: Bearer $MSB_GH_TOKEN", "https://httpbingo.org/headers")
	require.NoError(t, ctx.Err())
	require.NotContains(t, output.String(), "FAKE_HOST_RESTRICTED_SECRET")
	if err == nil {
		// Either block the request or leave its placeholder unresolved, never inject.
		require.Contains(t, output.String(), "$MSB_GH_TOKEN")
	}
}
