package sandbox_test

import (
	"testing"

	launcherconfig "opencode-sandbox/internal/config"
	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/require"
)

func TestWorkspaceApprovalDoesNotLoadCredentials(t *testing.T) {
	t.Parallel()
	root, _, paths := repoSettingsFixture(t, "[secrets.github]\nfile = 'missing-token'\nenv = 'GH_TOKEN'\nallowed_hosts = ['github.com']\nheaders = ['authorization']\n")
	settings, err := paths.LoadWorkspaceSettings(root, func(string, []byte, launcherconfig.Settings) error { return nil })
	require.NoError(t, err)
	// Missing credentials must not prevent reviewing and trusting configuration.
	_, err = sandbox.LoadSecrets(settings)
	require.Error(t, err)
	_, err = paths.LoadWorkspaceSettings(root, nil)
	require.NoError(t, err)
}
