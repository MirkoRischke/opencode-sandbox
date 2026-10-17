package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"opencode-sandbox/internal/sandbox"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	flags := flag.NewFlagSet("opencode-sandbox", flag.ContinueOnError)
	configPath := flags.String("config", "", "Guest OpenCode JSON configuration file")
	cliConfigPath := flags.String("cli-config", "", "Guest OpenCode CLI JSON settings file")
	if err := flags.Parse(os.Args[1:]); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("usage: opencode-sandbox [--config path] [--cli-config path] (run inside a Git worktree)")
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
	githubToken, err := sandbox.LoadGitHubToken()
	if err != nil {
		return err
	}
	ctx := context.Background()
	s, err := sandbox.Prepare(ctx, ".", config, githubToken, settings)
	if err != nil {
		return err
	}
	defer s.Close()
	if err := s.SyncTUIConfig(ctx, cliConfig); err != nil {
		return err
	}
	if err := s.ImportOpenAI(ctx); err != nil {
		return err
	}
	return s.Open(ctx)
}
