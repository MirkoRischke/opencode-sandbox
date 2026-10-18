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

func hostTUIConfigPath(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "opencode", "cli.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	return dir, path
}

func syncTUIConfig(t *testing.T, s *sandbox.Sandbox, configDir string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	config, err := sandbox.LoadCLIConfig("", configDir)
	require.NoError(t, err)
	require.NoError(t, s.SyncTUIConfig(ctx, config))
}
