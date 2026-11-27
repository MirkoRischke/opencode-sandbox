package sandbox_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	launcherconfig "opencode-sandbox/internal/config"
	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/require"
)

func TestHostHTTPGatewaySubstitutesOnlyItsAuthorizationAndPassesPlaceholders(t *testing.T) {
	t.Parallel()
	type request struct {
		headers http.Header
		body    string
	}
	requests := make(chan request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requests <- request{headers: r.Header.Clone(), body: string(body)}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	settings := testSettings(t)
	address, ok := server.Listener.Addr().(*net.TCPAddr)
	require.True(t, ok)
	settings.Network.VMToHost.Ports = []uint16{uint16(address.Port)}
	dir := t.TempDir()
	path := filepath.Join(dir, "service-key")
	require.NoError(t, os.Chmod(dir, 0700))
	require.NoError(t, os.WriteFile(path, []byte("FAKE_SERVICE_KEY"), 0600))
	settings.Secrets = map[string]launcherconfig.SecretSettings{
		"service": {Source: launcherconfig.SecretSourceSettings{File: path}, Guest: launcherconfig.SecretGuestSettings{Env: "SERVICE_KEY"}, AllowedHosts: []string{"service.example.com"}, Headers: []string{"x-service-key"}, AllowHTTP: true},
	}
	secrets, err := sandbox.LoadSecrets(settings)
	require.NoError(t, err)
	secrets = sandbox.AddGatewaySecret(secrets, "FAKE_GATEWAY_KEY")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	s, err := sandbox.Prepare(ctx, newWorktree(t), secrets, settings)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Remove(context.Background())) })
	const body = `{"messages":[{"role":"user","content":"$MSB_SERVICE_KEY $MSB_LLM_GATEWAY_KEY"}]}`
	execInSandbox(t, s, "curl", "--noproxy", "*", "--fail", "--silent", "--show-error", "--max-time", "10",
		"-H", "Authorization: Bearer $MSB_LLM_GATEWAY_KEY", "-H", "X-Untrusted: $MSB_LLM_GATEWAY_KEY",
		"-H", "X-Service-Key: $MSB_SERVICE_KEY", "-H", "Content-Type: application/json", "--data", body,
		fmt.Sprintf("http://host.microsandbox.internal:%d/v1/chat/completions", settings.Network.VMToHost.Ports[0]))
	select {
	case got := <-requests:
		require.Equal(t, "Bearer FAKE_GATEWAY_KEY", got.headers.Get("Authorization"))
		require.Equal(t, "$MSB_LLM_GATEWAY_KEY", got.headers.Get("X-Untrusted"))
		require.Equal(t, "$MSB_SERVICE_KEY", got.headers.Get("X-Service-Key"))
		require.Equal(t, body, got.body)
	case <-ctx.Done():
		t.Fatal("host gateway did not receive the request")
	}
}

func TestUnconfiguredSecretsAreNotLoadedOrInjected(t *testing.T) {
	t.Parallel()
	path := githubTokenPath(t)
	// Invalid contents and permissions would fail if the implicit loader ran.
	require.NoError(t, os.WriteFile(path, nil, 0644))
	dir := filepath.Dir(filepath.Dir(filepath.Dir(path)))
	settings, err := launcherconfig.LoadSettings(dir)
	require.NoError(t, err)
	secrets, err := sandbox.LoadSecrets(settings)
	require.NoError(t, err)
	require.Empty(t, secrets)
	worktree := newLauncherProbeWorktree(t)
	require.NoError(t, runLauncherProbe(t, worktree, map[string]string{
		"XDG_CONFIG_HOME": dir, "XDG_STATE_HOME": t.TempDir(),
		"GH_TOKEN": "host-credential-must-not-be-used",
	}, `test -z "${GH_TOKEN+x}"`))
	require.NotContains(t, storedSandboxConfigByName(t, launcherProbeVMName(t, worktree)), "host-credential-must-not-be-used")
}

func TestConfiguredSecretsUseRelativeFilesAndSubstituteOnlySelectedHeaders(t *testing.T) {
	t.Parallel()
	path := githubTokenPath(t)
	dir := filepath.Dir(filepath.Dir(filepath.Dir(path)))
	// Fake credentials only. Never send a developer credential to an echo service.
	require.NoError(t, os.WriteFile(path, []byte("FAKE_FIRST_SECRET\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(path), "second"), []byte("FAKE_SECOND_SECRET\n"), 0600))
	writeSettingsAt(t, dir, `
[network]
allowed_hosts = ['httpbingo.org']
[secrets.first]
source.file = 'creds/github-token'
guest.env = 'FIRST_TOKEN'
allowed_hosts = ['httpbingo.org']
headers = ['Authorization']
[secrets.second]
source.file = 'creds/second'
guest.env = 'SECOND_TOKEN'
allowed_hosts = ['httpbingo.org']
headers = ['X-Api-Key']
`)
	settings, err := launcherconfig.LoadSettings(dir)
	require.NoError(t, err)
	secrets, err := sandbox.LoadSecrets(settings)
	require.NoError(t, err)
	require.Len(t, secrets, 2)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	worktree := newWorktree(t)
	s, err := sandbox.Prepare(ctx, worktree, secrets, settings)
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
	requireNetworkBlocked(t, s, "https://github.com")
	t.Run("selected_headers", func(t *testing.T) {
		require.Equal(t, []string{"$MSB_FIRST_TOKEN"}, echoed.Headers["X-Untrusted"])
	})
}

func TestSecretHostDoesNotExpandNetworkAllowlist(t *testing.T) {
	t.Parallel()
	settings := githubSecretSettings("unused")
	settings.Network.AllowedHosts = []string{}
	secrets := githubSecrets(t, []byte("github_pat_FAKE_NETWORK_SCOPE"))
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	s, err := sandbox.Prepare(ctx, newWorktree(t), secrets, settings)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Remove(context.Background())) })
	requireNetworkBlocked(t, s, "https://github.com")
}

func TestSecretIsNotSubstitutedOnAnUnlistedHost(t *testing.T) {
	t.Parallel()
	settings := testSettings(t)
	settings.Network.AllowedHosts = []string{"httpbingo.org"}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	s, err := sandbox.Prepare(ctx, newWorktree(t), githubSecrets(t, []byte("FAKE_HOST_RESTRICTED_SECRET")), settings)
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
