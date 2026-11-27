package cli

import (
	"context"
	"errors"
	"fmt"
	"text/tabwriter"

	"opencode-sandbox/internal/logging"
	"opencode-sandbox/internal/sandbox"

	"github.com/spf13/cobra"
)

func newListCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all managed VMs, including stopped VMs",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			vms, err := sandbox.List(cmd.Context())
			if err != nil {
				return err
			}
			if len(vms) == 0 {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "No managed VMs found.")
				return err
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			if _, err := fmt.Fprintln(w, "NAME\tSTATE"); err != nil {
				return err
			}
			for _, vm := range vms {
				if _, err := fmt.Fprintf(w, "%s\t%s\n", vm.Name(), vm.Status()); err != nil {
					return err
				}
			}
			return w.Flush()
		},
	}
}

func newRemoveCommand() *cobra.Command {
	var all, orphaned, dryRun bool
	command := &cobra.Command{
		Use:     "remove",
		Aliases: []string{"rm"},
		Short:   "Stop and delete this directory's VM without confirmation",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Flags().Changed("dry-run") && !orphaned {
				return fmt.Errorf("--dry-run requires --orphaned")
			}
			if dryRun {
				return previewOrphanedVMs(cmd, all)
			}
			return logWorkspaceOperation(cmd.Context(), "remove", cmd.ErrOrStderr(), func(ctx context.Context) error {
				gateway, err := hostGateway()
				if err != nil {
					return err
				}
				var keyErrors []error
				removeErr := reportRemovals(cmd.OutOrStdout(), "VM", func(observer sandbox.RemovalObserver) error {
					logged := func(name string, size *uint64) {
						logging.FromContext(ctx).Info("VM removed", "vm", name)
						observer(name, size)
						keyErrors = append(keyErrors, gateway.DeleteKey(ctx, name))
					}
					if orphaned {
						return sandbox.RemoveOrphaned(ctx, ".", all, logged)
					}
					if all {
						return sandbox.RemoveAll(ctx, logged)
					}
					return sandbox.Remove(ctx, ".", logged)
				})
				return errors.Join(append(keyErrors, removeErr)...)
			})
		},
	}
	command.Flags().BoolVar(&all, "all", false, "Use all managed VMs; with --orphaned, check all repositories")
	command.Flags().BoolVar(&orphaned, "orphaned", false, "Delete stopped VMs for removed linked worktrees in the current Git repository")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "Show which VMs --orphaned would delete")
	return command
}

func previewOrphanedVMs(cmd *cobra.Command, all bool) error {
	vms, err := sandbox.OrphanedVMs(cmd.Context(), ".", all)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	if len(vms) == 0 {
		_, err := fmt.Fprintln(out, "Nothing to delete.")
		return err
	}
	for _, vm := range vms {
		if _, err := fmt.Fprintf(out, "Would delete VM %s (former worktree %q)\n", vm.Name, vm.WorktreePath); err != nil {
			return err
		}
	}
	return nil
}
