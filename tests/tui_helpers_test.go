package sandbox_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/require"
)

func hostTUIConfigPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "opencode", "cli.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	return path
}

func syncTUIConfig(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	config, err := sandbox.LoadCLIConfig("")
	require.NoError(t, err)
	require.NoError(t, s.SyncTUIConfig(ctx, config))
}
