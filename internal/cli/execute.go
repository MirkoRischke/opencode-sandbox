package cli

import (
	"context"
	"io"
	"log/slog"

	"opencode-sandbox/internal/logging"

	"github.com/spf13/cobra"
)

type executionKey struct{}

// execution owns logging until the final error has been handled, after all
// command cleanup has completed. It is private to a single invocation.
type execution struct {
	managed  bool
	level    slog.LevelVar
	override bool
	logger   *slog.Logger
	closers  []io.Closer
}

func newExecution(output io.Writer) *execution {
	e := &execution{}
	e.level.Set(slog.LevelWarn)
	e.logger = logging.Console(output, &e.level)
	return e
}

// Execute handles command errors once and returns an exit status, not an error.
func Execute(ctx context.Context, command *cobra.Command) int {
	e := newExecution(command.ErrOrStderr())
	e.managed = true
	ctx = context.WithValue(ctx, executionKey{}, e)
	defer func() {
		for _, closer := range e.closers {
			_ = closer.Close() //nolint:errcheck // The logging writer reports close failures.
		}
	}()
	if err := command.ExecuteContext(ctx); err != nil {
		e.logger.Error("Invocation failed", "error", err)
		return 1
	}
	return 0
}

func applyConsoleLevel(ctx context.Context, level string) {
	if e, ok := ctx.Value(executionKey{}).(*execution); ok && !e.override {
		var parsed slog.Level
		if err := parsed.UnmarshalText([]byte(level)); err == nil {
			e.level.Set(parsed)
		}
	}
}
