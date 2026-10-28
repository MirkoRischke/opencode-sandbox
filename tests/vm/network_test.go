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
	t.Parallel()
	settings := testSettings(t)
	settings.Network.AllowedHosts = []string{"GitHub.com"}
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
	t.Parallel()
	settings := testSettings(t)
	settings.Network.AllowedHosts = []string{}
	settings.Network.AdditionalAllowedHosts = []string{}
	s := startSandboxWithSettings(t, settings)
	requireNetworkBlocked(t, s, "https://github.com")
}

func TestUnrestrictedNetworkAllowsPublicInternetButBlocksProtectedDestinations(t *testing.T) {
	t.Parallel()
	settings := testSettings(t)
	settings.Network.Unrestricted = true
	settings.Network.AllowedHosts = []string{"github.com"}
	settings.Network.AdditionalAllowedHosts = []string{"api.github.com"}
	s := startSandboxWithSettings(t, settings)
	for _, url := range []string{"https://example.org", "http://example.org", "http://1.1.1.1"} {
		t.Run(url, func(t *testing.T) {
			execInSandbox(t, s, "curl", "--noproxy", "*", "--fail", "--silent", "--show-error", "--max-time", "15", url)
		})
	}
	for _, url := range []string{
		"http://host.microsandbox.internal", "http://192.168.1.1", "http://10.0.0.1",
		"http://169.254.169.254", "http://169.254.1.1", "http://[fc00::1]",
	} {
		t.Run(url, func(t *testing.T) { requireNetworkBlocked(t, s, url) })
	}
}

func TestMissingConfigDefaultsToDevelopmentAllowlistInVM(t *testing.T) {
	t.Parallel()
	settings, err := launcherconfig.LoadSettings(t.TempDir())
	require.NoError(t, err)
	s := startSandboxWithSettings(t, settings)
	execInSandbox(t, s, "curl", "--noproxy", "*", "--fail", "--silent", "--show-error", "--max-time", "15", "https://github.com")
	requireNetworkBlocked(t, s, "https://example.org")
	requireNetworkBlocked(t, s, "https://1.1.1.1")
}

func TestExistingSandboxRetainsItsNetworkPolicy(t *testing.T) {
	t.Parallel()
	initial := testSettings(t)
	initial.Network.AllowedHosts = []string{}
	initial.Network.AdditionalAllowedHosts = []string{}
	s := startSandboxWithSettings(t, initial)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	require.NoError(t, s.Stop(ctx))
	settings := testSettings(t)
	settings.Network.AllowedHosts = []string{"github.com"}
	resumed, err := sandbox.Prepare(ctx, s.Worktree, nil, settings)
	require.NoError(t, err)
	defer func() { require.NoError(t, resumed.Close()) }()
	requireNetworkBlocked(t, resumed, "https://github.com")
}
