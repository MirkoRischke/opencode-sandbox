package cli

import (
	"context"
	"io"
	"log/slog"
	"os"
	"runtime/debug"
	"time"

	"opencode-sandbox/internal/logging"
	"opencode-sandbox/internal/workspace"
)

// startLogging is best-effort: logging errors must not block workspace operations.
func startLogging(ctx context.Context, stateDir, root, command string, warnings io.Writer) (context.Context, func(error)) {
	var level slog.Leveler = slog.LevelWarn
	e, ok := ctx.Value(executionKey{}).(*execution)
	if ok {
		level = &e.level
	}
	logger, closer, err := logging.OpenWithLevel(stateDir, root, warnings, level)
	if err != nil {
		logger.Warn("Launcher logging unavailable", "error", err)
	}
	logger = logger.With("command", command)
	if e != nil && e.managed {
		e.logger = logger
		e.closers = append(e.closers, closer)
	}
	ctx = logging.WithLogger(ctx, logger)
	version, revision := "unknown", "unknown"
	if info, ok := debug.ReadBuildInfo(); ok {
		version = info.Main.Version
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				revision = setting.Value
			}
		}
	}
	started := time.Now()
	logger.Info("Invocation started", "version", version, "revision", revision)
	return ctx, func(err error) {
		if err != nil {
			logger.Info("Invocation finished", "success", false, "duration_ms", time.Since(started).Milliseconds())
		} else {
			logger.Info("Invocation finished", "duration_ms", time.Since(started).Milliseconds())
		}
		if e == nil || !e.managed {
			_ = closer.Close() //nolint:errcheck // The log writer reports close failures itself.
		}
	}
}

// logWorkspaceOperation also supports --all from an ordinary non-Git directory.
func logWorkspaceOperation(ctx context.Context, command string, warnings io.Writer, operation func(context.Context) error) (err error) {
	w, resolveErr := workspace.Resolve(ctx, ".")
	if resolveErr != nil {
		logging.FromContext(ctx).Warn("Launcher logging unavailable", "error", resolveErr)
		return operation(ctx)
	}
	stateDir, stateErr := hostStateDir(os.Getenv("XDG_STATE_HOME"), os.Getenv("HOME"))
	if stateErr != nil {
		logging.FromContext(ctx).Warn("Launcher logging unavailable", "error", stateErr)
		return operation(ctx)
	}
	ctx, finish := startLogging(ctx, stateDir, w.Root(), command, warnings)
	defer func() { finish(err) }()
	return operation(ctx)
}
