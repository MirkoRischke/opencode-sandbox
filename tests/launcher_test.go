package sandbox_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLauncherRejectsDirectoriesOutsideGit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	binary := buildLauncher(t)
	command := exec.CommandContext(ctx, binary)
	command.Dir = t.TempDir()

	output, err := command.CombinedOutput()

	require.Error(t, err)
	require.Contains(t, string(output), "discover worktree")
}

func TestLauncherAcceptsCustomConfigFlag(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	binary := buildLauncher(t)
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{}`), 0600))
	command := exec.CommandContext(ctx, binary, "--config", path)
	command.Dir = t.TempDir()

	output, err := command.CombinedOutput()

	require.Error(t, err)
	require.Contains(t, string(output), "discover worktree", "config must be accepted before worktree discovery")
}

func TestLauncherRejectsMissingCustomConfigBeforeStartingVM(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	binary := buildLauncher(t)
	command := exec.CommandContext(ctx, binary, "--config", filepath.Join(t.TempDir(), "missing.json"))

	output, err := command.CombinedOutput()

	require.Error(t, err)
	require.Contains(t, string(output), "read sandbox config")
}
