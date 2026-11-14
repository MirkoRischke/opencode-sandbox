package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

//nolint:forbidigo // Serial environment overrides isolate both XDG roots and the Docker executable.
func TestHostGatewayUsesOptionalXDGConfig(t *testing.T) {
	for _, xdg := range []bool{false, true} {
		t.Run(map[bool]string{false: "home defaults", true: "XDG overrides"}[xdg], func(t *testing.T) {
			home := t.TempDir()
			configDir := filepath.Join(home, ".config")
			dataDir := filepath.Join(home, ".local", "share")
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", "")
			t.Setenv("XDG_DATA_HOME", "")
			t.Setenv("OPENCODE_SANDBOX_GATEWAY_PROJECT", "")
			t.Setenv("OPENCODE_SANDBOX_GATEWAY_PORT", "")
			if xdg {
				configDir = t.TempDir()
				dataDir = t.TempDir()
				t.Setenv("XDG_CONFIG_HOME", configDir)
				t.Setenv("XDG_DATA_HOME", dataDir)
			}
			configPath := filepath.Join(configDir, "opencode-sandbox", "llm-gateway", "config.yaml")
			require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0700))
			require.NoError(t, os.WriteFile(configPath, []byte("model_list: []\n"), 0600))
			bin := t.TempDir()
			log := filepath.Join(bin, "calls")
			require.NoError(t, os.WriteFile(filepath.Join(bin, "docker"), []byte(`#!/bin/sh
printf '%s\n%s\n' "$*" "$OPENCODE_SANDBOX_GATEWAY_CONFIG" > "$CALL_LOG"
`), 0700))
			t.Setenv("CALL_LOG", log)
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			g, err := hostGateway()
			require.NoError(t, err)
			require.Equal(t, filepath.Join(dataDir, "opencode-sandbox", "llm-gateway"), g.Directory())
			_, err = executeCommand("gateway", "start")
			require.NoError(t, err)
			calls, err := os.ReadFile(log)
			require.NoError(t, err)
			require.Contains(t, string(calls), "compose.config.yaml")
			require.Contains(t, string(calls), "\n"+configPath+"\n")
		})
	}
}
