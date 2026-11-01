package cli

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"opencode-sandbox/internal/logging"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestExecuteLogsReturnedErrorOnce(t *testing.T) {
	t.Parallel()
	state := t.TempDir()
	var stderr, stdout bytes.Buffer
	command := NewCommand()
	command.SetErr(&stderr)
	command.SetOut(&stdout)
	command.AddCommand(&cobra.Command{
		Use: "probe",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, finish := startLogging(cmd.Context(), state, "/project", "probe", cmd.ErrOrStderr())
			defer finish(errors.New("failure"))
			logging.FromContext(ctx).Info("file-only info")
			logging.FromContext(ctx).Warn("visible warning")
			return errors.New("the underlying failure")
		},
	})
	command.SetArgs([]string{"probe", "--console-log-level", "warn"})
	require.Equal(t, 1, Execute(context.Background(), command))
	require.Empty(t, stdout.String())
	require.NotContains(t, stderr.String(), "file-only info")
	require.Contains(t, stderr.String(), "visible warning")
	require.Equal(t, 1, strings.Count(stderr.String(), "the underlying failure"))
	files, err := filepath.Glob(filepath.Join(logging.Directory(state, "/project"), "*.log"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	data, err := os.ReadFile(files[0])
	require.NoError(t, err)
	require.Contains(t, string(data), "file-only info")
	require.Equal(t, 1, strings.Count(string(data), "the underlying failure"))
}

func TestConsoleFlagOverridesConfiguration(t *testing.T) {
	t.Parallel()
	for _, level := range []string{"debug", "info", "warn", "error"} {
		t.Run(level, func(t *testing.T) {
			t.Parallel()
			var stderr bytes.Buffer
			command := NewCommand()
			command.SetErr(&stderr)
			command.AddCommand(&cobra.Command{
				Use: "probe",
				RunE: func(cmd *cobra.Command, _ []string) error {
					// Simulate settings becoming available after startup.
					require.NoError(t, applyConsoleLevel(cmd.Context(), "error"))
					logger := logging.FromContext(cmd.Context())
					logger.Debug("debug event")
					logger.Info("info event")
					logger.Warn("warn event")
					logger.Error("error event")
					return nil
				},
			})
			command.SetArgs([]string{"--console-log-level", level, "probe"})
			require.Zero(t, Execute(context.Background(), command))
			require.Contains(t, stderr.String(), level+" event")
			require.Contains(t, stderr.String(), "error event")
			if level != "debug" {
				require.NotContains(t, stderr.String(), "debug event")
			}
			if level == "error" {
				require.NotContains(t, stderr.String(), "warn event")
			}
		})
	}
}

func TestInvalidConsoleFlag(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	command := NewCommand()
	command.SetErr(&stderr)
	command.SetArgs([]string{"config", "default", "--console-log-level", "trace"})
	require.Equal(t, 1, Execute(context.Background(), command))
	require.Contains(t, stderr.String(), "--console-log-level must be")
}

func TestConsoleDefaultsToInfo(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	e := newExecution(&stderr)
	e.logger.Debug("hidden details")
	e.logger.Info("Preparing VM")
	require.Equal(t, "INFO Preparing VM\n", stderr.String())
}

func TestConfiguredConsoleLevelAppliesWithoutFlag(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	e := newExecution(&stderr)
	ctx := context.WithValue(context.Background(), executionKey{}, e)
	require.NoError(t, applyConsoleLevel(ctx, "info"))
	e.logger.Info("visible info")
	require.NoError(t, applyConsoleLevel(ctx, "error"))
	e.logger.Warn("hidden warning")
	require.Contains(t, stderr.String(), "visible info")
	require.NotContains(t, stderr.String(), "hidden warning")
}

func TestInvalidConfiguredConsoleLevelRetainsCause(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	e := newExecution(&stderr)
	ctx := context.WithValue(t.Context(), executionKey{}, e)

	err := applyConsoleLevel(ctx, "not-a-level")

	require.ErrorContains(t, err, `parse console log level "not-a-level"`)
	require.ErrorContains(t, err, "slog:")
	require.Error(t, errors.Unwrap(err))
	require.Equal(t, slog.LevelInfo, e.level.Level())
	require.Empty(t, stderr.String(), "caller handles the returned error")
	e.override = true
	require.NoError(t, applyConsoleLevel(ctx, "not-a-level"), "explicit flag overrides config")
}
