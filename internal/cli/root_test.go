package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestManagementCommandsRejectExtraArguments(t *testing.T) {
	t.Parallel()
	for _, command := range []string{"list", "ls", "remove", "rm"} {
		extras := []string{"--yes", "vm-name"}
		if command == "list" || command == "ls" {
			extras = append(extras, "--all")
		}
		for _, extra := range extras {
			t.Run(command+"/"+extra, func(t *testing.T) {
				_, err := executeCommand(command, extra)
				require.Error(t, err)
			})
		}
	}
	for _, command := range []string{"remove", "rm"} {
		_, err := executeCommand(command, "--all", "extra")
		require.Error(t, err)
	}
}

func TestRemoveAllFlag(t *testing.T) {
	t.Parallel()
	for _, all := range []bool{false, true} {
		cmd := newRemoveCommand()
		args := []string{}
		if all {
			args = append(args, "--all")
		}
		require.NoError(t, cmd.ParseFlags(args))
		got, err := cmd.Flags().GetBool("all")
		require.NoError(t, err)
		require.Equal(t, all, got)
	}
	for _, command := range []string{"remove", "rm"} {
		output, err := executeCommand(command, "--help")
		require.NoError(t, err)
		require.Contains(t, output, "--all")
		require.Contains(t, output, "this directory's VM")
	}
}

func TestConfigCommandRejectsInvalidArguments(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"config"}, {"config", "show"}, {"config", "default", "extra"}} {
		t.Run(strings.Join(args, "/"), func(t *testing.T) {
			_, err := executeCommand(args...)
			require.Error(t, err)
		})
	}
}

func TestSnapshotCommandRejectsInvalidArguments(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"snapshot"}, {"snapshot", "unknown"}, {"snapshot", "create", "extra"}, {"snapshot", "remove", "extra"}, {"snapshot", "remove", "--all", "extra"}, {"snapshot", "list", "extra"}} {
		t.Run(strings.Join(args, "/"), func(t *testing.T) {
			_, err := executeCommand(args...)
			require.Error(t, err)
		})
	}
}

func TestSnapshotRemoveAllFlag(t *testing.T) {
	t.Parallel()
	for _, all := range []bool{false, true} {
		cmd := newSnapshotCommand()
		remove, _, err := cmd.Find([]string{"remove"})
		require.NoError(t, err)
		args := []string{}
		if all {
			args = append(args, "--all")
		}
		require.NoError(t, remove.ParseFlags(args))
		got, err := remove.Flags().GetBool("all")
		require.NoError(t, err)
		require.Equal(t, all, got)
	}
	output, err := executeCommand("snapshot", "remove", "--help")
	require.NoError(t, err)
	require.Contains(t, output, "--all")
	require.Contains(t, output, "without confirmation")
}

func TestDiagnosticsCommandRejectsInvalidArguments(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"diagnostics"}, {"diagnostics", "unknown"}, {"diagnostics", "network", "extra"}, {"diagnostics", "logs", "extra"}} {
		t.Run(strings.Join(args, "/"), func(t *testing.T) {
			_, err := executeCommand(args...)
			require.Error(t, err)
		})
	}
}

func executeCommand(args ...string) (string, error) {
	var output bytes.Buffer
	cmd := NewCommand()
	cmd.SetArgs(args)
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	err := cmd.ExecuteContext(context.Background())
	return output.String(), err
}

func TestLaunchHelpDocumentsAWSFlags(t *testing.T) {
	t.Parallel()
	output, err := executeCommand("--help")
	require.NoError(t, err)
	require.Contains(t, output, "Usage:")
	require.Contains(t, output, "--aws-role")
	require.Contains(t, output, "--aws-profile")
}

func TestSubcommandHelpDoesNotInheritLaunchFlags(t *testing.T) {
	t.Parallel()
	for _, command := range []string{
		"list", "remove", "config", "config default", "diagnostics", "diagnostics network", "diagnostics logs",
		"snapshot", "snapshot create", "snapshot remove", "snapshot list", "completion",
	} {
		t.Run(command, func(t *testing.T) {
			t.Parallel()
			output, err := executeCommand(append(strings.Fields(command), "--help")...)
			require.NoError(t, err)
			require.Contains(t, output, "Usage:")
			require.NotContains(t, output, "--aws-role")
			require.NotContains(t, output, "--aws-profile")
		})
	}
}

func TestManagementAliases(t *testing.T) {
	t.Parallel()
	for alias, name := range map[string]string{
		"ls": "list", "rm": "remove",
		"snapshot ls": "snapshot list", "snapshot rm": "snapshot remove",
	} {
		t.Run(alias, func(t *testing.T) {
			output, err := executeCommand(append(strings.Fields(alias), "--help")...)
			require.NoError(t, err)
			require.Contains(t, output, "opencode-sandbox "+name+" [flags]")
		})
	}
}

func TestLaunchRejectsInvalidArguments(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"unknown"}, {"--unknown"}, {"--config"}} {
		t.Run(strings.Join(args, "/"), func(t *testing.T) {
			output, err := executeCommand(args...)
			require.Error(t, err)
			require.Empty(t, output, "errors are returned for main to print once")
		})
	}
}

func TestCompletionScriptsWithoutLoadingHostState(t *testing.T) {
	t.Parallel()
	if os.Getenv("OPENCODE_SANDBOX_COMPLETION_TEST") != "1" {
		dir := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(dir, "opencode-sandbox"), 0700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "opencode-sandbox", "config.toml"), []byte("invalid TOML ["), 0600))
		runIsolatedCommandTest(t, t.Name(), "XDG_CONFIG_HOME="+dir, "HERDR_ENV=1", "HERDR_AGENT=", "OPENCODE_SANDBOX_COMPLETION_TEST=1")
		return
	}
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			output, err := executeCommand("completion", shell)
			require.NoError(t, err)
			require.Contains(t, output, "opencode-sandbox")
		})
	}
}

func TestCompletionSuggestions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		prefix string
		want   []string
	}{
		{prefix: "", want: []string{"list", "remove", "config", "diagnostics", "snapshot", "completion"}},
		{prefix: "--aws-", want: []string{"--aws-role", "--aws-profile"}},
	} {
		t.Run(tc.prefix, func(t *testing.T) {
			output, err := executeCommand("__complete", tc.prefix)
			require.NoError(t, err)
			for _, want := range tc.want {
				require.Contains(t, output, want+"	")
			}
		})
	}
}

func TestIdentifyHerdrAgent(t *testing.T) {
	t.Parallel()
	if os.Getenv("OPENCODE_SANDBOX_HERDR_TEST") == "1" {
		require.NoError(t, identifyHerdrAgent())
		// Herdr reads the initial process environment, including after re-exec.
		environ, err := os.ReadFile("/proc/self/environ")
		require.NoError(t, err)
		want := []byte("HERDR_AGENT=" + os.Getenv("OPENCODE_SANDBOX_EXPECT_AGENT"))
		require.Contains(t, bytes.Split(environ, []byte{0}), want)
		return
	}
	for _, tc := range []struct{ name, herdr, agent, want string }{
		{name: "inside Herdr", herdr: "1", want: "opencode"},
		{name: "outside Herdr"},
		{name: "explicit hint", herdr: "1", agent: "custom", want: "custom"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runIsolatedCommandTest(t, "TestIdentifyHerdrAgent", "HERDR_ENV="+tc.herdr, "HERDR_AGENT="+tc.agent,
				"OPENCODE_SANDBOX_HERDR_TEST=1", "OPENCODE_SANDBOX_EXPECT_AGENT="+tc.want)
		})
	}
}

func runIsolatedCommandTest(t *testing.T, name string, env ...string) {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "-test.run=^"+name+"$")
	command.Env = append(os.Environ(), env...)
	output, err := command.CombinedOutput()
	require.NoError(t, err, "isolated test %s: %s", name, output)
}
