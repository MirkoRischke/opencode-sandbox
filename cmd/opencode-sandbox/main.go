package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"opencode-sandbox/internal/cli"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer cancel()
	return cli.Execute(ctx, cli.NewCommand())
}
