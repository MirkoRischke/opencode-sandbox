package gateway_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDevelopmentComposeSelectsOptionalConfig(t *testing.T) {
	t.Parallel()
	for _, xdg := range []bool{false, true} {
		t.Run(map[bool]string{false: "home defaults", true: "XDG overrides"}[xdg], func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			configRoot := filepath.Join(home, ".config")
			xdgRoot := ""
			if xdg {
				configRoot = filepath.Join(t.TempDir(), "config with spaces")
				xdgRoot = configRoot
			}
			bin := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(bin, "docker"), []byte(`#!/bin/sh
printf '%s\n' "$@" "$OPENCODE_SANDBOX_GATEWAY_CONFIG" "$OPENCODE_SANDBOX_GATEWAY_CONFIG_SHA256"
`), 0700))
			env := append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+xdgRoot,
				"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
				"OPENCODE_SANDBOX_GATEWAY_CONFIG=", "OPENCODE_SANDBOX_GATEWAY_CONFIG_SHA256=")
			run := func(action string) (string, error) {
				command := exec.CommandContext(t.Context(), "bash", "compose.sh", action)
				command.Env = env
				output, err := command.CombinedOutput()
				return string(output), err
			}
			output, err := run("up")
			require.NoError(t, err, output)
			require.NotContains(t, output, "compose.config.yaml")
			configPath := filepath.Join(configRoot, "opencode-sandbox", "llm-gateway", "config.yaml")
			require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0700))
			config := []byte("model_list: []\n")
			require.NoError(t, os.WriteFile(configPath, config, 0600))
			output, err = run("up")
			require.NoError(t, err, output)
			require.Contains(t, output, "compose.config.yaml\n")
			require.Contains(t, output, "\n"+configPath+"\n")
			require.Contains(t, output, fmt.Sprintf("%x", sha256.Sum256(config)))

			require.NoError(t, os.Remove(configPath))
			require.NoError(t, os.Mkdir(configPath, 0700))
			output, err = run("up")
			require.Error(t, err)
			require.Contains(t, output, "readable regular file")
			output, err = run("stop")
			require.NoError(t, err, output)
			require.NotContains(t, output, "compose.config.yaml")
		})
	}
}
