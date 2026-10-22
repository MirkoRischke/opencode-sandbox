package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirectoryIdentity(t *testing.T) {
	t.Parallel()
	state := t.TempDir()
	a := Directory(state, "/one/my project")
	b := Directory(state, "/two/my project")
	require.NotEqual(t, a, b)
	require.Equal(t, a, Directory(state, "/one/my project"))
	require.True(t, managedDirectory.MatchString(filepath.Base(a)))
	require.Contains(t, a, "my_project-")
	_, err := os.Stat(a)
	require.True(t, os.IsNotExist(err), "looking up a directory must not create it")
}

func TestJSONLogsArePrivateAndUnbuffered(t *testing.T) {
	t.Parallel()
	state := t.TempDir()
	var warnings bytes.Buffer
	logger, closer, err := Open(state, "/example/workspace", &warnings)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, closer.Close()) })
	ctx := WithLogger(context.Background(), logger)
	FromContext(ctx).Error("Operation failed", "error", fmt.Errorf("outer: %w", io.ErrUnexpectedEOF))
	files := logFiles(t, Directory(state, "/example/workspace"))
	require.Len(t, files, 1)
	contents, err := os.ReadFile(files[0])
	require.NoError(t, err)
	var record map[string]any
	require.NoError(t, json.Unmarshal(contents, &record))
	require.Equal(t, "outer: unexpected EOF", record["error"])
	require.Equal(t, "/example/workspace", record["workspace"])
	require.Equal(t, "ERROR", record["level"])
	for _, path := range []string{filepath.Dir(files[0]), filepath.Dir(filepath.Dir(files[0])), files[0]} {
		info, err := os.Stat(path)
		require.NoError(t, err)
		if info.IsDir() {
			require.Equal(t, os.FileMode(0700), info.Mode().Perm())
		} else {
			require.Equal(t, os.FileMode(0600), info.Mode().Perm())
		}
	}
	require.Empty(t, warnings.String())
}

func testWriter(t *testing.T, state, root string, now time.Time) *writer {
	t.Helper()
	w := &writer{directory: Directory(state, root), warnings: io.Discard, now: func() time.Time { return now }, limit: maxBytes}
	require.NoError(t, privateDirectory(filepath.Dir(w.directory)))
	require.NoError(t, w.rotate(now))
	t.Cleanup(func() { require.NoError(t, w.Close()) })
	return w
}

func logFiles(t *testing.T, directory string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(directory, "*.log"))
	require.NoError(t, err)
	return files
}

func TestRotationBySizeAndLocalDate(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 21, 23, 59, 59, 0, time.FixedZone("local", 2*60*60))
	w := testWriter(t, t.TempDir(), "/project", now)
	w.limit = 5
	_, err := w.Write([]byte("12345"))
	require.NoError(t, err)
	_, err = w.Write([]byte("next"))
	require.NoError(t, err)
	files := logFiles(t, w.directory)
	require.Len(t, files, 2)
	for _, file := range files {
		require.True(t, managedFile.MatchString(filepath.Base(file)))
		require.Contains(t, file, "2026-10-21T23-59-59.000000000+0200_")
	}
	w.now = func() time.Time { return now.Add(time.Second) }
	_, err = w.Write([]byte("day2"))
	require.NoError(t, err)
	files = logFiles(t, w.directory)
	require.Len(t, files, 3)
	require.Contains(t, filepath.Base(w.file.Name()), "2026-10-22T00-00-00.000000000+0200_")
}

func TestConcurrentWritersAndCleanup(t *testing.T) {
	t.Parallel()
	state := t.TempDir()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			writeRotatingLogs(t, state, 10)
		})
	}
	wg.Wait()
	files := logFiles(t, Directory(state, "/shared"))
	require.LessOrEqual(t, len(files), maxFiles)
	for _, file := range files {
		data, err := os.ReadFile(file)
		require.NoError(t, err)
		for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
			require.True(t, json.Valid(line), "invalid JSON in %s: %s", file, line)
		}
	}
}

func writeRotatingLogs(t *testing.T, state string, records int) {
	t.Helper()
	logger, closer, err := Open(state, "/shared", io.Discard)
	if !assert.NoError(t, err) {
		return
	}
	w, ok := closer.(*writer)
	if !ok {
		t.Error("unexpected log writer")
		return
	}
	w.limit = 150
	for range records {
		logger.Info("Concurrent record", "value", strings.Repeat("x", 80))
	}
	assert.NoError(t, closer.Close())
}

func TestWriteFailureWarnsWithoutBlocking(t *testing.T) {
	t.Parallel()
	w := testWriter(t, t.TempDir(), "/project", time.Now())
	var warnings bytes.Buffer
	w.warnings = &warnings
	require.NoError(t, w.file.Close())
	logger := slog.New(slog.NewJSONHandler(w, nil))
	logger.Info("lost record")
	logger.Info("another lost record")
	require.Equal(t, 1, strings.Count(warnings.String(), "warning:"))
	// Already deliberately closed above, rather than by normal lifecycle.
	w.closed = true
}
