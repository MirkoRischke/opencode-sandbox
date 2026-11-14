package gateway_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGateway(t *testing.T) {
	t.Parallel()
	stack := newGatewayStack(t)
	stack.command(t, "start")

	t.Run("ReportsDatabaseReadiness", func(t *testing.T) {
		status, body := stack.request(t, http.MethodGet, "/health/readiness", "", "")

		require.Equal(t, http.StatusOK, status)
		var readiness struct {
			Status string `json:"status"`
			DB     string `json:"db"`
		}
		require.NoError(t, json.Unmarshal(body, &readiness))
		require.Equal(t, "healthy", readiness.Status)
		require.Equal(t, "connected", readiness.DB)
	})

	t.Run("StartsWithoutConfig", func(t *testing.T) {
		key := stack.newKey(t)
		status, body := stack.request(t, http.MethodGet, "/v1/models", key, "")
		require.Equal(t, http.StatusOK, status)
		var models struct {
			Data []any `json:"data"`
		}
		require.NoError(t, json.Unmarshal(body, &models))
		require.Empty(t, models.Data, "a fresh gateway must not invent a model catalog")
		require.NoFileExists(t, stack.config)
	})

	t.Run("LoadsAndReloadsUserConfig", func(t *testing.T) {
		require.NoError(t, os.MkdirAll(filepath.Dir(stack.config), 0700))
		for _, model := range []string{"test-model-first", "test-model-edited"} {
			config := "model_list:\n  - model_name: " + model + "\n    litellm_params:\n      model: openai/gpt-6-luna\n      api_key: sk-test\n"
			require.NoError(t, os.WriteFile(stack.config, []byte(config), 0600))
			stack.command(t, "start")
			key := stack.newKey(t)
			status, body := stack.request(t, http.MethodGet, "/v1/models", key, "")
			require.Equal(t, http.StatusOK, status)
			require.Contains(t, string(body), `"`+model+`"`)
			if model == "test-model-edited" {
				require.NotContains(t, string(body), `"test-model-first"`)
			}
		}
	})

	t.Run("RejectsInvalidKey", func(t *testing.T) {
		status, _ := stack.request(t, http.MethodGet, "/v1/models", "sk-invalid", "")

		require.Equal(t, http.StatusUnauthorized, status)
	})

	t.Run("VirtualKeyCannotCreateTeams", func(t *testing.T) {
		key := stack.newKey(t)
		stack.requireKeyWorks(t, key)

		status, _ := stack.request(t, http.MethodPost, "/team/new", key, `{"team_alias":"forbidden"}`)

		require.Equal(t, http.StatusForbidden, status)
	})

	t.Run("DeletedKeyCannotAuthenticate", func(t *testing.T) {
		key := stack.newKey(t)
		stack.requireKeyWorks(t, key)

		err := stack.gateway.DeleteKey(t.Context(), strings.ReplaceAll(t.Name(), "/", "-"))

		require.NoError(t, err)
		status, _ := stack.request(t, http.MethodGet, "/v1/models", key, "")
		require.Equal(t, http.StatusUnauthorized, status)
	})

	t.Run("CredentialsSurviveContainerRecreation", func(t *testing.T) {
		key := stack.newKey(t)
		stack.requireKeyWorks(t, key)

		stack.command(t, "stop")
		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()
		// Removing stopped containers (not volumes) proves data was persisted,
		// rather than merely retained in a container's writable layer.
		output, err := stack.compose(ctx, "rm", "--force")
		require.NoError(t, err, "%s", stack.redact(output))
		stack.command(t, "start")

		stack.requireKeyWorks(t, key)
	})

	t.Run("RemovingConfigStartsWithoutFileModels", func(t *testing.T) {
		require.NoError(t, os.Remove(stack.config))
		stack.command(t, "start")
		key := stack.newKey(t)
		status, body := stack.request(t, http.MethodGet, "/v1/models", key, "")
		require.Equal(t, http.StatusOK, status)
		require.NotContains(t, string(body), `"test-model-edited"`)
	})

	t.Run("DatabaseModelsWorkWithoutConfig", func(t *testing.T) {
		master, err := os.ReadFile(filepath.Join(stack.gateway.Directory(), ".env"))
		require.NoError(t, err)
		var masterKey string
		for _, line := range strings.Split(string(master), "\n") {
			if value, ok := strings.CutPrefix(line, "LITELLM_MASTER_KEY="); ok {
				masterKey = value
			}
		}
		require.NotEmpty(t, masterKey)
		status, _ := stack.request(t, http.MethodPost, "/model/new", masterKey,
			`{"model_name":"database-model","litellm_params":{"model":"openai/gpt-6-luna","api_key":"sk-test"}}`)
		require.Equal(t, http.StatusOK, status)
		stack.command(t, "stop")
		stack.command(t, "start")
		key := stack.newKey(t)
		status, body := stack.request(t, http.MethodGet, "/v1/models", key, "")
		require.Equal(t, http.StatusOK, status)
		require.Contains(t, string(body), `"database-model"`)
	})

	t.Run("InvalidConfigFailsStartupButDoesNotPreventStop", func(t *testing.T) {
		require.NoError(t, os.WriteFile(stack.config, []byte("model_list: [\n"), 0600))
		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()
		command := exec.CommandContext(ctx, stack.binary, "gateway", "start")
		command.Env = stack.env
		output, err := command.CombinedOutput()
		require.Error(t, err, "invalid YAML must not be silently ignored")
		require.Contains(t, stack.redact(output), "start LLM gateway")
		stack.command(t, "stop")
	})
}
