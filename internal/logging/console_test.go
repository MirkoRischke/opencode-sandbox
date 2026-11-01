package logging

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConsoleProgressAndAttributes(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	logger := Console(&output, slog.LevelInfo).With("workspace", "/project", "pid", 123, "command", "launch", "vm", "example")
	logger.Debug("hidden details")
	logger.Info("VM connected", "created", true, "duration_ms", 2300, "image", "ubuntu:24.04", "new_field", "retained")
	logger.Error("Operation failed", "error", errors.New("permission denied"))
	require.Equal(t, "INFO VM connected created=true (2.3s) image=ubuntu:24.04 new_field=retained\nERROR Operation failed error=\"permission denied\"\n", output.String())
}

func TestConsoleGroupsAndDerivedHandlers(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	logger := Console(&output, nil).With("component", "test")
	group := logger.WithGroup("operation").With("attempt", 1)
	first := group.With("branch", "first")
	second := group.With("branch", "second")
	first.Info("First", slog.Group("nested", "path", "/my project"), slog.Group("", "inline", true), slog.Attr{}, slog.Group("empty"))
	second.Info("Second")
	logger.Info("Root")
	require.Equal(t, "INFO First component=test operation.attempt=1 operation.branch=first operation.nested.path=\"/my project\" operation.inline=true\nINFO Second component=test operation.attempt=1 operation.branch=second\nINFO Root component=test\n", output.String())
}

func TestConsoleEscapesControlCharacters(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	Console(&output, nil).Info("message\nnext line", "path", "a\nb")
	require.Equal(t, "INFO \"message\\nnext line\" path=\"a\\nb\"\n", output.String())
}

func TestConsoleConcurrentDerivedHandlers(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	logger := Console(&output, nil)
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Go(func() { logger.With("worker", i).Info("Working") })
	}
	wg.Wait()
	require.Len(t, bytes.Split(bytes.TrimSpace(output.Bytes()), []byte{'\n'}), 10)
}

func TestConsolePresentationDoesNotChangeJSON(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	state := t.TempDir()
	logger, closer, err := Open(state, "/project", &output)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, closer.Close()) })
	logger.Debug("Effective configuration", "settings", "diagnostic detail")
	logger.Info("VM ready", "vm", "example", "duration_ms", 2300)
	require.Equal(t, "INFO VM ready (2.3s)\n", output.String())
	files := logFiles(t, Directory(state, "/project"))
	require.Len(t, files, 1)
	data, err := os.ReadFile(files[0])
	require.NoError(t, err)
	for _, field := range []string{`"level":"DEBUG"`, `"settings":"diagnostic detail"`, `"workspace":"/project"`, `"pid":`, `"vm":"example"`, `"duration_ms":2300`} {
		require.Contains(t, string(data), field)
	}
}
