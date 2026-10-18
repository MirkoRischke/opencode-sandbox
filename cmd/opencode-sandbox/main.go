package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"opencode-sandbox/internal/cli"
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
	return cli.NewCommand().ExecuteContext(ctx)
}
