package gateway_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	gateway "opencode-sandbox/llm-gateway"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type gatewayStack struct {
	gateway *gateway.Gateway
	binary  string
	project string
	baseURL string
	env     []string
	keys    []string
	client  *http.Client
}

func newGatewayStack(t *testing.T) *gatewayStack {
	t.Helper()
	dir := t.TempDir()
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	address, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok)
	port := address.Port
	require.NoError(t, listener.Close())
	project := "gateway-test-" + strings.ToLower(rand.Text())
	options := gateway.Options{ProjectName: project, Port: uint16(port)}
	stack := &gatewayStack{
		gateway: gateway.NewWithOptions(dir, options),
		binary:  filepath.Join(dir, "launcher"),
		project: project,
		baseURL: fmt.Sprintf("http://127.0.0.1:%d", port),
		client:  &http.Client{Timeout: 15 * time.Second},
	}
	// Never inherit real gateway credentials or launcher configuration. Preserve
	// Docker connection settings.
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		if strings.HasPrefix(name, "XDG_") || strings.HasPrefix(name, "OPENCODE_SANDBOX_GATEWAY_") ||
			strings.HasPrefix(name, "COMPOSE_") || name == "LITELLM_MASTER_KEY" ||
			name == "LITELLM_SALT_KEY" || name == "POSTGRES_PASSWORD" {
			continue
		}
		stack.env = append(stack.env, value)
	}
	stack.env = append(stack.env,
		"XDG_DATA_HOME="+dir, "XDG_CONFIG_HOME="+dir, "XDG_STATE_HOME="+dir,
		"OPENCODE_SANDBOX_GATEWAY_PROJECT="+project,
		"OPENCODE_SANDBOX_GATEWAY_PORT="+strconv.Itoa(port))
	// A regression in CLI option handling must fail safely, not reconcile the
	// developer's shared project. This guard delegates to the real Docker binary.
	docker, err := exec.LookPath("docker")
	require.NoError(t, err)
	bin := filepath.Join(dir, "bin")
	require.NoError(t, os.Mkdir(bin, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(bin, "docker"), []byte(`#!/bin/sh
project=
previous=
for argument do
  if [ "$previous" = --project-name ]; then project="$argument"; fi
  previous="$argument"
done
if [ "$project" != "$OPENCODE_SANDBOX_GATEWAY_PROJECT" ]; then
  echo 'Refusing Docker invocation outside the isolated test project' >&2
  exit 1
fi
exec "$GATEWAY_TEST_DOCKER" "$@"
`), 0700))
	stack.env = append(stack.env, "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GATEWAY_TEST_DOCKER="+docker)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-o", stack.binary, "./cmd/opencode-sandbox")
	build.Dir = "../.."
	output, err := build.CombinedOutput()
	require.NoError(t, err, "build launcher: %s", output)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if t.Failed() {
			logs, err := stack.compose(ctx, "logs", "--no-color", "--tail", "100")
			if err != nil {
				t.Logf("collect gateway logs: %v", err)
			}
			t.Logf("gateway logs: %s", stack.redact(logs))
		}
		output, err := stack.compose(ctx, "down", "--volumes", "--remove-orphans", "--timeout", "5")
		assert.NoError(t, err, "clean up isolated gateway: %s", stack.redact(output))
	})
	return stack
}

func (s *gatewayStack) command(t *testing.T, action string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, s.binary, "gateway", action)
	command.Env = s.env
	command.Dir = filepath.Dir(s.binary)
	output, err := command.CombinedOutput()
	require.NoError(t, err, "gateway %s: %s", action, s.redact(output))
}

func (s *gatewayStack) compose(ctx context.Context, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "docker", append([]string{
		"compose", "--project-name", s.project,
		"--file", filepath.Join(s.gateway.Directory(), "compose.yaml"),
		"--env-file", filepath.Join(s.gateway.Directory(), ".env"),
	}, args...)...)
	command.Env = s.env
	return command.CombinedOutput()
}

func (s *gatewayStack) redact(output []byte) string {
	text := string(output)
	if env, err := os.ReadFile(filepath.Join(s.gateway.Directory(), ".env")); err == nil {
		for _, line := range strings.Split(string(env), "\n") {
			if _, value, ok := strings.Cut(line, "="); ok && value != "" {
				text = strings.ReplaceAll(text, value, "<REDACTED>")
			}
		}
	}
	for _, key := range s.keys {
		text = strings.ReplaceAll(text, key, "<REDACTED>")
	}
	return text
}

func (s *gatewayStack) request(t *testing.T, method, path, key, body string) (int, []byte) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), method, s.baseURL+path, strings.NewReader(body))
	require.NoError(t, err)
	if key != "" {
		request.Header.Set("Authorization", "Bearer "+key)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	require.NoError(t, err)
	defer func() { require.NoError(t, response.Body.Close()) }()
	data, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return response.StatusCode, data
}

func (s *gatewayStack) newKey(t *testing.T) string {
	t.Helper()
	name := strings.ReplaceAll(t.Name(), "/", "-")
	team, err := s.gateway.EnsureTeam(t.Context(), name, name)
	require.NoError(t, err)
	key, err := s.gateway.EnsureKey(t.Context(), team, name)
	require.NoError(t, err)
	s.keys = append(s.keys, key)
	return key
}

func (s *gatewayStack) requireKeyWorks(t *testing.T, key string) {
	t.Helper()
	status, _ := s.request(t, http.MethodGet, "/v1/models", key, "")
	require.Equal(t, http.StatusOK, status, "provisioned key must authenticate")
}
