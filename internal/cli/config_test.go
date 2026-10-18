package cli

import (
	"os"
	"path/filepath"
	"testing"

	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/require"
)

func TestConfigDirectoryUsesXDGWhenSet(t *testing.T) {
	t.Parallel()
	want := t.TempDir()
	dir, err := hostConfigDir(want, "")
	require.NoError(t, err)
	require.Equal(t, want, dir)
}

func TestSandboxConfigDefaultsToHomeConfigDirectory(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	path := filepath.Join(home, ".config", "opencode-sandbox", "opencode.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	require.NoError(t, os.WriteFile(path, []byte(`{}`), 0600))
	dir, err := hostConfigDir("", home)
	require.NoError(t, err)
	config, err := sandbox.LoadConfig("", dir)
	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(config))
}

func TestHostTUIConfigUsesHomeWhenXDGIsUnset(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	path := filepath.Join(home, ".config", "opencode", "cli.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	require.NoError(t, os.WriteFile(path, []byte(`{"mouse":false}`), 0600))
	dir, err := hostConfigDir("", home)
	require.NoError(t, err)
	config, err := sandbox.LoadCLIConfig("", dir)
	require.NoError(t, err)
	require.JSONEq(t, `{"mouse":false}`, string(config))
}
