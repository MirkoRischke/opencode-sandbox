package sandbox_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	launcherconfig "opencode-sandbox/internal/config"
	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/require"
)

func TestRestrictedNetworkAllowsOnlyConfiguredHTTPSHostsWithoutCredentials(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeSettings(t, "[network]\nallowed_hosts = ['GitHub.com']\n")
	settings, err := launcherconfig.LoadSettings()
	require.NoError(t, err)
	s := startSandboxWithSettings(t, settings)
	execInSandbox(t, s, "curl", "--noproxy", "*", "--fail", "--silent", "--show-error", "--max-time", "15", "https://github.com")
	for _, url := range []string{
		"https://example.org", "https://api.github.com", "http://github.com",
		"https://github.com:8443", "https://1.1.1.1",
		"http://host.microsandbox.internal", "http://127.0.0.1",
		"http://192.168.1.1", "http://169.254.169.254",
	} {
		t.Run(url, func(t *testing.T) { requireNetworkBlocked(t, s, url) })
	}
	var config struct {
		Network struct {
			Strict bool `json:"strict"`
			TLS    struct {
				Enabled        bool `json:"enabled"`
				VerifyUpstream bool `json:"verify_upstream"`
			} `json:"tls"`
		} `json:"network"`
	}
	require.NoError(t, json.Unmarshal([]byte(storedSandboxConfig(t, s)), &config))
	require.True(t, config.Network.Strict)
	require.True(t, config.Network.TLS.Enabled)
	require.True(t, config.Network.TLS.VerifyUpstream)
}

func TestEmptyNetworkAllowlistBlocksInternet(t *testing.T) {
	s := startSandboxWithSettings(t, launcherconfig.DefaultSettings())
	requireNetworkBlocked(t, s, "https://github.com")
}

func TestMissingConfigDefaultsToDenyingInternetInVM(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	settings, err := launcherconfig.LoadSettings()
	require.NoError(t, err)
	s := startSandboxWithSettings(t, settings)
	requireNetworkBlocked(t, s, "https://github.com")
	requireNetworkBlocked(t, s, "https://example.org")
	requireNetworkBlocked(t, s, "https://1.1.1.1")
}

func TestExistingSandboxRetainsItsNetworkPolicy(t *testing.T) {
	s := startSandboxWithSettings(t, launcherconfig.DefaultSettings())
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	require.NoError(t, s.Stop(ctx))
	settings := launcherconfig.DefaultSettings()
	settings.Network.AllowedHosts = []string{"github.com"}
	resumed, err := sandbox.Prepare(ctx, s.Worktree, nil, nil, settings)
	require.NoError(t, err)
	defer resumed.Close()
	requireNetworkBlocked(t, resumed, "https://github.com")
}
