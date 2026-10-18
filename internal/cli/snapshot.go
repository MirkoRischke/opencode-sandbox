package cli

import (
	"fmt"
	"os"

	"opencode-sandbox/internal/config"
	"opencode-sandbox/internal/sandbox"

	"github.com/spf13/cobra"
)

func newSnapshotCommand() *cobra.Command {
	var all bool
	remove := &cobra.Command{
		Use: "remove", Short: "Delete repository snapshots without removing worktree VMs",
		Aliases: []string{"rm"},
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return reportRemovals(cmd.OutOrStdout(), "snapshot", func(observer sandbox.RemovalObserver) error {
				if all {
					return sandbox.RemoveAllSnapshots(cmd.Context(), observer)
				}
				return sandbox.RemoveSnapshot(cmd.Context(), ".", observer)
			})
		},
	}
	remove.Flags().BoolVar(&all, "all", false, "Delete managed snapshots for all repositories without confirmation")
	command := &cobra.Command{
		Use: "snapshot", Short: "Manage prepared repository disk baselines",
		Args: cobra.NoArgs, RunE: requireSubcommand,
	}
	command.AddCommand(newSnapshotListCommand(), &cobra.Command{
		Use: "create", Short: "Prepare a fresh VM and replace this repository's baseline",
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
	}, remove)
	return command
}
