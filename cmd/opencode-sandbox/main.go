package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"

	launcherconfig "opencode-sandbox/internal/config"
	"opencode-sandbox/internal/sandbox"

	"github.com/spf13/cobra"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer cancel()
	return newCommand().ExecuteContext(ctx)
}

func newCommand() *cobra.Command {
	var configPath, cliConfigPath, awsRole, awsProfile string
	root := &cobra.Command{
		Use:           "opencode-sandbox",
		Short:         "Launch OpenCode in an isolated VM for the current Git worktree",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true, // main prints errors once, including runtime failures.
		RunE: func(cmd *cobra.Command, _ []string) error {
			return launch(cmd.Context(), configPath, cliConfigPath, awsRole, awsProfile)
		},
	}
	root.Flags().StringVar(&configPath, "config", "", "Guest OpenCode JSON configuration file")
	root.Flags().StringVar(&cliConfigPath, "cli-config", "", "Guest OpenCode CLI JSON settings file")
	root.Flags().StringVar(&awsRole, "aws-role", "", "IAM role ARN to assume for this session (optional)")
	root.Flags().StringVar(&awsProfile, "aws-profile", "", "Optional host AWS source profile (requires --aws-role)")
	root.AddCommand(&cobra.Command{
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
	}, &cobra.Command{
		Use:     "remove",
		Aliases: []string{"rm"},
		Short:   "Stop and delete all managed VMs without confirmation",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return sandbox.Remove(cmd.Context())
		},
	})
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
	root.AddCommand(config, diagnostics)
	return root
}

func requireSubcommand(cmd *cobra.Command, _ []string) error {
	return fmt.Errorf("a subcommand is required for %s; run '%s --help' for usage", cmd.CommandPath(), cmd.CommandPath())
}

func launch(ctx context.Context, configPath, cliConfigPath, awsRole, awsProfile string) (err error) {
	if err := identifyHerdrAgent(); err != nil {
		return err
	}
	var awsOptions *sandbox.AWSOptions
	if awsRole != "" || awsProfile != "" {
		awsOptions = &sandbox.AWSOptions{SourceProfile: awsProfile, RoleARN: awsRole}
		if err := awsOptions.Validate(); err != nil {
			return err
		}
	}
	config, err := sandbox.LoadConfig(configPath)
	if err != nil {
		return err
	}
	cliConfig, err := sandbox.LoadCLIConfig(cliConfigPath)
	if err != nil {
		return err
	}
	settings, err := launcherconfig.LoadRepoSettings(".", approveRepoConfig)
	if err != nil {
		return err
	}
	secrets, err := sandbox.LoadSecrets(settings)
	if err != nil {
		return err
	}
	s, err := sandbox.Prepare(ctx, ".", config, secrets, settings)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, s.Shutdown()) }()
	stopAWS, err := s.StartAWS(ctx, awsOptions)
	if err != nil {
		return err
	}
	defer stopAWS()
	if err := s.SyncTUIConfig(ctx, cliConfig); err != nil {
		return err
	}
	if err := s.ImportOpenAI(ctx); err != nil {
		return err
	}
	return s.Open(ctx)
}

func identifyHerdrAgent() error {
	if os.Getenv("HERDR_ENV") != "1" || os.Getenv("HERDR_AGENT") != "" {
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("identify Herdr agent: %w", err)
	}
	if err := os.Setenv("HERDR_AGENT", "opencode"); err != nil {
		return fmt.Errorf("identify Herdr agent: %w", err)
	}
	// Herdr reads /proc/<pid>/environ on Linux, which exposes the initial
	// environment rather than later Setenv changes. Exec preserves the PID,
	// terminal, and arguments while making the hint visible to Herdr.
	return syscall.Exec(executable, os.Args, os.Environ())
}
