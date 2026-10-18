package cli

import (
	launcherconfig "opencode-sandbox/internal/config"

	"github.com/spf13/cobra"
)

func newConfigCommand() *cobra.Command {
	config := &cobra.Command{
		Use:   "config",
		Short: "Inspect launcher configuration",
		Args:  cobra.NoArgs,
		RunE:  requireSubcommand,
	}
	config.AddCommand(&cobra.Command{
		Use:   "default",
		Short: "Print documented launcher defaults without loading configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return launcherconfig.WriteDefaultConfig(cmd.OutOrStdout())
		},
	})
	return config
}
