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
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("HERDR_ENV", "1")
	writeSettings(t, "invalid TOML [")
	trustDir := filepath.Join(os.Getenv("XDG_STATE_HOME"), "opencode-sandbox")
	require.NoError(t, os.Mkdir(trustDir, 0700))
	trustPath := filepath.Join(trustDir, "trust.json")
	require.NoError(t, os.WriteFile(trustPath, []byte("invalid JSON"), 0600))
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, buildLauncher(t), "config", "default")
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

func TestLauncherRejectsUntrustedRepoConfigBeforeLoadingSecrets(t *testing.T) {
	root, _ := repoSettingsFixture(t, "[secrets.github]\nallowed_hosts = ['other.example.com']\n")
	writeSettings(t, "[secrets.github]\nfile = 'missing-token'\nenv = 'GH_TOKEN'\nallowed_hosts = ['github.com']\nheaders = ['authorization']\n")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, buildLauncher(t))
	command.Dir = root
	command.Stdin = strings.NewReader("yes\n")
	output, err := command.CombinedOutput()
	require.Error(t, err)
	require.Contains(t, string(output), "untrusted repo config")
	require.NotContains(t, string(output), "missing-token")
	_, err = os.Stat(filepath.Join(os.Getenv("XDG_STATE_HOME"), "opencode-sandbox", "trust.json"))
	require.True(t, os.IsNotExist(err))
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
