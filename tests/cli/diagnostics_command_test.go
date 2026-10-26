package sandbox_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"opencode-sandbox/internal/logging"

	"github.com/stretchr/testify/require"
)

func TestLauncherDiagnosticsUsesHomeFallbackWithoutLoadingSettings(t *testing.T) {
	t.Parallel()
	root := newWorktree(t)
	home := t.TempDir()
	config := t.TempDir()
	writeSettingsAt(t, config, "invalid = [")
	require.NoError(t, os.WriteFile(filepath.Join(root, ".opencode-sandbox.toml"), []byte("invalid = ["), 0600))
	identity := sha256.Sum256([]byte(root))
	path := filepath.Join(home, ".local", "state", "opencode-sandbox", "diagnostics", fmt.Sprintf("%x", identity[:16]), "network.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	data := []byte("{\n  \"denied_targets\": []\n}\n")
	require.NoError(t, os.WriteFile(path, data, 0600))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, buildLauncher(t), "--console-log-level", "warn", "diagnostics", "network")
	command.Dir = root
	command.Env = append(os.Environ(), "HOME="+home, "XDG_STATE_HOME=", "XDG_CONFIG_HOME="+config, "HERDR_ENV=")
	output, err := command.CombinedOutput()
	require.NoError(t, err, "%s", output)
	require.Equal(t, data, output)
}

func TestLauncherLogLocationDoesNotLoadOrCreateState(t *testing.T) {
	t.Parallel()
	root := newWorktree(t)
	alias := filepath.Join(t.TempDir(), "workspace-link")
	require.NoError(t, os.Symlink(root, alias))
	config := t.TempDir()
	writeSettingsAt(t, config, "invalid = [")
	require.NoError(t, os.WriteFile(filepath.Join(root, ".opencode-sandbox.toml"), []byte("invalid = ["), 0600))
	for _, tc := range []struct{ name, directory, xdg string }{
		{"xdg", root, t.TempDir()},
		{"home", root, ""},
		{"symlink", alias, t.TempDir()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			state := tc.xdg
			if state == "" {
				state = filepath.Join(home, ".local", "state")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, buildLauncher(t), "--console-log-level", "warn", "diagnostics", "logs")
			command.Dir = tc.directory
			command.Env = append(os.Environ(), "HOME="+home, "XDG_STATE_HOME="+tc.xdg, "XDG_CONFIG_HOME="+config, "HERDR_ENV=")
			output, err := command.CombinedOutput()
			require.NoError(t, err, "%s", output)
			require.Equal(t, logging.Directory(state, root), strings.TrimSpace(string(output)))
			_, err = os.Stat(filepath.Join(state, "opencode-sandbox"))
			require.True(t, os.IsNotExist(err))
		})
	}
}
