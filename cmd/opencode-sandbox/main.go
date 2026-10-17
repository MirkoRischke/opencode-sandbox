package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"

	launcherconfig "opencode-sandbox/internal/config"
	"opencode-sandbox/internal/sandbox"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() (err error) {
	if len(os.Args) > 1 && os.Args[1] == "config" {
		if len(os.Args) != 3 || os.Args[2] != "default" {
			return fmt.Errorf("usage: opencode-sandbox config default")
		}
		return launcherconfig.WriteDefaultConfig(os.Stdout)
	}
	if len(os.Args) > 1 && (os.Args[1] == "list" || os.Args[1] == "ls" || os.Args[1] == "remove" || os.Args[1] == "rm") {
		if len(os.Args) != 2 {
			return fmt.Errorf("usage: opencode-sandbox %s", os.Args[1])
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
		defer cancel()
		if os.Args[1] == "remove" || os.Args[1] == "rm" {
			return sandbox.Remove(ctx)
		}
		vms, err := sandbox.List(ctx)
		if err != nil {
			return err
		}
		if len(vms) == 0 {
			_, err := fmt.Fprintln(os.Stdout, "No managed VMs found.")
			return err
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		if _, err := fmt.Fprintln(w, "NAME\tSTATE"); err != nil {
			return err
		}
		for _, vm := range vms {
			if _, err := fmt.Fprintf(w, "%s\t%s\n", vm.Name(), vm.Status()); err != nil {
				return err
			}
		}
		return w.Flush()
	}
	flags := flag.NewFlagSet("opencode-sandbox", flag.ContinueOnError)
	flags.Usage = func() {
		//nolint:errcheck // flag.Usage cannot return output errors.
		fmt.Fprintln(flags.Output(), `Usage: opencode-sandbox [launch flags] | list (ls) | remove (rm) | config default
  list, ls    List all managed VMs, including stopped VMs
  remove, rm  Stop and delete all managed VMs without confirmation
  config default  Print documented launcher defaults without loading configuration`)
		flags.PrintDefaults()
	}
	configPath := flags.String("config", "", "Guest OpenCode JSON configuration file")
	cliConfigPath := flags.String("cli-config", "", "Guest OpenCode CLI JSON settings file")
	awsRole := flags.String("aws-role", "", "IAM role ARN to assume for this session (optional)")
	awsProfile := flags.String("aws-profile", "", "Optional host AWS source profile (requires --aws-role)")
	if err := flags.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("usage: opencode-sandbox [--config path] [--cli-config path] [--aws-role arn [--aws-profile name]] | list (ls) | remove (rm) (launch inside a Git worktree)")
	}
	if err := identifyHerdrAgent(); err != nil {
		return err
	}
	var awsOptions *sandbox.AWSOptions
	if *awsRole != "" || *awsProfile != "" {
		awsOptions = &sandbox.AWSOptions{SourceProfile: *awsProfile, RoleARN: *awsRole}
		if err := awsOptions.Validate(); err != nil {
			return err
		}
	}
	config, err := sandbox.LoadConfig(*configPath)
	if err != nil {
		return err
	}
	cliConfig, err := sandbox.LoadCLIConfig(*cliConfigPath)
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
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer cancel()
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
