package cli

import (
	"fmt"
	"text/tabwriter"

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
	var all bool
	command := &cobra.Command{
		Use:     "remove",
		Aliases: []string{"rm"},
		Short:   "Stop and delete this worktree's VM without confirmation",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if all {
				return sandbox.RemoveAll(cmd.Context())
			}
			return sandbox.Remove(cmd.Context(), ".")
		},
	}
	command.Flags().BoolVar(&all, "all", false, "Delete all managed VMs without confirmation")
	return command
}
