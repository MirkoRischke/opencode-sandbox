package gateway_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	gateway "opencode-sandbox/llm-gateway"

	"github.com/stretchr/testify/require"
)

// The fake Docker executable observes the external Compose boundary, without
// requiring Docker or touching the developer's shared gateway.
//
//nolint:forbidigo // The external executable seam needs process PATH; these tests run serially and never alter the real gateway.
func TestEnsureRefreshesComposeAndReconcilesHealthyProject(t *testing.T) {
	// PATH is process-local; this test must not run in parallel.
	bin := t.TempDir()
	log := filepath.Join(bin, "calls")
	require.NoError(t, os.WriteFile(filepath.Join(bin, "docker"), []byte(`#!/bin/sh
echo "$*" >> "$CALL_LOG"
case "$*" in
  *" ps "*)
    if test -f ready; then printf 'litellm healthy\ndb healthy\n'; fi ;;
  *" up "*) touch ready ;;
  *" stop") rm -f ready ;;
esac
`), 0700))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CALL_LOG", log)
	g := gateway.New(t.TempDir())
	require.NoError(t, g.Ensure(context.Background(), io.Discard))
	composePath := filepath.Join(g.Directory(), "compose.yaml")
	firstCompose, err := os.ReadFile(composePath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(composePath, []byte("outdated compose"), 0600))
	configPath := filepath.Join(g.Directory(), "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("custom config"), 0600))
	firstEnv, err := os.ReadFile(filepath.Join(g.Directory(), ".env"))
	require.NoError(t, err)
	require.NoError(t, g.Ensure(context.Background(), io.Discard))
	secondCompose, err := os.ReadFile(composePath)
	require.NoError(t, err)
	require.Equal(t, firstCompose, secondCompose, "startup must restore the bundled Compose file")
	config, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Equal(t, "custom config", string(config), "startup must preserve existing configuration")
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

func TestStopBeforeFirstStartDoesNotCreateGatewayFiles(t *testing.T) {
	t.Parallel()
	g := gateway.New(t.TempDir())
	require.NoError(t, g.Stop(context.Background(), io.Discard))
	require.NoDirExists(t, g.Directory())
}

//nolint:forbidigo // Serial PATH override tests device-login invocation without contacting any provider.
func TestAuthenticateStartsGatewayAndRunsSelectedProviderLogin(t *testing.T) {
	bin := t.TempDir()
	log := filepath.Join(bin, "calls")
	require.NoError(t, os.WriteFile(filepath.Join(bin, "docker"), []byte(`#!/bin/sh
echo "$*" >> "$CALL_LOG"
case "$*" in *" exec "*) echo 'Complete device login in your browser';; esac
`), 0700))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CALL_LOG", log)
	g := gateway.New(t.TempDir())
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

//nolint:forbidigo // Serial test owns PATH; concurrent callers still exercise cross-process-compatible file locking.
func TestConcurrentLaunchersSerializeGatewayReconciliation(t *testing.T) {
	bin := t.TempDir()
	log := filepath.Join(bin, "calls")
	require.NoError(t, os.WriteFile(filepath.Join(bin, "docker"), []byte(`#!/bin/sh
case "$*" in
  *" ps "*) if test -f ready; then printf 'litellm healthy\ndb healthy\n'; fi ;;
  *" up "*)
    mkdir reconciling || exit 1
    echo up >> "$CALL_LOG"
    sleep 0.1
    touch ready
    rmdir reconciling ;;
esac
`), 0700))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CALL_LOG", log)
	dataDir := t.TempDir()
	var workers sync.WaitGroup
	errors := make(chan error, 4)
	for range 4 {
		workers.Go(func() { errors <- gateway.New(dataDir).Ensure(context.Background(), io.Discard) })
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

//nolint:forbidigo // Serial PATH override isolates Compose failure from the developer's Docker installation.
func TestGatewayStartupFailureIsReturned(t *testing.T) {
	bin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\ncase \"$*\" in *\" up \"*) exit 17;; esac\n"), 0700))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	err := gateway.New(t.TempDir()).Ensure(context.Background(), io.Discard)
	require.ErrorContains(t, err, "start LLM gateway")
	require.ErrorContains(t, err, "exit status 17")
}
