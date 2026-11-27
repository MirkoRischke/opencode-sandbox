package gateway_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	gateway "opencode-sandbox/llm-gateway"

	"github.com/stretchr/testify/require"
)

func fakeDocker(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "docker")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0700))
	return path
}

func TestEnsureRefreshesComposeAndReconcilesHealthyProject(t *testing.T) {
	t.Parallel()
	docker := fakeDocker(t, `echo "$*" >> calls
case "$*" in
  *" ps "*)
    if test -f ready; then printf 'litellm healthy\ndb healthy\n'; fi ;;
  *" up "*) touch ready ;;
  *" stop") rm -f ready ;;
esac
`)
	g := gateway.NewWithOptions(t.TempDir(), gateway.Options{DockerPath: docker})
	log := filepath.Join(g.Directory(), "calls")
	require.NoError(t, g.Ensure(context.Background(), io.Discard))
	composePath := filepath.Join(g.Directory(), "compose.yaml")
	firstCompose, err := os.ReadFile(composePath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(composePath, []byte("outdated compose"), 0600))
	overridePath := filepath.Join(g.Directory(), "compose.config.yaml")
	firstOverride, err := os.ReadFile(overridePath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(overridePath, []byte("outdated override"), 0600))
	require.NoFileExists(t, filepath.Join(g.Directory(), "config.yaml"))
	firstEnv, err := os.ReadFile(filepath.Join(g.Directory(), ".env"))
	require.NoError(t, err)
	require.NoError(t, g.Ensure(context.Background(), io.Discard))
	secondCompose, err := os.ReadFile(composePath)
	require.NoError(t, err)
	require.Equal(t, firstCompose, secondCompose, "startup must restore the bundled Compose file")
	override, err := os.ReadFile(overridePath)
	require.NoError(t, err)
	require.Equal(t, firstOverride, override, "startup must restore the bundled Compose override")
	secondEnv, err := os.ReadFile(filepath.Join(g.Directory(), ".env"))
	require.NoError(t, err)
	require.True(t, bytes.Equal(firstEnv, secondEnv), "gateway secrets must survive later launches")
	calls, err := os.ReadFile(log)
	require.NoError(t, err)
	require.Equal(t, 2, strings.Count(string(calls), "up --wait --wait-timeout 180"))
	env, err := os.Stat(filepath.Join(g.Directory(), ".env"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), env.Mode().Perm())
	require.NoError(t, g.Stop(context.Background(), io.Discard))
	require.NoFileExists(t, filepath.Join(g.Directory(), "ready"))
	afterStop, err := os.ReadFile(filepath.Join(g.Directory(), ".env"))
	require.NoError(t, err)
	require.True(t, bytes.Equal(firstEnv, afterStop), "stopping must preserve credentials")
	require.NoError(t, g.Ensure(context.Background(), io.Discard))
	calls, err = os.ReadFile(log)
	require.NoError(t, err)
	require.Equal(t, 3, strings.Count(string(calls), "up --wait --wait-timeout 180"))
	require.Contains(t, string(calls), " stop\n")
	require.NotContains(t, string(calls), " down")
}

func TestOptionalConfigIsSelectedAndReconciledOnStartup(t *testing.T) {
	t.Parallel()
	docker := fakeDocker(t, `printf '%s\n%s\n%s\n' "$*" "$OPENCODE_SANDBOX_GATEWAY_CONFIG" "$OPENCODE_SANDBOX_GATEWAY_CONFIG_SHA256" >> calls
`)
	configPath := filepath.Join(t.TempDir(), "config with spaces.yaml")
	g := gateway.NewWithOptions(t.TempDir(), gateway.Options{ConfigFile: configPath, DockerPath: docker})
	log := filepath.Join(g.Directory(), "calls")

	// A missing optional file must not be created or mounted.
	require.NoError(t, g.Ensure(t.Context(), io.Discard))
	calls, err := os.ReadFile(log)
	require.NoError(t, err)
	require.NotContains(t, string(calls), "compose.config.yaml")
	require.NoFileExists(t, configPath)

	for _, config := range []string{"model_list: []\n", "model_list: []\n# changed\n"} {
		require.NoError(t, os.WriteFile(configPath, []byte(config), 0600))
		require.NoError(t, os.WriteFile(log, nil, 0600))
		require.NoError(t, g.Ensure(t.Context(), io.Discard))
		calls, err = os.ReadFile(log)
		require.NoError(t, err)
		require.Contains(t, string(calls), "--file "+filepath.Join(g.Directory(), "compose.config.yaml"))
		require.Contains(t, string(calls), "\n"+configPath+"\n")
		require.Contains(t, string(calls), fmt.Sprintf("%x", sha256.Sum256([]byte(config))))
		data, err := os.ReadFile(configPath)
		require.NoError(t, err)
		require.Equal(t, config, string(data), "user config is read, never overwritten")
	}

	require.NoError(t, os.Remove(configPath))
	require.NoError(t, os.WriteFile(log, nil, 0600))
	require.NoError(t, g.Ensure(t.Context(), io.Discard))
	calls, err = os.ReadFile(log)
	require.NoError(t, err)
	require.NotContains(t, string(calls), "compose.config.yaml", "removing the config must remove the override")

	// A bad config path must fail startup, but must not prevent stopping.
	require.NoError(t, os.Mkdir(configPath, 0700))
	require.NoError(t, os.WriteFile(log, nil, 0600))
	require.ErrorContains(t, g.Ensure(t.Context(), io.Discard), "must be a regular file")
	calls, err = os.ReadFile(log)
	require.NoError(t, err)
	require.Empty(t, calls, "invalid config must not invoke Docker")
	require.NoError(t, g.Stop(t.Context(), io.Discard))
	calls, err = os.ReadFile(log)
	require.NoError(t, err)
	require.Contains(t, string(calls), " stop\n")
	require.NotContains(t, string(calls), "compose.config.yaml")
}

func TestStopBeforeFirstStartDoesNotCreateGatewayFiles(t *testing.T) {
	t.Parallel()
	g := gateway.New(t.TempDir())
	require.NoError(t, g.Stop(context.Background(), io.Discard))
	require.NoDirExists(t, g.Directory())
}

func TestAuthenticateStartsGatewayAndRunsSelectedProviderLogin(t *testing.T) {
	t.Parallel()
	docker := fakeDocker(t, `echo "$*" >> calls
case "$*" in *" exec "*) echo 'Complete device login in your browser';; esac
`)
	g := gateway.NewWithOptions(t.TempDir(), gateway.Options{DockerPath: docker})
	log := filepath.Join(g.Directory(), "calls")
	for _, provider := range []string{"chatgpt", "copilot"} {
		var output bytes.Buffer
		require.NoError(t, g.Authenticate(context.Background(), provider, nil, &output, io.Discard))
		require.Contains(t, output.String(), "Complete device login")
	}
	calls, err := os.ReadFile(log)
	require.NoError(t, err)
	require.Contains(t, string(calls), "litellm.llms.chatgpt.authenticator")
	require.Contains(t, string(calls), "litellm.llms.github_copilot.authenticator")
	require.Contains(t, string(calls), "exec -T litellm python -u -c")
	require.ErrorContains(t, g.Authenticate(context.Background(), "invalid", nil, io.Discard, io.Discard), "unknown gateway provider")
}

func TestConcurrentLaunchersSerializeGatewayReconciliation(t *testing.T) {
	t.Parallel()
	docker := fakeDocker(t, `case "$*" in
  *" ps "*) if test -f ready; then printf 'litellm healthy\ndb healthy\n'; fi ;;
  *" up "*)
    mkdir reconciling || exit 1
    echo up >> calls
    sleep 0.1
    touch ready
    rmdir reconciling ;;
esac
`)
	dataDir := t.TempDir()
	options := gateway.Options{DockerPath: docker}
	log := filepath.Join(gateway.NewWithOptions(dataDir, options).Directory(), "calls")
	var workers sync.WaitGroup
	errors := make(chan error, 4)
	for range 4 {
		workers.Go(func() { errors <- gateway.NewWithOptions(dataDir, options).Ensure(context.Background(), io.Discard) })
	}
	workers.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	calls, err := os.ReadFile(log)
	require.NoError(t, err)
	require.Equal(t, strings.Repeat("up\n", 4), string(calls))
}

func TestGatewayStartupFailureIsReturned(t *testing.T) {
	t.Parallel()
	docker := fakeDocker(t, "case \"$*\" in *\" up \"*) exit 17;; esac\n")
	err := gateway.NewWithOptions(t.TempDir(), gateway.Options{DockerPath: docker}).Ensure(context.Background(), io.Discard)
	require.ErrorContains(t, err, "start LLM gateway")
	require.ErrorContains(t, err, "exit status 17")
}
