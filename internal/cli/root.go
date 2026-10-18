package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewCommand constructs the CLI without loading host state or launching a sandbox.
func NewCommand() *cobra.Command {
	var awsRole, awsProfile string
	root := &cobra.Command{
		Use:           "opencode-sandbox",
		Short:         "Launch OpenCode in an isolated VM for the current Git worktree",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true, // main prints errors once, including runtime failures.
		RunE: func(cmd *cobra.Command, _ []string) error {
			return launch(cmd.Context(), awsRole, awsProfile)
		},
	}
	root.Flags().StringVar(&awsRole, "aws-role", "", "IAM role ARN to assume for this session (optional)")
	root.Flags().StringVar(&awsProfile, "aws-profile", "", "Optional host AWS source profile (requires --aws-role)")
	root.AddCommand(newListCommand(), newRemoveCommand(), newConfigCommand(), newDiagnosticsCommand(), newSnapshotCommand())
	return root
}

func requireSubcommand(cmd *cobra.Command, _ []string) error {
	return fmt.Errorf("a subcommand is required for %s; run '%s --help' for usage", cmd.CommandPath(), cmd.CommandPath())
}
