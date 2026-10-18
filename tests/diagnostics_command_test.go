package sandbox_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

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
	command := exec.CommandContext(ctx, buildLauncher(t), "diagnostics", "network")
	command.Dir = root
	command.Env = append(os.Environ(), "HOME="+home, "XDG_STATE_HOME=", "XDG_CONFIG_HOME="+config, "HERDR_ENV=")
	output, err := command.CombinedOutput()
	require.NoError(t, err, "%s", output)
	require.Equal(t, data, output)
}
