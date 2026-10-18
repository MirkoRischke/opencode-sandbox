package cli

import (
	"opencode-sandbox/internal/sandbox"
	"os"

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
		Short: "Print saved network diagnostics JSON for the current directory",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			stateDir, err := hostStateDir(os.Getenv("XDG_STATE_HOME"), os.Getenv("HOME"))
			if err != nil {
				return err
			}
			return sandbox.WriteNetworkDiagnostics(cmd.Context(), ".", stateDir, cmd.OutOrStdout())
		},
	})
	return diagnostics
}
