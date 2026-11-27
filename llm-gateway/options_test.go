package gateway_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	gateway "opencode-sandbox/llm-gateway"

	"github.com/stretchr/testify/require"
)

func TestEnsureUsesCustomComposeOptions(t *testing.T) {
	t.Parallel()
	docker := fakeDocker(t, `printf '%s\n%s\n' "$*" "$OPENCODE_SANDBOX_GATEWAY_PORT" > calls
`)
	for _, tc := range []struct {
		name    string
		options gateway.Options
		want    string
	}{
		{"default project name", gateway.Options{}, "--project-name opencode-sandbox-gateway"},
		{"default host port", gateway.Options{}, "\n4000\n"},
		{"project name", gateway.Options{ProjectName: "isolated-gateway"}, "--project-name isolated-gateway"},
		{"host port", gateway.Options{Port: 14000}, "\n14000\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.options.DockerPath = docker
			g := gateway.NewWithOptions(t.TempDir(), tc.options)

			err := g.Ensure(context.Background(), io.Discard)

			require.NoError(t, err)
			calls, err := os.ReadFile(filepath.Join(g.Directory(), "calls"))
			require.NoError(t, err)
			require.Contains(t, string(calls), tc.want)
		})
	}
}

func TestDirectoryUsesLLMGatewayDataDirectory(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()
	require.Equal(t, filepath.Join(dataDir, "opencode-sandbox", "llm-gateway"), gateway.New(dataDir).Directory())
}
