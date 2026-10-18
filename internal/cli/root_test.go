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
				if err == nil {
					t.Fatal("expected management argument error")
				}
			})
		}
	}
	for _, command := range []string{"remove", "rm"} {
		if _, err := executeCommand(command, "--all", "extra"); err == nil {
			t.Fatal("expected argument error with --all")
		}
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
		if err := cmd.ParseFlags(args); err != nil {
			t.Fatal(err)
		}
		got, err := cmd.Flags().GetBool("all")
		if err != nil || got != all {
			t.Fatalf("expected all=%t, got %t: %v", all, got, err)
		}
	}
	for _, command := range []string{"remove", "rm"} {
		output, err := executeCommand(command, "--help")
		if err != nil || !strings.Contains(output, "--all") || !strings.Contains(output, "this worktree's VM") {
			t.Fatalf("expected current-worktree help and --all flag, got %v\n%s", err, output)
		}
	}
}

func TestConfigCommandRejectsInvalidArguments(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"config"}, {"config", "show"}, {"config", "default", "extra"}} {
		t.Run(strings.Join(args, "/"), func(t *testing.T) {
			if _, err := executeCommand(args...); err == nil {
				t.Fatal("expected config argument error")
			}
		})
	}
}

func TestSnapshotCommandRejectsInvalidArguments(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"snapshot"}, {"snapshot", "unknown"}, {"snapshot", "create", "extra"}, {"snapshot", "remove", "extra"}, {"snapshot", "remove", "--all", "extra"}, {"snapshot", "list", "extra"}} {
		t.Run(strings.Join(args, "/"), func(t *testing.T) {
			if _, err := executeCommand(args...); err == nil {
				t.Fatal("expected snapshot argument error")
			}
		})
	}
}

func TestSnapshotRemoveAllFlag(t *testing.T) {
	t.Parallel()
	for _, all := range []bool{false, true} {
		cmd := newSnapshotCommand()
		remove, _, err := cmd.Find([]string{"remove"})
		if err != nil {
			t.Fatal(err)
		}
		args := []string{}
		if all {
			args = append(args, "--all")
		}
		if err := remove.ParseFlags(args); err != nil {
			t.Fatal(err)
		}
		got, err := remove.Flags().GetBool("all")
		if err != nil || got != all {
			t.Fatalf("expected all=%t, got %t: %v", all, got, err)
		}
	}
	output, err := executeCommand("snapshot", "remove", "--help")
	if err != nil || !strings.Contains(output, "--all") || !strings.Contains(output, "without confirmation") {
		t.Fatalf("expected documented --all flag, got %v\n%s", err, output)
	}
}

func TestDiagnosticsCommandRejectsInvalidArguments(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"diagnostics"}, {"diagnostics", "unknown"}, {"diagnostics", "network", "extra"}} {
		t.Run(strings.Join(args, "/"), func(t *testing.T) {
			if _, err := executeCommand(args...); err == nil {
				t.Fatal("expected diagnostics argument error")
			}
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

func TestCommandHelp(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"--help"}, {"list", "--help"}, {"remove", "--help"},
		{"config", "--help"}, {"config", "default", "--help"},
		{"diagnostics", "--help"}, {"diagnostics", "network", "--help"},
		{"snapshot", "--help"}, {"snapshot", "create", "--help"}, {"snapshot", "remove", "--help"},
		{"snapshot", "list", "--help"},
		{"completion", "--help"},
	} {
		t.Run(strings.Join(args, "/"), func(t *testing.T) {
			output, err := executeCommand(args...)
			if err != nil || !strings.Contains(output, "Usage:") {
				t.Fatalf("expected command help, got %v\n%s", err, output)
			}
			if len(args) == 1 {
				for _, flag := range []string{"--config", "--cli-config", "--aws-role", "--aws-profile"} {
					if !strings.Contains(output, flag) {
						t.Errorf("root help missing %s", flag)
					}
				}
			} else if strings.Contains(output, "--aws-role") {
				t.Error("launch flags must not be inherited by subcommands")
			}
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
			if err != nil || !strings.Contains(output, "opencode-sandbox "+name+" [flags]") {
				t.Fatalf("expected help for %s, got %v\n%s", name, err, output)
			}
		})
	}
}

func TestLaunchRejectsInvalidArguments(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"unknown"}, {"--unknown"}, {"--config"}} {
		t.Run(strings.Join(args, "/"), func(t *testing.T) {
			output, err := executeCommand(args...)
			if err == nil {
				t.Fatal("expected launch argument error")
			}
			if output != "" {
				t.Fatalf("errors should be returned for main to print once, got %q", output)
			}
		})
	}
}

func TestCompletionScriptsWithoutLoadingHostState(t *testing.T) {
	t.Parallel()
	if os.Getenv("OPENCODE_SANDBOX_COMPLETION_TEST") != "1" {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, "opencode-sandbox"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "opencode-sandbox", "config.toml"), []byte("invalid TOML ["), 0600); err != nil {
			t.Fatal(err)
		}
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, executable, "-test.run=^TestCompletionScriptsWithoutLoadingHostState$")
		command.Env = append(os.Environ(), "XDG_CONFIG_HOME="+dir, "HERDR_ENV=1", "HERDR_AGENT=", "OPENCODE_SANDBOX_COMPLETION_TEST=1")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("completion failed: %v\n%s", err, output)
		}
		return
	}
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			output, err := executeCommand("completion", shell)
			if err != nil || !strings.Contains(output, "opencode-sandbox") {
				t.Fatalf("expected %s completion script, got %v\n%s", shell, err, output)
			}
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
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tc.want {
				if !strings.Contains(output, want+"\t") {
					t.Errorf("completion missing %s:\n%s", want, output)
				}
			}
		})
	}
}

func TestIdentifyHerdrAgent(t *testing.T) {
	t.Parallel()
	if os.Getenv("OPENCODE_SANDBOX_HERDR_TEST") == "1" {
		if err := identifyHerdrAgent(); err != nil {
			t.Fatal(err)
		}
		// Exercise the same interface Herdr uses, not just os.Getenv.
		environ, err := os.ReadFile("/proc/self/environ")
		if err != nil {
			t.Fatal(err)
		}
		want := []byte("HERDR_AGENT=" + os.Getenv("OPENCODE_SANDBOX_EXPECT_AGENT"))
		for _, entry := range bytes.Split(environ, []byte{0}) {
			if bytes.Equal(entry, want) {
				return
			}
		}
		t.Fatalf("initial process environment missing %q", want)
		return
	}

	for _, tc := range []struct {
		name, herdr, agent, want string
	}{
		{name: "inside Herdr", herdr: "1", want: "opencode"},
		{name: "outside Herdr"},
		{name: "explicit hint", herdr: "1", agent: "custom", want: "custom"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, executable, "-test.run=^TestIdentifyHerdrAgent$")
			command.Env = append(os.Environ(),
				"HERDR_ENV="+tc.herdr, "HERDR_AGENT="+tc.agent,
				"OPENCODE_SANDBOX_HERDR_TEST=1", "OPENCODE_SANDBOX_EXPECT_AGENT="+tc.want)
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("agent identification failed: %v\n%s", err, output)
			}
		})
	}
}
