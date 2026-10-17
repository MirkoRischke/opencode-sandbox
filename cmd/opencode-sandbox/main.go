package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"opencode-sandbox/internal/sandbox"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() (err error) {
	flags := flag.NewFlagSet("opencode-sandbox", flag.ContinueOnError)
	configPath := flags.String("config", "", "Guest OpenCode JSON configuration file")
	cliConfigPath := flags.String("cli-config", "", "Guest OpenCode CLI JSON settings file")
	awsRole := flags.String("aws-role", "", "IAM role ARN to assume for this session (optional)")
	awsProfile := flags.String("aws-profile", "", "Optional host AWS source profile (requires --aws-role)")
	if err := flags.Parse(os.Args[1:]); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("usage: opencode-sandbox [--config path] [--cli-config path] [--aws-role arn [--aws-profile name]] (run inside a Git worktree)")
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
	settings, err := sandbox.LoadSettings()
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
