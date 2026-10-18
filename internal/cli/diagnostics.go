package cli

import (
	"opencode-sandbox/internal/sandbox"

	"github.com/spf13/cobra"
)

func newDiagnosticsCommand() *cobra.Command {
	diagnostics := &cobra.Command{
		Use:   "diagnostics",
		Short: "Inspect saved sandbox diagnostics",
		Args:  cobra.NoArgs,
		RunE:  requireSubcommand,
	}
	diagnostics.AddCommand(&cobra.Command{
		Use:   "network",
		Short: "Print saved network diagnostics JSON for the current worktree",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return sandbox.WriteNetworkDiagnostics(cmd.Context(), ".", cmd.OutOrStdout())
		},
	})
	return diagnostics
}
