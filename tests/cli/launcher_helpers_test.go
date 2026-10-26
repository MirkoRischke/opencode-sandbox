package sandbox_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Tests that isolate command behaviour from logging configuration pass an
// explicit --console-log-level override to bypass startup logging policy reads.
var launcherBinary string

var launcherBuild = sync.OnceValue(func() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-o", launcherBinary, "./cmd/opencode-sandbox")
	build.Dir = "../.."
	output, err := build.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build launcher: %w: %s", err, output)
	}
	return nil
})

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("/tmp/opencode", "launcher-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	launcherBinary = filepath.Join(dir, "opencode-sandbox")
	code := m.Run()
	if err := os.RemoveAll(dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		code = 1
	}
	os.Exit(code)
}

func buildLauncher(t *testing.T) string {
	t.Helper()
	require.NoError(t, launcherBuild())
	return launcherBinary
}
