package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testGateway(t *testing.T, handler http.HandlerFunc) *Gateway {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	g := New(t.TempDir())
	g.baseURL = server.URL
	g.client = server.Client()
	require.NoError(t, os.MkdirAll(g.directory, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(g.directory, ".env"), []byte("LITELLM_MASTER_KEY=sk-test-admin\n"), 0600))
	return g
}

func TestTeamAndVMKeysAreCreatedOnceAndReused(t *testing.T) {
	t.Parallel()
	var calls []string
	g := testGateway(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer sk-test-admin", r.Header.Get("Authorization"))
		assert.Equal(t, http.MethodPost, r.Method)
		calls = append(calls, r.URL.Path)
		var body map[string]any
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&body)) {
			return
		}
		switch r.URL.Path {
		case "/team/new":
			assert.Equal(t, "oc-repo-id", body["team_id"])
			assert.Equal(t, "my-repo", body["team_alias"])
		case "/key/generate":
			assert.Equal(t, "oc-repo-id", body["team_id"])
			assert.Equal(t, "llm_api", body["key_type"])
			assert.Equal(t, []any{"all-proxy-models"}, body["models"])
			alias, ok := body["key_alias"].(string)
			if !assert.True(t, ok) {
				return
			}
			assert.NoError(t, json.NewEncoder(w).Encode(map[string]string{"key": "sk-" + alias}))
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
	})
	ctx := context.Background()
	for range 2 {
		team, err := g.EnsureTeam(ctx, "repo-id", "my-repo")
		require.NoError(t, err)
		for _, vm := range []string{"oc-main", "oc-feature"} {
			key, err := g.EnsureKey(ctx, team, vm)
			require.NoError(t, err)
			require.Equal(t, "sk-"+vm, key)
		}
	}
	require.Equal(t, []string{"/team/new", "/key/generate", "/key/generate"}, calls)
	info, err := os.Stat(g.keyPath("oc-main"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
	info, err = os.Stat(filepath.Dir(g.keyPath("oc-main")))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0700), info.Mode().Perm())
}

func TestConcurrentLaunchesReuseOneKey(t *testing.T) {
	t.Parallel()
	var requests int
	g := testGateway(t, func(w http.ResponseWriter, _ *http.Request) {
		requests++
		assert.NoError(t, json.NewEncoder(w).Encode(map[string]string{"key": "sk-vm"}))
	})
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			key, err := g.EnsureKey(context.Background(), "team", "oc-vm")
			require.NoError(t, err)
			require.Equal(t, "sk-vm", key)
		})
	}
	wg.Wait()
	require.Equal(t, 1, requests)
}

func TestRecreatingVMGeneratesANewKey(t *testing.T) {
	t.Parallel()
	var generated int
	g := testGateway(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/key/generate" {
			generated++
			key := "sk-first"
			if generated > 1 {
				key = "sk-recreated"
			}
			assert.NoError(t, json.NewEncoder(w).Encode(map[string]string{"key": key}))
		}
	})
	ctx := context.Background()
	key, err := g.EnsureKey(ctx, "team", "oc-vm")
	require.NoError(t, err)
	require.Equal(t, "sk-first", key)
	require.NoError(t, g.DeleteKey(ctx, "oc-vm"))
	key, err = g.EnsureKey(ctx, "team", "oc-vm")
	require.NoError(t, err)
	require.Equal(t, "sk-recreated", key)
	require.Equal(t, 2, generated)
}

func TestDeleteKeyRemovesRemoteAndLocalCredential(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusOK, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls int
			g := testGateway(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				assert.Equal(t, "/key/delete", r.URL.Path)
				var body struct {
					Keys []string `json:"keys"`
				}
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				assert.Equal(t, []string{"sk-vm"}, body.Keys)
				w.WriteHeader(status)
			})
			require.NoError(t, os.MkdirAll(filepath.Dir(g.keyPath("oc-vm")), 0700))
			require.NoError(t, os.WriteFile(g.keyPath("oc-vm"), []byte("sk-vm"), 0600))
			err := g.DeleteKey(context.Background(), "oc-vm")
			if status == http.StatusOK {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "delete it manually in LiteLLM")
			}
			require.NoFileExists(t, g.keyPath("oc-vm"))
			require.NoError(t, g.DeleteKey(context.Background(), "oc-vm"))
			require.Equal(t, 1, calls, "deletion is never retried")
		})
	}
}

func TestProvisioningFailureDoesNotStoreOrDiscloseKey(t *testing.T) {
	t.Parallel()
	g := testGateway(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, err := w.Write([]byte(`{"error":"sk-sensitive-echo"}`))
		assert.NoError(t, err)
	})
	_, err := g.EnsureKey(context.Background(), "team", "oc-vm")
	require.ErrorContains(t, err, "HTTP 403")
	require.NotContains(t, err.Error(), "sk-sensitive-echo")
	require.NotContains(t, err.Error(), "sk-test-admin")
	require.NoFileExists(t, g.keyPath("oc-vm"))
}
