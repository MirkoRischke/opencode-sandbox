package sandbox_test

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGatewayRejectsInvalidPortBeforeStartingDocker(t *testing.T) {
	t.Parallel()
	for _, port := range []string{"0", "65536", "not-a-port"} {
		t.Run(port, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, buildLauncher(t), "--console-log-level", "warn", "gateway", "start")
			command.Env = append(os.Environ(), "XDG_DATA_HOME="+dir,
				"OPENCODE_SANDBOX_GATEWAY_PORT="+port, "PATH="+t.TempDir())

			output, err := command.CombinedOutput()

			require.Error(t, err)
			require.Contains(t, string(output), "OPENCODE_SANDBOX_GATEWAY_PORT")
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			require.Empty(t, entries)
		})
	}
}
