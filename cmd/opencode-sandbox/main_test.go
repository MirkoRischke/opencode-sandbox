package main

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
	for _, command := range []string{"list", "ls", "remove", "rm"} {
		for _, extra := range []string{"--all", "--yes", "vm-name"} {
			t.Run(command+"/"+extra, func(t *testing.T) {
				_, err := executeCommand(command, extra)
				if err == nil {
					t.Fatal("expected management argument error")
				}
			})
		}
	}
}

func TestConfigCommandRejectsInvalidArguments(t *testing.T) {
	for _, args := range [][]string{{"config"}, {"config", "show"}, {"config", "default", "extra"}} {
		t.Run(strings.Join(args, "/"), func(t *testing.T) {
			if _, err := executeCommand(args...); err == nil {
				t.Fatal("expected config argument error")
			}
		})
	}
}

func TestDiagnosticsCommandRejectsInvalidArguments(t *testing.T) {
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
	cmd := newCommand()
	cmd.SetArgs(args)
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	err := cmd.ExecuteContext(context.Background())
	return output.String(), err
}

func TestCommandHelp(t *testing.T) {
	for _, args := range [][]string{
		{"--help"}, {"list", "--help"}, {"remove", "--help"},
		{"config", "--help"}, {"config", "default", "--help"},
		{"diagnostics", "--help"}, {"diagnostics", "network", "--help"},
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
	for alias, name := range map[string]string{"ls": "list", "rm": "remove"} {
		t.Run(alias, func(t *testing.T) {
			output, err := executeCommand(alias, "--help")
			if err != nil || !strings.Contains(output, "opencode-sandbox "+name+" [flags]") {
				t.Fatalf("expected help for %s, got %v\n%s", name, err, output)
			}
		})
	}
}

func TestLaunchRejectsInvalidArguments(t *testing.T) {
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
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HERDR_ENV", "1")
	t.Setenv("HERDR_AGENT", "")
	if err := os.Mkdir(filepath.Join(dir, "opencode-sandbox"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "opencode-sandbox", "config.toml"), []byte("invalid TOML ["), 0600); err != nil {
		t.Fatal(err)
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
	for _, tc := range []struct {
		prefix string
		want   []string
	}{
		{prefix: "", want: []string{"list", "remove", "config", "diagnostics", "completion"}},
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
			t.Setenv("HERDR_ENV", tc.herdr)
			t.Setenv("HERDR_AGENT", tc.agent)
			t.Setenv("OPENCODE_SANDBOX_HERDR_TEST", "1")
			t.Setenv("OPENCODE_SANDBOX_EXPECT_AGENT", tc.want)
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			output, err := exec.CommandContext(ctx, executable, "-test.run=^TestIdentifyHerdrAgent$").CombinedOutput()
			if err != nil {
				t.Fatalf("agent identification failed: %v\n%s", err, output)
			}
		})
	}
}
