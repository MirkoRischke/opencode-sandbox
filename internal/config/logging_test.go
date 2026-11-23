package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoggingSettingsLayers(t *testing.T) {
	t.Parallel()
	p := Paths{ConfigDir: t.TempDir(), StateDir: t.TempDir()}
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	repo := filepath.Join(root, "repo")
	require.NoError(t, os.Mkdir(repo, 0700))
	settings, err := p.LoadLoggingSettings(root)
	require.NoError(t, err)
	require.Equal(t, "info", settings.ConsoleLevel)
	global := filepath.Join(p.ConfigDir, "opencode-sandbox", "config.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(global), 0700))
	globalData := []byte("[logging]\nconsole_level = 'error'\n")
	require.NoError(t, os.WriteFile(global, globalData, 0600))
	global, err = filepath.EvalSymlinks(global)
	require.NoError(t, err)
	parent := filepath.Join(root, workspaceConfigName)
	parentData := []byte("[logging]\nconsole_level = 'warn'\n")
	require.NoError(t, os.WriteFile(parent, parentData, 0600))
	data := []byte("[logging]\nconsole_level = 'info'\n")
	local := filepath.Join(repo, workspaceConfigName)
	require.NoError(t, os.WriteFile(local, data, 0600))
	settings, err = p.LoadLoggingSettings(repo)
	require.NoError(t, err)
	require.Equal(t, "info", settings.ConsoleLevel, "unapproved global settings must not apply")
	require.NoError(t, trustConfig(p.StateDir, global, globalData, func() error { return nil }))
	settings, err = p.LoadLoggingSettings(repo)
	require.NoError(t, err)
	require.Equal(t, "error", settings.ConsoleLevel, "unapproved workspace settings must not apply")
	require.NoError(t, trustConfig(p.StateDir, parent, parentData, func() error { return nil }))
	require.NoError(t, trustConfig(p.StateDir, local, data, func() error { return nil }))
	settings, err = p.LoadLoggingSettings(repo)
	require.NoError(t, err)
	require.Equal(t, "info", settings.ConsoleLevel)
	require.NoError(t, os.WriteFile(local, []byte("[logging]\nconsole_level = 'debug'\n"), 0600))
	settings, err = p.LoadLoggingSettings(repo)
	require.NoError(t, err)
	require.Equal(t, "warn", settings.ConsoleLevel, "changed workspace settings need approval again")
}

func TestConsoleLevelValidation(t *testing.T) {
	t.Parallel()
	for _, level := range []string{"debug", "info", "warn", "error", "", "trace", "WARN"} {
		t.Run(level, func(t *testing.T) {
			t.Parallel()
			settings := DefaultSettings()
			settings.Logging.ConsoleLevel = level
			err := settings.Validate()
			if level == "" || level == "trace" || level == "WARN" {
				require.ErrorContains(t, err, "logging.console_level")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
