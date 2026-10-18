package sandbox_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	launcherconfig "opencode-sandbox/internal/config"
	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/require"
)

func repoSettingsFixture(t *testing.T, config string) (string, string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root := filepath.Join(t.TempDir(), "repo")
	runGit(t, "init", root)
	path := filepath.Join(root, ".opencode-sandbox.toml")
	require.NoError(t, os.WriteFile(path, []byte(config), 0600))
	return root, path
}

func approveSettings(string, []byte, launcherconfig.Settings) error { return nil }

func TestRepoSettingsAdditionalNetworkHostsMergeIndependently(t *testing.T) {
	for _, tc := range []struct {
		name, config        string
		allowed, additional []string
	}{
		{"add repo hosts", "[network]\nadditional_allowed_hosts = ['registry.npmjs.org']\n", []string{"github.com"}, []string{"registry.npmjs.org"}},
		{"inherit both", "# inherit\n", []string{"github.com"}, []string{"global.example.com"}},
		{"clear additions", "[network]\nadditional_allowed_hosts = []\n", []string{"github.com"}, []string{}},
		{"clear baseline", "[network]\nallowed_hosts = []\n", []string{}, []string{"global.example.com"}},
		{"deny all", "[network]\nallowed_hosts = []\nadditional_allowed_hosts = []\n", []string{}, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, _ := repoSettingsFixture(t, tc.config)
			writeSettings(t, "[network]\nallowed_hosts = ['github.com']\nadditional_allowed_hosts = ['global.example.com']\n")
			settings, err := launcherconfig.LoadRepoSettings(root, func(_ string, _ []byte, effective launcherconfig.Settings) error {
				require.Equal(t, tc.allowed, effective.Network.AllowedHosts)
				require.Equal(t, tc.additional, effective.Network.AdditionalAllowedHosts)
				return nil
			})
			require.NoError(t, err)
			require.Equal(t, tc.allowed, settings.Network.AllowedHosts)
			require.Equal(t, tc.additional, settings.Network.AdditionalAllowedHosts)
		})
	}
}

func TestRepoSettingsMergeAllAttributes(t *testing.T) {
	root, path := repoSettingsFixture(t, `
[vm]
memory_mib = 8192
docker_disk_mib = 20480
[network]
allowed_hosts = []
[secrets.github]
allowed_hosts = ['custom.example.com']
headers = ['x-token']
[secrets.other]
file = 'creds/other'
env = 'OTHER_TOKEN'
allowed_hosts = ['other.example.com']
headers = ['authorization']
`)
	writeSettings(t, `
[vm]
cpus = 4
root_disk_mib = 9000
[network]
allowed_hosts = ['github.com']
[secrets.github]
file = 'creds/github-token'
env = 'GH_TOKEN'
allowed_hosts = ['api.github.com']
headers = ['authorization']
`)
	subdir := filepath.Join(root, "subdir")
	require.NoError(t, os.Mkdir(subdir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(subdir, ".opencode-sandbox.toml"), []byte("invalid"), 0600))
	calls := 0
	settings, err := launcherconfig.LoadRepoSettings(subdir, func(gotPath string, data []byte, effective launcherconfig.Settings) error {
		calls++
		require.Equal(t, path, gotPath)
		want, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, want, data)
		require.Equal(t, []string{"custom.example.com"}, effective.Secrets["github"].AllowedHosts)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Equal(t, launcherconfig.VMSettings{CPUs: 4, MemoryMiB: 8192, RootDiskMiB: 9000, DockerDiskMiB: 20480}, settings.VM)
	require.Empty(t, settings.Network.AllowedHosts)
	require.Equal(t, launcherconfig.SecretSettings{
		File: "creds/github-token", Env: "GH_TOKEN",
		AllowedHosts: []string{"custom.example.com"}, Headers: []string{"x-token"},
	}, settings.Secrets["github"])
	require.Equal(t, filepath.Join(root, "creds/other"), settings.Secrets["other"].File)
	// Credential files do not exist: loading/reviewing settings must not read them.
	_, err = sandbox.LoadSecrets(settings)
	require.Error(t, err)
	_, err = launcherconfig.LoadRepoSettings(root, nil)
	require.NoError(t, err)
}

func TestRepoSettingsSecretFileOverrideUsesRepoOrigin(t *testing.T) {
	root, _ := repoSettingsFixture(t, "[secrets.github]\nfile = 'local-token'\n")
	writeSettings(t, "[secrets.github]\nfile = 'global-token'\nenv = 'GH_TOKEN'\nallowed_hosts = ['github.com']\nheaders = ['authorization']\n")
	settings, err := launcherconfig.LoadRepoSettings(root, approveSettings)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "local-token"), settings.Secrets["github"].File)
}

func TestRepoSettingsTrustUsesExactReviewedBytes(t *testing.T) {
	original := "[vm]\nmemory_mib = 8192\n"
	root, path := repoSettingsFixture(t, original)
	settings, err := launcherconfig.LoadRepoSettings(root, func(_ string, data []byte, _ launcherconfig.Settings) error {
		require.Equal(t, original, string(data))
		return os.WriteFile(path, []byte("[vm]\nmemory_mib = 16384\n"), 0600)
	})
	require.NoError(t, err)
	require.EqualValues(t, 8192, settings.VM.MemoryMiB)
	_, err = launcherconfig.LoadRepoSettings(root, nil)
	require.ErrorContains(t, err, "untrusted repo config")

	trustPath := filepath.Join(os.Getenv("XDG_STATE_HOME"), "opencode-sandbox", "trust.json")
	data, err := os.ReadFile(trustPath)
	require.NoError(t, err)
	var store struct {
		Version      int `json:"version"`
		Repositories map[string]struct {
			ConfigSHA256 string `json:"config_sha256"`
		} `json:"repositories"`
	}
	require.NoError(t, json.Unmarshal(data, &store))
	require.Equal(t, 1, store.Version)
	require.Equal(t, fmt.Sprintf("%x", sha256.Sum256([]byte(original))), store.Repositories[root].ConfigSHA256)
	for path, mode := range map[string]os.FileMode{trustPath: 0600, filepath.Dir(trustPath): 0700} {
		info, err := os.Stat(path)
		require.NoError(t, err)
		require.Equal(t, mode, info.Mode().Perm())
	}
}

func TestRepoSettingsTrustBoundToCanonicalRepoPath(t *testing.T) {
	root, path := repoSettingsFixture(t, "# approved\n")
	_, err := launcherconfig.LoadRepoSettings(root, approveSettings)
	require.NoError(t, err)
	alias := filepath.Join(t.TempDir(), "alias")
	require.NoError(t, os.Symlink(root, alias))
	_, err = launcherconfig.LoadRepoSettings(alias, nil)
	require.NoError(t, err)
	other := filepath.Join(t.TempDir(), "other")
	runGit(t, "init", other)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(other, ".opencode-sandbox.toml"), data, 0600))
	_, err = launcherconfig.LoadRepoSettings(other, nil)
	require.ErrorContains(t, err, "untrusted repo config")
	// Even a comment-only change invalidates approval.
	require.NoError(t, os.WriteFile(path, []byte("# changed\n"), 0600))
	_, err = launcherconfig.LoadRepoSettings(root, nil)
	require.ErrorContains(t, err, "untrusted repo config")
}

func TestRepoSettingsApprovalRejectedOrUnavailable(t *testing.T) {
	root, _ := repoSettingsFixture(t, "[vm]\ncpus = 4\n")
	_, err := launcherconfig.LoadRepoSettings(root, nil)
	require.ErrorContains(t, err, "launch interactively")
	_, err = launcherconfig.LoadRepoSettings(root, func(string, []byte, launcherconfig.Settings) error {
		return fmt.Errorf("declined")
	})
	require.ErrorContains(t, err, "declined")
	_, err = os.Stat(filepath.Join(os.Getenv("XDG_STATE_HOME"), "opencode-sandbox", "trust.json"))
	require.True(t, os.IsNotExist(err))
}

func TestRepoSettingsMissingConfigSkipsTrust(t *testing.T) {
	root, path := repoSettingsFixture(t, "")
	require.NoError(t, os.Remove(path))
	settings, err := launcherconfig.LoadRepoSettings(root, nil)
	require.NoError(t, err)
	require.Equal(t, launcherconfig.DefaultSettings(), settings)
	entries, err := os.ReadDir(os.Getenv("XDG_STATE_HOME"))
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestRepoSettingsRejectInvalidBeforeApproval(t *testing.T) {
	for _, config := range []string{
		"[network", "[vm]\ncpuz = 4", "[vm]\ncpus = 0", "[vm]\ncpus = 256",
		"[network]\nallowed_hosts = 'github.com'", "[network]\nallowed_hosts = ['*.github.com']",
		"[network]\nadditional_allowed_hosts = ['*.github.com']", "[network]\nadditional_allowed_hosts = 'github.com'",
		"[secrets.test]\nfile = 'token'", "[secrets.test]\nvalue = 'inline'",
	} {
		t.Run(config, func(t *testing.T) {
			root, _ := repoSettingsFixture(t, config)
			_, err := launcherconfig.LoadRepoSettings(root, func(string, []byte, launcherconfig.Settings) error {
				t.Fatal("invalid config must not reach approval")
				return nil
			})
			require.Error(t, err)
		})
	}
}

func TestRepoSettingsMalformedTrustFailsClosed(t *testing.T) {
	for _, stored := range []string{"", "null", "{}", `{"version":2,"repositories":{}}`, `{"version":1,"repositories":null}`, `{"version":1,"repositories":{}} {}`, `{"version":1,"repositories":{},"typo":true}`} {
		t.Run(stored, func(t *testing.T) {
			root, _ := repoSettingsFixture(t, "# config\n")
			dir := filepath.Join(os.Getenv("XDG_STATE_HOME"), "opencode-sandbox")
			require.NoError(t, os.Mkdir(dir, 0700))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "trust.json"), []byte(stored), 0600))
			_, err := launcherconfig.LoadRepoSettings(root, func(string, []byte, launcherconfig.Settings) error {
				t.Fatal("malformed trust must fail closed")
				return nil
			})
			require.Error(t, err)
		})
	}
}

func TestRepoSettingsTrustFallsBackToHome(t *testing.T) {
	root, _ := repoSettingsFixture(t, "# config\n")
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", t.TempDir())
	_, err := launcherconfig.LoadRepoSettings(root, approveSettings)
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(os.Getenv("HOME"), ".local/state/opencode-sandbox/trust.json"))
	require.NoError(t, err)
}
