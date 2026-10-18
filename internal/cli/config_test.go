package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	launcherconfig "opencode-sandbox/internal/config"

	"github.com/stretchr/testify/require"
)

func TestConfigDirectoryUsesXDGWhenSet(t *testing.T) {
	t.Parallel()
	want := t.TempDir()
	dir, err := hostConfigDir(want, "")
	require.NoError(t, err)
	require.Equal(t, want, dir)
}

func TestStateDirectoryUsesXDGWhenSet(t *testing.T) {
	t.Parallel()
	want := t.TempDir()
	dir, err := hostStateDir(want, "")
	require.NoError(t, err)
	require.Equal(t, want, dir)
}

func TestSettingsUseHomeWhenXDGIsUnset(t *testing.T) {
	t.Parallel()
	dir, err := hostConfigDir("", t.TempDir())
	require.NoError(t, err)
	path := filepath.Join(dir, "opencode-sandbox", "config.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	require.NoError(t, os.WriteFile(path, []byte("[network]\nallowed_hosts = []\n"), 0600))
	settings, err := launcherconfig.LoadSettings(dir)
	require.NoError(t, err)
	require.Empty(t, settings.Network.AllowedHosts)
}

func TestRepoSettingsTrustFallsBackToHome(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	state, err := hostStateDir("", home)
	require.NoError(t, err)
	root := t.TempDir()
	output, err := exec.Command("git", "-C", root, "init").CombinedOutput()
	require.NoError(t, err, "%s", output)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".opencode-sandbox.toml"), []byte("# config\n"), 0600))
	paths := launcherconfig.Paths{ConfigDir: t.TempDir(), StateDir: state}
	_, err = paths.LoadRepoSettings(root, func(string, []byte, launcherconfig.Settings) error { return nil })
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(home, ".local/state/opencode-sandbox/trust.json"))
	require.NoError(t, err)
}
