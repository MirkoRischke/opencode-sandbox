package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"opencode-sandbox/internal/config"
	"opencode-sandbox/internal/logging"
	"opencode-sandbox/internal/workspace"

	"github.com/spf13/cobra"
)

// NewCommand constructs the CLI without loading host state or launching a sandbox.
func NewCommand() *cobra.Command {
	var awsRole, awsProfile string
	root := &cobra.Command{
		Use:               "opencode-sandbox",
		Short:             "Launch OpenCode in an isolated VM for the current directory",
		Args:              cobra.NoArgs,
		SilenceUsage:      true,
		SilenceErrors:     true, // Execute handles errors once, including runtime failures.
		PersistentPreRunE: configureConsoleLogging,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var role, profile *string
			if cmd.Flags().Changed("aws-role") {
				role = &awsRole
			}
			if cmd.Flags().Changed("aws-profile") {
				profile = &awsProfile
			}
			return launch(cmd.Context(), role, profile)
		},
	}
	root.PersistentFlags().String("console-log-level", "", "Minimum stderr log level: debug, info, warn, or error (default info; overrides config)")
	root.Flags().StringVar(&awsRole, "aws-role", "", "IAM role ARN to assume for this session (optional)")
	root.Flags().StringVar(&awsProfile, "aws-profile", "", "Optional host AWS source profile (requires a role via config or --aws-role)")
	root.AddCommand(newListCommand(), newRemoveCommand(), newConfigCommand(), newDiagnosticsCommand(), newSnapshotCommand(), newSecretCommand())
	return root
}

func configureConsoleLogging(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	e, ok := ctx.Value(executionKey{}).(*execution)
	if !ok {
		e = newExecution(cmd.ErrOrStderr())
		ctx = context.WithValue(ctx, executionKey{}, e)
		cmd.SetContext(ctx)
	}
	ctx = logging.WithLogger(ctx, e.logger)
	cmd.SetContext(ctx)
	if cmd.Flags().Changed("console-log-level") {
		value, err := cmd.Flags().GetString("console-log-level")
		if err != nil {
			return err
		}
		switch value {
		case "debug", "info", "warn", "error":
		default:
			return fmt.Errorf("--console-log-level must be debug, info, warn, or error (got %q)", value)
		}
		var level slog.Level
		if err := level.UnmarshalText([]byte(value)); err != nil {
			return err
		}
		e.level.Set(level)
		e.override = true
	}
	// Pure documentation/completion commands do not read user configuration.
	if cmd.Name() == "default" || cmd.Name() == "completion" || cmd.Parent() != nil && cmd.Parent().Name() == "completion" || cmd.Name() == "__complete" {
		return nil
	}
	if e.override {
		return nil
	}
	configDir, err := hostConfigDir(os.Getenv("XDG_CONFIG_HOME"), os.Getenv("HOME"))
	if err != nil {
		return err
	}
	stateDir, err := hostStateDir(os.Getenv("XDG_STATE_HOME"), os.Getenv("HOME"))
	if err != nil {
		return err
	}
	var directory string
	if w, err := workspace.Resolve(ctx, "."); err == nil {
		directory = w.Root()
	}
	settings, err := (config.Paths{ConfigDir: configDir, StateDir: stateDir}).LoadLoggingSettings(directory)
	if err != nil {
		return err
	}
	return applyConsoleLevel(ctx, settings.ConsoleLevel)
}

func requireSubcommand(cmd *cobra.Command, _ []string) error {
	return fmt.Errorf("a subcommand is required for %s; run '%s --help' for usage", cmd.CommandPath(), cmd.CommandPath())
}
