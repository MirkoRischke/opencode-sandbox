package gateway_test

import (
	"context"
	"encoding/json"
	"net/http"
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

	t.Run("ProvisionedKeyCanListConfiguredModels", func(t *testing.T) {
		key := stack.newKey(t)

		status, body := stack.request(t, http.MethodGet, "/v1/models", key, "")

		require.Equal(t, http.StatusOK, status)
		var models struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(body, &models))
		var ids []string
		for _, model := range models.Data {
			ids = append(ids, model.ID)
		}
		require.Contains(t, ids, "chatgpt/gpt-6-luna")
		require.Contains(t, ids, "copilot/gpt-6-luna")
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
}
