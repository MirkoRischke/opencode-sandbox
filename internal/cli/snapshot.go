package cli

import (
	"context"
	"fmt"
	"os"

	"opencode-sandbox/internal/config"
	"opencode-sandbox/internal/logging"
	"opencode-sandbox/internal/sandbox"
	"opencode-sandbox/internal/workspace"

	"github.com/spf13/cobra"
)

func newSnapshotCommand() *cobra.Command {
	command := &cobra.Command{
		Use: "snapshot", Short: "Manage prepared workspace disk baselines",
		Args: cobra.NoArgs, RunE: requireSubcommand,
	}
	command.AddCommand(newSnapshotListCommand(), newSnapshotCreateCommand(), newSnapshotRemoveCommand())
	return command
}

func newSnapshotRemoveCommand() *cobra.Command {
	var all bool
	remove := &cobra.Command{
		Use: "remove", Short: "Delete workspace snapshots without removing VMs",
		Aliases: []string{"rm"},
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return logWorkspaceOperation(cmd.Context(), "snapshot remove", cmd.ErrOrStderr(), func(ctx context.Context) error {
				return reportRemovals(cmd.OutOrStdout(), "snapshot", func(observer sandbox.RemovalObserver) error {
					logged := func(name string, size *uint64) {
						logging.FromContext(ctx).Info("Snapshot removed", "snapshot", name, "size_bytes", size)
						observer(name, size)
					}
					if all {
						return sandbox.RemoveAllSnapshots(ctx, logged)
					}
					return sandbox.RemoveSnapshot(ctx, ".", logged)
				})
			})
		},
	}
	remove.Flags().BoolVar(&all, "all", false, "Delete managed snapshots for all workspaces without confirmation")
	return remove
}

func newSnapshotCreateCommand() *cobra.Command {
	return &cobra.Command{
		Use: "create", Short: "Prepare a fresh VM and replace this directory's baseline",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) (err error) {
			configDir, err := hostConfigDir(os.Getenv("XDG_CONFIG_HOME"), os.Getenv("HOME"))
			if err != nil {
				return err
			}
			stateDir, err := hostStateDir(os.Getenv("XDG_STATE_HOME"), os.Getenv("HOME"))
			if err != nil {
				return err
			}
			paths := config.Paths{ConfigDir: configDir, StateDir: stateDir}
			w, err := workspace.Resolve(cmd.Context(), ".")
			if err != nil {
				return err
			}
			ctx, finish := startLogging(cmd.Context(), stateDir, w.Root(), "snapshot create", cmd.ErrOrStderr())
			defer func() { finish(err) }()
			paths.Logger = logging.FromContext(ctx)
			settings, err := paths.LoadWorkspaceSettings(w.Root(), approveWorkspaceConfig)
			if err != nil {
				return err
			}
			paths.Logger.Info("Effective configuration", "settings", settings)
			secrets, err := sandbox.LoadSecrets(settings)
			if err != nil {
				return err
			}
			reference, err := sandbox.CreateWorkspaceSnapshot(ctx, w, secrets, settings,
				sandbox.DiagnosticsOptions{StateDir: stateDir, Output: cmd.ErrOrStderr()})
			if err != nil {
				return err
			}
			paths.Logger.Info("Workspace snapshot created", "snapshot", reference)
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "Workspace snapshot created:", reference)
			return err
		},
	}
}
