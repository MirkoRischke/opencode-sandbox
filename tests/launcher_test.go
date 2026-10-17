package sandbox_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLauncherRejectsDirectoriesOutsideGit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "opencode-sandbox")
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./cmd/opencode-sandbox")
	build.Dir = ".."
	output, err := build.CombinedOutput()
	require.NoError(t, err, "build launcher: %s", output)
	command := exec.CommandContext(ctx, binary)
	command.Dir = t.TempDir()

	output, err = command.CombinedOutput()

	require.Error(t, err)
	require.Contains(t, string(output), "discover worktree")
}
