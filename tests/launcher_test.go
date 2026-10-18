package sandbox_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	launcherconfig "opencode-sandbox/internal/config"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/require"
)

func TestLauncherPrintsDefaultConfigWithoutLoadingHostState(t *testing.T) {
	t.Parallel()
	configDir, stateDir := t.TempDir(), t.TempDir()
	writeSettingsAt(t, configDir, "invalid TOML [")
	trustDir := filepath.Join(stateDir, "opencode-sandbox")
	require.NoError(t, os.Mkdir(trustDir, 0700))
	trustPath := filepath.Join(trustDir, "trust.json")
	require.NoError(t, os.WriteFile(trustPath, []byte("invalid JSON"), 0600))
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, buildLauncher(t), "config", "default")
	command.Env = append(os.Environ(), "XDG_CONFIG_HOME="+configDir, "XDG_STATE_HOME="+stateDir, "HERDR_ENV=1")
	command.Dir = t.TempDir() // Printing defaults must work outside Git.
	output, err := command.CombinedOutput()
	require.NoError(t, err, "%s", output)
	var settings launcherconfig.Settings
	metadata, err := toml.Decode(string(output), &settings)
	require.NoError(t, err)
	require.Empty(t, metadata.Undecoded())
	require.Equal(t, launcherconfig.DefaultSettings(), settings)
	require.Contains(t, string(output), "# Number of virtual CPUs")
	require.Contains(t, string(output), "# [secrets.example]")
	stored, err := os.ReadFile(trustPath)
	require.NoError(t, err)
	require.Equal(t, "invalid JSON", string(stored))
}

func TestLauncherRejectsUnsafeWorkspaceBeforeLoadingSettings(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	dir := t.TempDir()
	writeSettingsAt(t, dir, "invalid TOML [")
	binary := buildLauncher(t)
	command := exec.CommandContext(ctx, binary)
	command.Env = append(os.Environ(), "XDG_CONFIG_HOME="+dir)
	command.Dir = "/"

	output, err := command.CombinedOutput()

	require.Error(t, err)
	require.Contains(t, string(output), "unsafe workspace directory")
}

func TestLauncherListsVMsOutsideGitWithoutLoadingSettings(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	dir := t.TempDir()
	writeSettingsAt(t, dir, "invalid TOML [")
	binary := buildLauncher(t)
	for _, name := range []string{"list", "ls"} {
		t.Run(name, func(t *testing.T) {
			command := exec.CommandContext(ctx, binary, name)
			command.Env = append(os.Environ(), "XDG_CONFIG_HOME="+dir)
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

func TestLauncherRejectsMissingMountBeforeStartingVM(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeSettingsAt(t, dir, "[[mounts]]\nsource = 'missing.json'\ntarget = '/etc/opencode/opencode.json'\n")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, buildLauncher(t))
	command.Dir = newWorktree(t)
	command.Env = append(os.Environ(), "XDG_CONFIG_HOME="+dir)

	output, err := command.CombinedOutput()

	require.Error(t, err)
	require.Contains(t, string(output), "mounts[0].source")
	require.Contains(t, string(output), filepath.Join(dir, "opencode-sandbox", "missing.json"))
}

func TestLauncherRejectsInvalidSettingsBeforeStartingVM(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeSettingsAt(t, dir, "[network]\nallowed_hosts = ['*']\n")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, buildLauncher(t))
	command.Dir = newWorktree(t)
	command.Env = append(os.Environ(), "XDG_CONFIG_HOME="+dir)

	output, err := command.CombinedOutput()

	require.Error(t, err)
	require.Contains(t, string(output), "network.allowed_hosts")
	require.Contains(t, string(output), "config.toml")
}

func TestLauncherRejectsUntrustedRepoConfigBeforeLoadingSecrets(t *testing.T) {
	t.Parallel()
	root, _, paths := repoSettingsFixture(t, "[secrets.github]\nallowed_hosts = ['other.example.com']\n")
	writeSettingsAt(t, paths.ConfigDir, "[secrets.github]\nfile = 'missing-token'\nenv = 'GH_TOKEN'\nallowed_hosts = ['github.com']\nheaders = ['authorization']\n")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, buildLauncher(t))
	command.Dir = root
	command.Env = append(os.Environ(), "XDG_CONFIG_HOME="+paths.ConfigDir, "XDG_STATE_HOME="+paths.StateDir)
	command.Stdin = strings.NewReader("yes\n")
	output, err := command.CombinedOutput()
	require.Error(t, err)
	require.Contains(t, string(output), "untrusted workspace config")
	require.NotContains(t, string(output), "missing-token")
	_, err = os.Stat(filepath.Join(paths.StateDir, "opencode-sandbox", "trust.json"))
	require.True(t, os.IsNotExist(err))
}

func TestLauncherValidatesAWSFlagsBeforeStartingVM(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, buildLauncher(t), "--aws-role", "arn:aws:iam::123456789012:role/SandboxReadOnly")
	dir := t.TempDir()
	writeSettingsAt(t, dir, "invalid TOML [")
	command.Env = append(os.Environ(), "XDG_CONFIG_HOME="+dir)
	command.Dir = t.TempDir()
	output, err := command.CombinedOutput()
	require.Error(t, err)
	require.Contains(t, string(output), "config.toml")
	require.NotContains(t, string(output), "--aws-role")
}
