package sandbox_test

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// runInTerminal reexecutes the test with a controlling terminal, as required by
// the SDK's interactive attachment. The child returns false to run the test body.
func runInTerminal(t *testing.T) bool {
	t.Helper()
	if os.Getenv("SANDBOX_TERMINAL_TEST_CHILD") == "1" {
		return false
	}
	binary, err := os.Executable()
	require.NoError(t, err)
	args := []string{"-test.run=^" + t.Name() + "$", "-test.v", "-test.timeout=2m"}
	var scriptArgs []string
	switch runtime.GOOS {
	case "linux":
		quotedBinary := "'" + strings.ReplaceAll(binary, "'", "'\"'\"'") + "'"
		scriptArgs = []string{"-q", "-e", "-c", quotedBinary + " " + strings.Join(args, " "), "/dev/null"}
	case "darwin":
		scriptArgs = append([]string{"-q", "/dev/null", binary}, args...)
	default:
		t.Skip("interactive terminal test requires Linux or macOS")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "script", scriptArgs...)
	command.Env = append(os.Environ(), "SANDBOX_TERMINAL_TEST_CHILD=1")
	output, err := command.CombinedOutput()
	require.NoError(t, err, "terminal test: %s", output)
	return true
}
