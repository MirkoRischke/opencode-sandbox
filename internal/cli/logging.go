package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"time"

	"opencode-sandbox/internal/logging"
	"opencode-sandbox/internal/workspace"
)

// startLogging is best-effort: logging errors must not block workspace operations.
func startLogging(ctx context.Context, stateDir, root, command string, warnings io.Writer) (context.Context, func(error)) {
	logger, closer, err := logging.Open(stateDir, root, warnings)
	if err != nil {
		fmt.Fprintln(warnings, "warning: launcher logging unavailable:", err) //nolint:errcheck // Preserve the operation even if warnings cannot be written.
		return ctx, func(error) {}
	}
	logger = logger.With("command", command)
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
			logger.Error("Invocation failed", "error", err, "duration_ms", time.Since(started).Milliseconds())
		} else {
			logger.Info("Invocation finished", "duration_ms", time.Since(started).Milliseconds())
		}
		_ = closer.Close() //nolint:errcheck // The log writer reports close failures itself.
	}
}

// logWorkspaceOperation also supports --all from an ordinary non-Git directory.
func logWorkspaceOperation(ctx context.Context, command string, warnings io.Writer, operation func(context.Context) error) (err error) {
	w, resolveErr := workspace.Resolve(ctx, ".")
	if resolveErr != nil {
		fmt.Fprintln(warnings, "warning: launcher logging unavailable:", resolveErr) //nolint:errcheck // --all must still work outside a valid workspace.
		return operation(ctx)
	}
	stateDir, stateErr := hostStateDir(os.Getenv("XDG_STATE_HOME"), os.Getenv("HOME"))
	if stateErr != nil {
		fmt.Fprintln(warnings, "warning: launcher logging unavailable:", stateErr) //nolint:errcheck // Logging is best-effort.
		return operation(ctx)
	}
	ctx, finish := startLogging(ctx, stateDir, w.Root(), command, warnings)
	defer func() { finish(err) }()
	return operation(ctx)
}
