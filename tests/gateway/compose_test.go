package gateway_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestComposeDefinitionWithAndWithoutConfig(t *testing.T) {
	t.Parallel()
	for _, withConfig := range []bool{false, true} {
		t.Run(map[bool]string{false: "without config", true: "with config"}[withConfig], func(t *testing.T) {
			t.Parallel()
			dir, err := filepath.Abs("../../llm-gateway")
			require.NoError(t, err)
			configPath := filepath.Join(t.TempDir(), "config with spaces.yaml")
			require.NoError(t, os.WriteFile(configPath, []byte("model_list: []\n"), 0600))
			envFile := filepath.Join(t.TempDir(), ".env")
			require.NoError(t, os.WriteFile(envFile, nil, 0600))
			args := []string{"compose", "--env-file", envFile, "--file", filepath.Join(dir, "compose.yaml")}
			if withConfig {
				args = append(args, "--file", filepath.Join(dir, "compose.config.yaml"))
			}
			command := exec.CommandContext(t.Context(), "docker", append(args, "config", "--format", "json")...)
			command.Env = append(os.Environ(), "LITELLM_MASTER_KEY=sk-test", "LITELLM_SALT_KEY=sk-test",
				"POSTGRES_PASSWORD=test", "OPENCODE_SANDBOX_GATEWAY_CONFIG="+configPath,
				"OPENCODE_SANDBOX_GATEWAY_CONFIG_SHA256=test-checksum")
			output, err := command.CombinedOutput()
			require.NoError(t, err, "%s", output)
			var project struct {
				Services map[string]struct {
					Command []string          `json:"command"`
					Labels  map[string]string `json:"labels"`
					Volumes []struct {
						Source   string `json:"source"`
						Target   string `json:"target"`
						ReadOnly bool   `json:"read_only"`
						Bind     struct {
							CreateHostPath bool `json:"create_host_path"`
						} `json:"bind"`
					} `json:"volumes"`
				} `json:"services"`
			}
			require.NoError(t, json.Unmarshal(output, &project))
			service := project.Services["litellm"]
			require.Len(t, service.Volumes, map[bool]int{false: 2, true: 3}[withConfig])
			if !withConfig {
				require.Equal(t, []string{"--port", "4000"}, service.Command)
				return
			}
			require.Equal(t, []string{"--config", "/app/config.yaml", "--port", "4000"}, service.Command)
			require.Equal(t, "test-checksum", service.Labels["opencode-sandbox.llm-config-sha256"])
			for _, volume := range service.Volumes {
				if volume.Target == "/app/config.yaml" {
					require.Equal(t, configPath, volume.Source)
					require.True(t, volume.ReadOnly)
					require.False(t, volume.Bind.CreateHostPath)
					return
				}
			}
			t.Fatal("config bind mount missing")
		})
	}
}
