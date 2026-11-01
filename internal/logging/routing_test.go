package logging

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDestinationLevelsAndFormats(t *testing.T) {
	t.Parallel()
	for _, level := range []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError} {
		t.Run(level.String(), func(t *testing.T) {
			t.Parallel()
			var console bytes.Buffer
			var threshold slog.LevelVar
			threshold.Set(level)
			state := t.TempDir()
			logger, closer, err := OpenWithLevel(state, "/project", &console, &threshold)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, closer.Close()) })
			logger = logger.With("component", "test").WithGroup("operation")
			for _, severity := range []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError} {
				logger.Log(t.Context(), severity, severity.String()+" record", "attempt", 1)
				if severity >= level {
					require.Contains(t, console.String(), severity.String()+" record")
				} else {
					require.NotContains(t, console.String(), severity.String()+" record")
				}
			}
			require.Contains(t, console.String(), "component=test")
			require.Contains(t, console.String(), "operation.attempt=1")
			files := logFiles(t, Directory(state, "/project"))
			require.Len(t, files, 1)
			data, err := os.ReadFile(files[0])
			require.NoError(t, err)
			for _, severity := range []string{"DEBUG", "INFO", "WARN", "ERROR"} {
				require.Contains(t, string(data), `"level":"`+severity+`"`)
			}
			console.Reset()
			threshold.Set(slog.LevelError)
			logger.Warn("hidden after update")
			logger.Error("visible after update")
			require.NotContains(t, console.String(), "hidden after update")
			require.Contains(t, console.String(), "visible after update")
		})
	}
}

func TestConsoleSurvivesFileSetupFailure(t *testing.T) {
	t.Parallel()
	state := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(state, nil, 0600))
	var console bytes.Buffer
	logger, closer, err := OpenWithLevel(state, "/project", &console, slog.LevelWarn)
	require.Error(t, err)
	require.NoError(t, closer.Close())
	logger.Warn("still visible")
	require.Contains(t, console.String(), "still visible")
}

func TestConsoleSurvivesFileWriteFailure(t *testing.T) {
	t.Parallel()
	var console bytes.Buffer
	logger, closer, err := OpenWithLevel(t.TempDir(), "/project", &console, slog.LevelWarn)
	require.NoError(t, err)
	w, ok := closer.(*writer)
	require.True(t, ok)
	require.NoError(t, w.file.Close())
	logger.Error("visible despite file failure")
	require.Contains(t, console.String(), "Launcher logging failed")
	require.Contains(t, console.String(), "visible despite file failure")
	require.NoError(t, closer.Close())
}
