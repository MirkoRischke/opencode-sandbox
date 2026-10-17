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
	writeSettings(t, "[network]\nallowed_hosts = []\n")
	binary := buildLauncher(t)
	command := exec.CommandContext(ctx, binary)
	command.Dir = t.TempDir()

	output, err := command.CombinedOutput()

	require.Error(t, err)
	require.Contains(t, string(output), "discover worktree")
}

func TestLauncherListsVMsOutsideGitWithoutLoadingSettings(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeSettings(t, "invalid TOML [")
	binary := buildLauncher(t)
	for _, name := range []string{"list", "ls"} {
		t.Run(name, func(t *testing.T) {
			command := exec.CommandContext(ctx, binary, name)
			command.Dir = t.TempDir()

			output, err := command.CombinedOutput()

			require.NoError(t, err, "%s", output)
			if string(output) != "No managed VMs found.\n" {
				require.Contains(t, string(output), "NAME")
				require.Contains(t, string(output), "STATE")
			}
		})
	}
}

func TestLauncherAcceptsCustomConfigFlag(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeSettings(t, "[network]\nallowed_hosts = []\n")
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
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	binary := buildLauncher(t)
	command := exec.CommandContext(ctx, binary, "--config", filepath.Join(t.TempDir(), "missing.json"))

	output, err := command.CombinedOutput()

	require.Error(t, err)
	require.Contains(t, string(output), "read sandbox config")
}

func TestLauncherAcceptsCustomCLIConfigFlag(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeSettings(t, "[network]\nallowed_hosts = []\n")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	binary := buildLauncher(t)
	path := filepath.Join(t.TempDir(), "cli.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"theme":{"name":"tokyonight"}}`), 0600))
	command := exec.CommandContext(ctx, binary, "--cli-config", path)
	command.Dir = t.TempDir()

	output, err := command.CombinedOutput()

	require.Error(t, err)
	require.Contains(t, string(output), "discover worktree", "CLI config must be accepted before worktree discovery")
}

func TestLauncherRejectsMissingCLIConfigBeforeStartingVM(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	binary := buildLauncher(t)
	command := exec.CommandContext(ctx, binary, "--cli-config", filepath.Join(t.TempDir(), "missing.json"))

	output, err := command.CombinedOutput()

	require.Error(t, err)
	require.Contains(t, string(output), "read CLI config")
}

func TestLauncherRejectsInvalidSettingsBeforeStartingVM(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeSettings(t, "[network]\nallowed_hosts = ['*']\n")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, buildLauncher(t))
	command.Dir = newWorktree(t)

	output, err := command.CombinedOutput()

	require.Error(t, err)
	require.Contains(t, string(output), "network.allowed_hosts")
	require.Contains(t, string(output), "config.toml")
}

func TestLauncherValidatesAWSFlagsBeforeStartingVM(t *testing.T) {
	binary := buildLauncher(t)
	for _, args := range [][]string{
		{"--aws-role", "not-a-role"},
		{"--aws-profile", "sandbox-test"},
		{"--aws-role", "arn:aws:iam::123456789012:user/NotARole"},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		command := exec.CommandContext(ctx, binary, args...)
		command.Dir = t.TempDir()
		output, err := command.CombinedOutput()
		cancel()
		require.Error(t, err)
		require.Contains(t, string(output), "--aws-role")
		require.NotContains(t, string(output), "discover worktree")
	}
}

func TestLauncherAcceptsAWSRoleWithoutProfile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, buildLauncher(t), "--aws-role", "arn:aws:iam::123456789012:role/SandboxReadOnly")
	command.Dir = t.TempDir()
	output, err := command.CombinedOutput()
	require.Error(t, err)
	require.Contains(t, string(output), "discover worktree")
}
