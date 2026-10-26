package config_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeSettingsAt(t *testing.T, configDir, contents string) {
	t.Helper()
	dir := filepath.Join(configDir, "opencode-sandbox")
	require.NoError(t, os.MkdirAll(dir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"), []byte(contents), 0600))
}

func runGit(t *testing.T, args ...string) {
	t.Helper()
	output, err := exec.Command("git", args...).CombinedOutput()
	require.NoError(t, err, "prepare Git fixture: %s", output)
}
