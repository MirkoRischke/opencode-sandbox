package main

import (
	"context"
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
	if len(os.Args) != 1 {
		return fmt.Errorf("usage: opencode-sandbox (run inside a Git worktree)")
	}
	ctx := context.Background()
	s, err := sandbox.Prepare(ctx, ".")
	if err != nil {
		return err
	}
	defer s.Close()
	if err := s.ImportOpenAI(ctx); err != nil {
		return err
	}
	return s.Open(ctx)
}
