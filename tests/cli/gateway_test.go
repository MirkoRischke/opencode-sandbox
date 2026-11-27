package sandbox_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLauncherGatewayUsesOptionalXDGConfig(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		xdg  bool
	}{{"home defaults", false}, {"XDG overrides", true}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			configDir := filepath.Join(home, ".config")
			dataDir := filepath.Join(home, ".local", "share")
			var xdgConfig, xdgData string
			if tc.xdg {
				configDir, dataDir = t.TempDir(), t.TempDir()
				xdgConfig, xdgData = configDir, dataDir
			}
			configPath := filepath.Join(configDir, "opencode-sandbox", "llm-gateway", "config.yaml")
			require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0700))
			require.NoError(t, os.WriteFile(configPath, []byte("model_list: []\n"), 0600))
			bin := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(bin, "docker"), []byte(`#!/bin/sh
printf '%s\n%s\n' "$*" "$OPENCODE_SANDBOX_GATEWAY_CONFIG" > calls
`), 0700))
			command := exec.CommandContext(t.Context(), buildLauncher(t), "--console-log-level", "warn", "gateway", "start")
			command.Env = append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+xdgConfig, "XDG_DATA_HOME="+xdgData,
				"OPENCODE_SANDBOX_GATEWAY_PROJECT=", "OPENCODE_SANDBOX_GATEWAY_PORT=", "PATH="+bin)
			output, err := command.CombinedOutput()
			require.NoError(t, err, "%s", output)
			calls, err := os.ReadFile(filepath.Join(dataDir, "opencode-sandbox", "llm-gateway", "calls"))
			require.NoError(t, err)
			require.Contains(t, string(calls), "compose.config.yaml")
			require.Contains(t, string(calls), "\n"+configPath+"\n")
		})
	}
}

func TestGatewayRejectsInvalidPortBeforeStartingDocker(t *testing.T) {
	t.Parallel()
	for _, port := range []string{"0", "65536", "not-a-port"} {
		t.Run(port, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, buildLauncher(t), "--console-log-level", "warn", "gateway", "start")
			command.Env = append(os.Environ(), "XDG_DATA_HOME="+dir,
				"OPENCODE_SANDBOX_GATEWAY_PORT="+port, "PATH="+t.TempDir())

			output, err := command.CombinedOutput()

			require.Error(t, err)
			require.Contains(t, string(output), "OPENCODE_SANDBOX_GATEWAY_PORT")
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			require.Empty(t, entries)
		})
	}
}
