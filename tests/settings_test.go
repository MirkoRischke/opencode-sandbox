package sandbox_test

import (
	"os"
	"path/filepath"
	"testing"

	launcherconfig "opencode-sandbox/internal/config"

	"github.com/stretchr/testify/require"
)

func TestSettingsLoadFromConfigDirectory(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeSettings(t, "[network]\nallowed_hosts = ['github.com', 'api.github.com']\n")

	settings, err := launcherconfig.LoadSettings()

	require.NoError(t, err)
	require.Equal(t, []string{"github.com", "api.github.com"}, settings.Network.AllowedHosts)
}

func TestSettingsLoadAdditionalNetworkHosts(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeSettings(t, "[network]\nadditional_allowed_hosts = ['registry.npmjs.org']\n")
	settings, err := launcherconfig.LoadSettings()
	require.NoError(t, err)
	require.Empty(t, settings.Network.AllowedHosts)
	require.Equal(t, []string{"registry.npmjs.org"}, settings.Network.AdditionalAllowedHosts)
}

func TestSettingsRejectInvalidAdditionalNetworkHosts(t *testing.T) {
	for _, value := range []string{
		"'github.com'", "['https://github.com']", "['*.github.com']",
		"['1.1.1.1']", "['github.com:443']", "['']",
	} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			writeSettings(t, "[network]\nadditional_allowed_hosts = "+value+"\n")
			_, err := launcherconfig.LoadSettings()
			require.ErrorContains(t, err, "additional_allowed_hosts")
		})
	}
}

func TestVMSettingsOverrideDefaultsIndependently(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeSettings(t, "[vm]\nmemory_mib = 3072\n")
	settings, err := launcherconfig.LoadSettings()
	require.NoError(t, err)
	require.Equal(t, launcherconfig.VMSettings{CPUs: 2, MemoryMiB: 3072, RootDiskMiB: 4096, DockerDiskMiB: 10240}, settings.VM)
	require.Empty(t, settings.Network.AllowedHosts)
}

func TestSettingsUseHomeWhenXDGIsUnset(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	dir := filepath.Join(os.Getenv("HOME"), ".config", "opencode-sandbox")
	require.NoError(t, os.MkdirAll(dir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"), []byte("[network]\nallowed_hosts = []\n"), 0600))

	settings, err := launcherconfig.LoadSettings()

	require.NoError(t, err)
	require.Empty(t, settings.Network.AllowedHosts)
}

func TestSettingsMissingFileUsesRestrictiveDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	settings, err := launcherconfig.LoadSettings()
	require.NoError(t, err)
	require.Equal(t, launcherconfig.DefaultSettings(), settings)
}

func TestSettingsOmittedNetworkUsesRestrictiveDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeSettings(t, "")
	settings, err := launcherconfig.LoadSettings()
	require.NoError(t, err)
	require.Equal(t, launcherconfig.DefaultSettings(), settings)
}

func TestSettingsRejectInvalidConfiguration(t *testing.T) {
	for name, contents := range map[string]string{
		"zero CPUs":            "[vm]\ncpus = 0\n",
		"negative CPUs":        "[vm]\ncpus = -1\n",
		"CPU overflow":         "[vm]\ncpus = 256\n",
		"zero memory":          "[vm]\nmemory_mib = 0\n",
		"negative memory":      "[vm]\nmemory_mib = -1\n",
		"memory overflow":      "[vm]\nmemory_mib = 4294967296\n",
		"fractional memory":    "[vm]\nmemory_mib = 512.5\n",
		"zero disk":            "[vm]\nroot_disk_mib = 0\n",
		"negative disk":        "[vm]\nroot_disk_mib = -1\n",
		"disk overflow":        "[vm]\nroot_disk_mib = 4294967296\n",
		"zero Docker disk":     "[vm]\ndocker_disk_mib = 0\n",
		"negative Docker disk": "[vm]\ndocker_disk_mib = -1\n",
		"Docker disk overflow": "[vm]\ndocker_disk_mib = 4294967296\n",
		"malformed TOML":       "[network",
		"wrong type":           "[network]\nallowed_hosts = 'github.com'\n",
		"unknown field":        "[network]\nallowed_hosts = []\nallow_hosts = ['github.com']\n",
		"URL":                  "[network]\nallowed_hosts = ['https://github.com']\n",
		"wildcard":             "[network]\nallowed_hosts = ['*.github.com']\n",
		"IP":                   "[network]\nallowed_hosts = ['1.1.1.1']\n",
		"port":                 "[network]\nallowed_hosts = ['github.com:443']\n",
		"policy group":         "[network]\nallowed_hosts = ['public']\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			writeSettings(t, contents)
			_, err := launcherconfig.LoadSettings()
			require.Error(t, err)
		})
	}
}

func TestExampleSettingsLoad(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	data, err := os.ReadFile("../docs/config.example.toml")
	require.NoError(t, err)
	writeSettings(t, string(data))
	_, err = launcherconfig.LoadSettings()
	require.NoError(t, err)
}
