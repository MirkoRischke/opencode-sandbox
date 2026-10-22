package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"opencode-sandbox/internal/config"
	"opencode-sandbox/internal/sandbox"

	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"
	"golang.org/x/term"
)

type secretDependencies struct {
	read   func(*cobra.Command) (string, error)
	set    func(string, string, string) error
	delete func(string, string) error
	list   func(string) ([]string, error)
}

func newSecretCommand() *cobra.Command {
	return secretCommand(secretDependencies{
		read: readSecretInput, set: keyring.Set, delete: keyring.Delete, list: keyring.ListUsers,
	})
}

// keyringEntry identifies metadata only; secret values never enter command output.
type keyringEntry struct {
	name, service, account string
}

func secretCommand(deps secretDependencies) *cobra.Command {
	var service string
	command := &cobra.Command{
		Use: "secret", Short: "Manage OS-keyring credentials without reading or changing configuration",
		Args: cobra.NoArgs, RunE: requireSubcommand,
	}
	command.PersistentFlags().StringVar(&service, "service", config.DefaultKeyringService, "Keyring service namespace")
	set := secretEntryCommand("set", "Store or replace a keyring credential using hidden input", &service, deps.store)
	remove := secretEntryCommand("remove", "Remove a keyring credential after confirmation", &service, deps.remove)
	remove.Aliases = []string{"rm"}
	command.AddCommand(set, remove, &cobra.Command{
		Use: "list", Aliases: []string{"ls"}, Short: "List keyring account names without reading secret values",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return deps.listAccounts(cmd, service) },
	})
	return command
}

// Both mutations validate and announce the same metadata before keyring access.
func secretEntryCommand(operation, description string, service *string, run func(*cobra.Command, keyringEntry) error) *cobra.Command {
	var account string
	command := &cobra.Command{
		Use: operation + " <name>", Short: description, Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if account == "" {
				account = args[0]
			}
			entry := keyringEntry{name: args[0], service: *service, account: account}
			if err := validateKeyringIdentifiers(entry.service, entry.account); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "Secret %q: keyring service %q, account %q\n", entry.name, entry.service, entry.account); err != nil {
				return err
			}
			return run(cmd, entry)
		},
	}
	command.Flags().StringVar(&account, "account", "", "Keyring account (defaults to <name>)")
	return command
}

func (deps secretDependencies) store(cmd *cobra.Command, entry keyringEntry) error {
	value, err := deps.read(cmd)
	if err != nil {
		return err
	}
	if err := sandbox.ValidateSecretValue(value); err != nil {
		return err
	}
	if err := deps.set(entry.service, entry.account, value); err != nil {
		return fmt.Errorf("store secret %q: keyring operation failed; check availability, unlock state, and provider size limits (details suppressed)", entry.name)
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), "Secret stored. Recreate existing VMs to use the new value.")
	return err
}

func (deps secretDependencies) remove(cmd *cobra.Command, entry keyringEntry) error {
	if _, err := fmt.Fprint(cmd.ErrOrStderr(), "This keyring entry may be shared across projects. Deleting it does not revoke credentials or remove copies in existing VMs. Delete? [y/N] "); err != nil {
		return err
	}
	answer, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil {
		return fmt.Errorf("secret deletion not confirmed: %w", err)
	}
	if answer = strings.ToLower(strings.TrimSpace(answer)); answer != "y" && answer != "yes" {
		return fmt.Errorf("secret deletion not confirmed")
	}
	if err := deps.delete(entry.service, entry.account); errors.Is(err, keyring.ErrNotFound) {
		_, err = fmt.Fprintln(cmd.OutOrStdout(), "Keyring entry already absent.")
		return err
	} else if err != nil {
		return fmt.Errorf("delete secret %q: keyring operation failed; check that the keyring is available and unlocked (details suppressed)", entry.name)
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), "Keyring entry deleted. Existing VMs retain their stored credentials; revoke upstream for immediate revocation.")
	return err
}

func (deps secretDependencies) listAccounts(cmd *cobra.Command, service string) error {
	if err := validateKeyringIdentifiers(service); err != nil {
		return err
	}
	accounts, err := deps.list(service)
	if err != nil {
		return fmt.Errorf("list keyring entries: keyring operation failed; check that the keyring is available and unlocked (details suppressed)")
	}
	if len(accounts) == 0 {
		_, err = fmt.Fprintln(cmd.OutOrStdout(), "No keyring entries found.")
		return err
	}
	slices.Sort(accounts)
	for _, account := range slices.Compact(accounts) {
		// Quote untrusted keychain metadata to prevent terminal escape injection.
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%q\n", account); err != nil {
			return err
		}
	}
	return nil
}

func validateKeyringIdentifiers(values ...string) error {
	for _, value := range values {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf("keyring service/account must be non-empty and contain no newlines or NUL bytes")
		}
	}
	return nil
}

func readSecretInput(cmd *cobra.Command) (string, error) {
	input, ok := cmd.InOrStdin().(*os.File)
	if !ok || !term.IsTerminal(int(input.Fd())) {
		return "", fmt.Errorf("secret set requires an interactive terminal for hidden input")
	}
	if _, err := fmt.Fprint(cmd.ErrOrStderr(), "Secret value (hidden): "); err != nil {
		return "", err
	}
	value, err := term.ReadPassword(int(input.Fd()))
	if _, writeErr := fmt.Fprintln(cmd.ErrOrStderr()); writeErr != nil {
		return "", writeErr
	}
	if err != nil {
		return "", fmt.Errorf("read secret input failed (details suppressed)")
	}
	return string(value), nil
}
