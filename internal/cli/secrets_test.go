package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

func executeSecret(t *testing.T, deps secretDependencies, input string, args ...string) (string, error) {
	t.Helper()
	var output bytes.Buffer
	cmd := secretCommand(deps)
	cmd.SetArgs(args)
	cmd.SetIn(strings.NewReader(input))
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	err := cmd.ExecuteContext(context.Background())
	return output.String(), err
}

func TestSecretSetStoresCredentialWithoutPrintingValue(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name             string
		flags            []string
		service, account string
	}{
		{name: "defaults", service: "opencode-sandbox", account: "github"},
		{name: "custom entry", flags: []string{"--service", "company", "--account", "work"}, service: "company", account: "work"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			stored := ""
			deps := secretDependencies{
				read: func(*cobra.Command) (string, error) { return "SENSITIVE_VALUE", nil },
				set: func(service, account, value string) error {
					require.Equal(t, tc.service, service)
					require.Equal(t, tc.account, account)
					stored = value
					return nil
				},
			}

			output, err := executeSecret(t, deps, "", append([]string{"set", "github"}, tc.flags...)...)

			require.NoError(t, err)
			require.Equal(t, "SENSITIVE_VALUE", stored)
			require.NotContains(t, output, stored)
			require.Contains(t, output, "Recreate existing VMs")
		})
	}
}

func TestSecretRemoveRequiresExplicitConfirmation(t *testing.T) {
	t.Parallel()
	for _, answer := range []string{"no\n", "\n", "", "yes"} {
		t.Run(answer, func(t *testing.T) {
			t.Parallel()
			deps := secretDependencies{delete: func(string, string) error { t.Fatal("must not delete without confirmation"); return nil }}

			output, err := executeSecret(t, deps, answer, "remove", "github")

			require.ErrorContains(t, err, "not confirmed")
			require.Contains(t, output, "shared across projects")
		})
	}
}

func TestSecretRemoveDeletesOnlyTheConfirmedEntry(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name             string
		flags            []string
		service, account string
	}{
		{name: "defaults", service: "opencode-sandbox", account: "github"},
		{name: "custom entry", flags: []string{"--service", "company", "--account", "work"}, service: "company", account: "work"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			deleted := false
			deps := secretDependencies{delete: func(service, account string) error {
				require.Equal(t, tc.service, service)
				require.Equal(t, tc.account, account)
				deleted = true
				return nil
			}}

			output, err := executeSecret(t, deps, "yes\n", append([]string{"rm", "github"}, tc.flags...)...)

			require.NoError(t, err)
			require.True(t, deleted)
			require.Contains(t, output, "Existing VMs retain")
		})
	}
}

func TestSecretCommandsRejectInvalidIdentifiersBeforeAccess(t *testing.T) {
	t.Parallel()
	deps := secretDependencies{
		read:   func(*cobra.Command) (string, error) { t.Fatal("must not read input"); return "", nil },
		set:    func(string, string, string) error { t.Fatal("must not store"); return nil },
		delete: func(string, string) error { t.Fatal("must not delete"); return nil },
		list:   func(string) ([]string, error) { t.Fatal("must not list"); return nil, nil },
	}
	for _, operation := range []string{"set", "remove"} {
		for _, name := range []string{"", "   ", "bad\nname", "bad\x00name"} {
			_, err := executeSecret(t, deps, "yes\n", operation, name)
			require.Error(t, err)
		}
	}
	for _, service := range []string{"", "   ", "bad\nservice", "bad\x00service"} {
		_, err := executeSecret(t, deps, "", "list", "--service", service)
		require.Error(t, err)
	}
}

func TestSecretCommandsSuppressProviderErrors(t *testing.T) {
	t.Parallel()
	deps := secretDependencies{
		read:   func(*cobra.Command) (string, error) { return "SENSITIVE_VALUE", nil },
		set:    func(string, string, string) error { return errors.New("SENSITIVE_VALUE") },
		delete: func(string, string) error { return errors.New("SENSITIVE_VALUE") },
		list:   func(string) ([]string, error) { return nil, errors.New("SENSITIVE_VALUE") },
	}
	for _, operation := range []string{"set", "remove"} {
		output, err := executeSecret(t, deps, "yes\n", operation, "github")
		require.ErrorContains(t, err, "details suppressed")
		require.NotContains(t, err.Error(), "SENSITIVE_VALUE")
		require.NotContains(t, output, "SENSITIVE_VALUE")
	}
	output, err := executeSecret(t, deps, "", "list")
	require.ErrorContains(t, err, "details suppressed")
	require.NotContains(t, output+err.Error(), "SENSITIVE_VALUE")
	deps.delete = func(string, string) error { return keyring.ErrNotFound }
	output, err = executeSecret(t, deps, "y\n", "remove", "github")
	require.NoError(t, err)
	require.Contains(t, output, "already absent")
}

func TestSecretSetRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"", "bad\nvalue", "bad\x00value", strings.Repeat("x", 16385)} {
		deps := secretDependencies{
			read: func(*cobra.Command) (string, error) { return value, nil },
			set:  func(string, string, string) error { t.Fatal("must not store invalid input"); return nil },
		}
		_, err := executeSecret(t, deps, "", "set", "github")
		require.Error(t, err)
	}
}

func TestSecretSetRequiresInteractiveTerminal(t *testing.T) {
	t.Parallel()
	deps := secretDependencies{
		read: readSecretInput,
	}
	_, err := executeSecret(t, deps, "secret\n", "set", "github")
	require.ErrorContains(t, err, "interactive terminal")
}

func TestSecretCommandArguments(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"secret"}, {"secret", "unknown"}, {"secret", "set"}, {"secret", "remove"}, {"secret", "rm"},
		{"secret", "set", "github", "token"}, {"secret", "remove", "github", "extra"}, {"secret", "rm", "github", "extra"},
		{"secret", "list", "extra"}, {"secret", "ls", "extra"}, {"secret", "list", "--account", "github"},
	} {
		_, err := executeCommand(args...)
		require.Error(t, err)
	}
}

func TestSecretCommandHelp(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"secret", "--help"}, {"secret", "set", "--help"}, {"secret", "remove", "--help"}, {"secret", "rm", "--help"}, {"secret", "list", "--help"}, {"secret", "ls", "--help"}} {
		_, err := executeCommand(args...)
		require.NoError(t, err)
	}
}

func TestSecretListOnlyListsMetadata(t *testing.T) {
	t.Parallel()
	for _, service := range []string{"opencode-sandbox", "company"} {
		deps := secretDependencies{
			list: func(got string) ([]string, error) {
				require.Equal(t, service, got)
				return []string{"github", "context7", "github", "unsafe\x1b[2J"}, nil
			},
		}
		output, err := executeSecret(t, deps, "", "list", "--service", service)
		require.NoError(t, err)
		require.Equal(t, "\"context7\"\n\"github\"\n\"unsafe\\x1b[2J\"\n", output)
	}
}

func TestSecretListReportsEmptyKeyring(t *testing.T) {
	t.Parallel()
	deps := secretDependencies{list: func(service string) ([]string, error) {
		require.Equal(t, "opencode-sandbox", service)
		return nil, nil
	}}
	output, err := executeSecret(t, deps, "", "ls")
	require.NoError(t, err)
	require.Contains(t, output, "No keyring entries")
}
