//go:build live

package sandbox

import (
	"bytes"
	"context"
	"testing"
	"time"

	"opencode-sandbox/internal/config"
	"opencode-sandbox/internal/githubapp"
	"opencode-sandbox/internal/workspace"

	"github.com/stretchr/testify/require"
	m "github.com/superradcompany/microsandbox/sdk/go"
)

func TestGitHubAppAccessChangesOnVMLaunch(t *testing.T) {
	t.Parallel()
	w, err := workspace.Resolve(t.Context(), t.TempDir())
	require.NoError(t, err)
	settings := config.DefaultSettings()
	settings.GitHub = &config.GitHubSettings{Owner: "acme", AppID: 123}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	t.Cleanup(func() {
		require.NoError(t, RemoveSnapshot(context.Background(), w.Root()))
	})

	launch := func(auth *githubAuth) *Sandbox {
		t.Helper()
		s, err := prepareWorkspace(ctx, w, nil, settings,
			func(context.Context, *config.GitHubSettings, string) (*githubAuth, error) { return auth, nil }, nil)
		require.NoError(t, err)
		return s
	}
	exec := func(s *Sandbox, args ...string) string {
		t.Helper()
		var out bytes.Buffer
		require.NoError(t, s.Exec(ctx, nil, &out, &out, args...), "output: %s", out.String())
		return out.String()
	}
	configJSON := func(s *Sandbox) string {
		t.Helper()
		handle, err := m.GetSandbox(ctx, s.Name)
		require.NoError(t, err)
		return handle.ConfigJSON()
	}

	read := githubapp.Token{Value: "fixture-read-only", ExpiresAt: time.Now().Add(time.Hour)}
	write := githubapp.Token{Value: "fixture-read-write", ExpiresAt: time.Now().Add(time.Hour)}
	readOnly := &githubAuth{owner: "acme", read: read}
	readWrite := &githubAuth{owner: "acme", repo: "widget", read: read, write: write}
	first := launch(readOnly)
	t.Cleanup(func() {
		handle, err := m.GetSandbox(context.Background(), first.Name)
		require.NoError(t, err)
		require.NoError(t, handle.Destroy(context.Background()))
	})
	require.Equal(t, "$MSB_GH_TOKEN\n", exec(first, "printenv", "GH_TOKEN"))
	require.Contains(t, exec(first, "cat", sandboxInstructions), "read repositories covered by the app")
	require.NoError(t, first.Shutdown())

	second := launch(readWrite)
	require.Equal(t, "$MSB_GH_READ_TOKEN\n", exec(second, "printenv", "GH_READ_TOKEN"))
	require.Contains(t, configJSON(second), write.Value)
	require.Contains(t, exec(second, "cat", sandboxInstructions), "acme/widget")
	bootID := exec(second, "cat", "/proc/sys/kernel/random/boot_id")
	rotated := githubapp.Token{Value: "fixture-rotated-write", ExpiresAt: time.Now().Add(time.Hour)}
	refreshAuth := &githubAuth{mint: func(_ context.Context, repo string) (githubapp.Token, error) {
		require.Equal(t, "widget", repo)
		return rotated, nil
	}}
	_, err = second.rotateGitHubToken(ctx, refreshAuth, readWrite.bindings(false)[0])
	require.NoError(t, err)
	require.Contains(t, configJSON(second), rotated.Value)
	require.Equal(t, bootID, exec(second, "cat", "/proc/sys/kernel/random/boot_id"))
	require.Equal(t, "$MSB_GH_TOKEN\n", exec(second, "printenv", "GH_TOKEN"))
	require.NoError(t, second.Shutdown())

	third := launch(readOnly)
	require.NotContains(t, configJSON(third), rotated.Value)
	require.NotContains(t, exec(third, "printenv"), "GH_READ_TOKEN=")
	instructions := exec(third, "cat", sandboxInstructions)
	require.Contains(t, instructions, "read repositories covered by the app")
	require.NotContains(t, instructions, "acme/widget")
	require.NoError(t, third.Shutdown())

	settings.GitHub = nil
	fourth := launch(nil)
	require.NotContains(t, configJSON(fourth), read.Value)
	require.NotContains(t, exec(fourth, "printenv"), "GH_TOKEN=")
	require.NotContains(t, exec(fourth, "cat", sandboxInstructions), "GitHub App access")
	require.NoError(t, fourth.Shutdown())
}
