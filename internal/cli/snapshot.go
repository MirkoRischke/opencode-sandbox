package cli

import (
	"fmt"
	"os"

	"opencode-sandbox/internal/config"
	"opencode-sandbox/internal/sandbox"

	"github.com/spf13/cobra"
)

func newSnapshotCommand() *cobra.Command {
	command := &cobra.Command{
		Use: "snapshot", Short: "Manage the prepared disk baseline shared by this repository's worktrees",
		Args: cobra.NoArgs, RunE: requireSubcommand,
	}
	command.AddCommand(&cobra.Command{
		Use: "create", Short: "Run snapshot.command in a fresh VM and replace this repository's baseline",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			configDir, err := hostConfigDir(os.Getenv("XDG_CONFIG_HOME"), os.Getenv("HOME"))
			if err != nil {
				return err
			}
			stateDir, err := hostStateDir(os.Getenv("XDG_STATE_HOME"), os.Getenv("HOME"))
			if err != nil {
				return err
			}
			paths := config.Paths{ConfigDir: configDir, StateDir: stateDir}
			settings, err := paths.LoadRepoSettings(".", approveRepoConfig)
			if err != nil {
				return err
			}
			secrets, err := sandbox.LoadSecrets(settings)
			if err != nil {
				return err
			}
			reference, err := sandbox.CreateSnapshot(cmd.Context(), ".", secrets, settings,
				sandbox.DiagnosticsOptions{StateDir: stateDir, Output: cmd.ErrOrStderr()})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "Repository snapshot created:", reference)
			return err
		},
	}, &cobra.Command{
		Use: "remove", Short: "Delete this repository's baseline without removing worktree VMs",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return sandbox.RemoveSnapshot(cmd.Context(), ".")
		},
	})
	return command
}
