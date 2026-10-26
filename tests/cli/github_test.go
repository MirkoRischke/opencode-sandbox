package sandbox_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	launcherconfig "opencode-sandbox/internal/config"
	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/require"
)

func TestGitHubTokenLoadsOnlyFromDedicatedCredentialFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0700))
	path := filepath.Join(dir, "github-token")
	require.NoError(t, os.WriteFile(path, []byte("github_pat_FAKE\n"), 0600))
	settings := launcherconfig.DefaultSettings()
	settings.Secrets = map[string]launcherconfig.SecretSettings{"github": {
		File: path, Env: "GH_TOKEN", AllowedHosts: []string{"api.github.com", "github.com"}, Headers: []string{"authorization"},
	}}

	secrets, err := sandbox.LoadSecrets(settings)

	require.NoError(t, err)
	require.Len(t, secrets, 1)
	require.NoError(t, os.Remove(path))
	_, err = sandbox.LoadSecrets(settings)
	require.Error(t, err)
	// The real launcher must not fall back to GH_TOKEN when its file is missing.
	dir = t.TempDir()
	writeSettingsAt(t, dir, "[secrets.github]\nfile = 'missing-token'\nenv = 'GH_TOKEN'\nallowed_hosts = ['github.com']\nheaders = ['authorization']\n")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, buildLauncher(t))
	command.Dir = newWorktree(t)
	command.Env = append(os.Environ(), "XDG_CONFIG_HOME="+dir, "XDG_STATE_HOME="+t.TempDir(), "GH_TOKEN=host-gh-token-must-not-be-used", "HERDR_ENV=")
	output, err := command.CombinedOutput()
	require.Error(t, err)
	require.Contains(t, string(output), "load secret")
	require.NotContains(t, string(output), "host-gh-token-must-not-be-used")
}
