package config_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	launcherconfig "opencode-sandbox/internal/config"

	"github.com/stretchr/testify/require"
)

func repoSettingsFixture(t *testing.T, config string) (string, string, launcherconfig.Paths) {
	t.Helper()
	paths := launcherconfig.Paths{ConfigDir: t.TempDir(), StateDir: t.TempDir()}
	root := filepath.Join(t.TempDir(), "repo")
	runGit(t, "init", root)
	path := filepath.Join(root, ".opencode-sandbox.toml")
	require.NoError(t, os.WriteFile(path, []byte(config), 0600))
	return root, path, paths
}

func approveSettings(string, []byte, launcherconfig.Settings) error { return nil }

func TestWorkspaceSettingsIgnoreParentConfiguration(t *testing.T) {
	t.Parallel()
	root, _, paths := repoSettingsFixture(t, "[vm]\ncpus = 7\n")
	writeSettingsAt(t, paths.ConfigDir, "[vm]\ncpus = 3\n")
	subdir := filepath.Join(root, "nested")
	require.NoError(t, os.Mkdir(subdir, 0700))
	settings, err := paths.LoadWorkspaceSettings(subdir, nil)
	require.NoError(t, err, "parent configuration must not require approval")
	require.EqualValues(t, 3, settings.VM.CPUs)
	_, err = os.Stat(filepath.Join(paths.StateDir, "opencode-sandbox", "trust.json"))
	require.ErrorIs(t, err, os.ErrNotExist)
	require.NoError(t, os.WriteFile(filepath.Join(subdir, ".opencode-sandbox.toml"), []byte("[vm]\ncpus = 5\n"), 0600))
	_, err = paths.LoadWorkspaceSettings(subdir, nil)
	require.ErrorContains(t, err, "untrusted workspace config")
	settings, err = paths.LoadWorkspaceSettings(subdir, approveSettings)
	require.NoError(t, err)
	require.EqualValues(t, 5, settings.VM.CPUs)
}

func TestNonGitWorkspaceSettingsRequireApproval(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	paths := launcherconfig.Paths{ConfigDir: t.TempDir(), StateDir: t.TempDir()}
	require.NoError(t, os.WriteFile(filepath.Join(root, ".opencode-sandbox.toml"), []byte("[vm]\ncpus = 5\n"), 0600))
	_, err := paths.LoadWorkspaceSettings(root, nil)
	require.ErrorContains(t, err, "untrusted workspace config")
	settings, err := paths.LoadWorkspaceSettings(root, approveSettings)
	require.NoError(t, err)
	require.EqualValues(t, 5, settings.VM.CPUs)
	_, err = paths.LoadWorkspaceSettings(root, nil)
	require.NoError(t, err, "unchanged approved configuration must be reusable")
}

func TestRepoSettingsVMImage(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, config, want string
	}{
		{"override", "[vm]\nimage = 'repo-sandbox:local'\n", "repo-sandbox:local"},
		{"inherit", "[vm]\nmemory_mib = 4096\n", "global-sandbox:local"},
		{"reject empty", "[vm]\nimage = ''\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, _, paths := repoSettingsFixture(t, tc.config)
			writeSettingsAt(t, paths.ConfigDir, "[vm]\nimage = 'global-sandbox:local'\n")
			settings, err := paths.LoadWorkspaceSettings(root, approveSettings)
			if tc.want == "" {
				require.ErrorContains(t, err, "vm.image")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, settings.VM.Image)
		})
	}
}

func TestRepoSettingsAdditionalNetworkHostsMergeIndependently(t *testing.T) {
	t.Parallel()
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
			root, _, paths := repoSettingsFixture(t, tc.config)
			writeSettingsAt(t, paths.ConfigDir, "[network]\nallowed_hosts = ['github.com']\nadditional_allowed_hosts = ['global.example.com']\n")
			settings, err := paths.LoadWorkspaceSettings(root, func(_ string, _ []byte, effective launcherconfig.Settings) error {
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
	t.Parallel()
	root, path, paths := repoSettingsFixture(t, `
[vm]
memory_mib = 8192
docker_disk_mib = 20480
[network]
allowed_hosts = []
[secrets.github]
file = 'creds/repo-github-token'
env = 'GH_TOKEN'
allowed_hosts = ['custom.example.com']
headers = ['x-token']
[secrets.other]
file = 'creds/other'
env = 'OTHER_TOKEN'
allowed_hosts = ['other.example.com']
headers = ['authorization']
`)
	writeSettingsAt(t, paths.ConfigDir, `
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
	calls := 0
	settings, err := paths.LoadWorkspaceSettings(root, func(gotPath string, data []byte, effective launcherconfig.Settings) error {
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
	require.Equal(t, launcherconfig.VMSettings{Image: "opencode-sandbox:base", CPUs: 4, MemoryMiB: 8192, RootDiskMiB: 9000, DockerDiskMiB: 20480}, settings.VM)
	require.Empty(t, settings.Network.AllowedHosts)
	require.Equal(t, launcherconfig.SecretSettings{
		File: filepath.Join(root, "creds/repo-github-token"), Env: "GH_TOKEN",
		AllowedHosts: []string{"custom.example.com"}, Headers: []string{"x-token"},
	}, settings.Secrets["github"])
	require.Equal(t, filepath.Join(root, "creds/other"), settings.Secrets["other"].File)
	_, err = paths.LoadWorkspaceSettings(root, nil)
	require.NoError(t, err)
}

func TestRepoSettingsSecretFileOverrideUsesRepoOrigin(t *testing.T) {
	t.Parallel()
	root, _, paths := repoSettingsFixture(t, "[secrets.github]\nfile = 'local-token'\nenv = 'GH_TOKEN'\nallowed_hosts = ['github.com']\nheaders = ['authorization']\n")
	writeSettingsAt(t, paths.ConfigDir, "[secrets.github]\nfile = 'global-token'\nenv = 'GH_TOKEN'\nallowed_hosts = ['github.com']\nheaders = ['authorization']\n")
	settings, err := paths.LoadWorkspaceSettings(root, approveSettings)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "local-token"), settings.Secrets["github"].File)
}

func TestRepoSettingsTrustUsesExactReviewedBytes(t *testing.T) {
	t.Parallel()
	original := "[vm]\nmemory_mib = 8192\n"
	root, path, paths := repoSettingsFixture(t, original)
	settings, err := paths.LoadWorkspaceSettings(root, func(_ string, data []byte, _ launcherconfig.Settings) error {
		require.Equal(t, original, string(data))
		return os.WriteFile(path, []byte("[vm]\nmemory_mib = 16384\n"), 0600)
	})
	require.NoError(t, err)
	require.EqualValues(t, 8192, settings.VM.MemoryMiB)
	_, err = paths.LoadWorkspaceSettings(root, nil)
	require.ErrorContains(t, err, "untrusted workspace config")

	trustPath := filepath.Join(paths.StateDir, "opencode-sandbox", "trust.json")
	data, err := os.ReadFile(trustPath)
	require.NoError(t, err)
	var store struct {
		Version    int `json:"version"`
		Workspaces map[string]struct {
			ConfigSHA256 string `json:"config_sha256"`
		} `json:"workspaces"`
	}
	require.NoError(t, json.Unmarshal(data, &store))
	require.Equal(t, 1, store.Version)
	require.Equal(t, fmt.Sprintf("%x", sha256.Sum256([]byte(original))), store.Workspaces[root].ConfigSHA256)
	for path, mode := range map[string]os.FileMode{trustPath: 0600, filepath.Dir(trustPath): 0700} {
		info, err := os.Stat(path)
		require.NoError(t, err)
		require.Equal(t, mode, info.Mode().Perm())
	}
}

func TestRepoSettingsTrustBoundToCanonicalRepoPath(t *testing.T) {
	t.Parallel()
	root, path, paths := repoSettingsFixture(t, "# approved\n")
	_, err := paths.LoadWorkspaceSettings(root, approveSettings)
	require.NoError(t, err)
	alias := filepath.Join(t.TempDir(), "alias")
	require.NoError(t, os.Symlink(root, alias))
	_, err = paths.LoadWorkspaceSettings(alias, nil)
	require.NoError(t, err)
	other := filepath.Join(t.TempDir(), "other")
	runGit(t, "init", other)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(other, ".opencode-sandbox.toml"), data, 0600))
	_, err = paths.LoadWorkspaceSettings(other, nil)
	require.ErrorContains(t, err, "untrusted workspace config")
	// Even a comment-only change invalidates approval.
	require.NoError(t, os.WriteFile(path, []byte("# changed\n"), 0600))
	_, err = paths.LoadWorkspaceSettings(root, nil)
	require.ErrorContains(t, err, "untrusted workspace config")
}

func TestRepoSettingsApprovalRejectedOrUnavailable(t *testing.T) {
	t.Parallel()
	root, _, paths := repoSettingsFixture(t, "[vm]\ncpus = 4\n")
	_, err := paths.LoadWorkspaceSettings(root, nil)
	require.ErrorContains(t, err, "launch interactively")
	_, err = paths.LoadWorkspaceSettings(root, func(string, []byte, launcherconfig.Settings) error {
		return fmt.Errorf("declined")
	})
	require.ErrorContains(t, err, "declined")
	_, err = os.Stat(filepath.Join(paths.StateDir, "opencode-sandbox", "trust.json"))
	require.True(t, os.IsNotExist(err))
}

func TestRepoSettingsMissingConfigSkipsTrust(t *testing.T) {
	t.Parallel()
	root, path, paths := repoSettingsFixture(t, "")
	require.NoError(t, os.Remove(path))
	settings, err := paths.LoadWorkspaceSettings(root, nil)
	require.NoError(t, err)
	require.Equal(t, launcherconfig.DefaultSettings(), settings)
	entries, err := os.ReadDir(paths.StateDir)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestRepoSettingsRejectInvalidBeforeApproval(t *testing.T) {
	t.Parallel()
	for _, config := range []string{
		"[network", "[vm]\ncpuz = 4", "[vm]\ncpus = 0", "[vm]\ncpus = 256",
		"[network]\nallowed_hosts = 'github.com'", "[network]\nallowed_hosts = ['*.github.com']",
		"[network]\nadditional_allowed_hosts = ['*.github.com']", "[network]\nadditional_allowed_hosts = 'github.com'",
		"[secrets.test]\nfile = 'token'", "[secrets.test]\nvalue = 'inline'",
	} {
		t.Run(config, func(t *testing.T) {
			root, _, paths := repoSettingsFixture(t, config)
			_, err := paths.LoadWorkspaceSettings(root, func(string, []byte, launcherconfig.Settings) error {
				t.Fatal("invalid config must not reach approval")
				return nil
			})
			require.Error(t, err)
		})
	}
}

func TestRepoSettingsMalformedTrustFailsClosed(t *testing.T) {
	t.Parallel()
	for _, stored := range []string{"", "null", "{}", `{"version":2,"workspaces":{}}`, `{"version":1,"workspaces":null}`, `{"version":1,"workspaces":{}} {}`, `{"version":1,"workspaces":{},"typo":true}`} {
		t.Run(stored, func(t *testing.T) {
			root, _, paths := repoSettingsFixture(t, "# config\n")
			dir := filepath.Join(paths.StateDir, "opencode-sandbox")
			require.NoError(t, os.Mkdir(dir, 0700))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "trust.json"), []byte(stored), 0600))
			_, err := paths.LoadWorkspaceSettings(root, func(string, []byte, launcherconfig.Settings) error {
				t.Fatal("malformed trust must fail closed")
				return nil
			})
			require.Error(t, err)
		})
	}
}
