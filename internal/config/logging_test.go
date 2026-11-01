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
	root := t.TempDir()
	settings, err := p.LoadLoggingSettings(root)
	require.NoError(t, err)
	require.Equal(t, "info", settings.ConsoleLevel)
	global := filepath.Join(p.ConfigDir, "opencode-sandbox", "config.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(global), 0700))
	require.NoError(t, os.WriteFile(global, []byte("[logging]\nconsole_level = 'error'\n"), 0600))
	data := []byte("[logging]\nconsole_level = 'info'\n")
	require.NoError(t, os.WriteFile(filepath.Join(root, workspaceConfigName), data, 0600))
	settings, err = p.LoadLoggingSettings(root)
	require.NoError(t, err)
	require.Equal(t, "error", settings.ConsoleLevel, "unapproved workspace settings must not apply")
	require.NoError(t, trustWorkspaceSettings(p.StateDir, root, data, func() error { return nil }))
	settings, err = p.LoadLoggingSettings(root)
	require.NoError(t, err)
	require.Equal(t, "info", settings.ConsoleLevel)
	require.NoError(t, os.WriteFile(filepath.Join(root, workspaceConfigName), []byte("[logging]\nconsole_level = 'debug'\n"), 0600))
	settings, err = p.LoadLoggingSettings(root)
	require.NoError(t, err)
	require.Equal(t, "error", settings.ConsoleLevel, "changed workspace settings need approval again")
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
