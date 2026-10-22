package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func fixtureLog(t *testing.T, directory string, n int, modified time.Time) string {
	t.Helper()
	require.NoError(t, privateDirectory(directory))
	path := filepath.Join(directory, fmt.Sprintf("2026-10-01T00-00-00.000000000+0000_123_%d.log", n))
	require.NoError(t, os.WriteFile(path, []byte("fixture\n"), 0600))
	require.NoError(t, os.Chtimes(path, modified, modified))
	return path
}

func TestRetentionRemovesOnlyExpiredSegments(t *testing.T) {
	t.Parallel()
	now := time.Now()
	directory := Directory(t.TempDir(), "/project")
	expired := fixtureLog(t, directory, 1, now.Add(-maxAge-time.Second))
	boundary := fixtureLog(t, directory, 2, now.Add(-maxAge))
	recent := fixtureLog(t, directory, 3, now.Add(-time.Hour))

	require.NoError(t, cleanup(filepath.Dir(directory), now))

	require.NoFileExists(t, expired)
	require.FileExists(t, boundary)
	require.FileExists(t, recent)
}

func TestRetentionPreservesActiveSegmentsUntilClosed(t *testing.T) {
	t.Parallel()
	now := time.Now()
	w := testWriter(t, t.TempDir(), "/active", now)
	active := w.file.Name()
	expired := now.Add(-2 * maxAge)
	require.NoError(t, os.Chtimes(active, expired, expired))

	require.NoError(t, cleanup(filepath.Dir(w.directory), now))
	require.FileExists(t, active, "an active segment must survive its retention deadline")

	require.NoError(t, w.Close())
	require.NoError(t, cleanup(filepath.Dir(w.directory), now))
	require.NoDirExists(t, w.directory, "remove the empty workspace directory after its last segment")
}

func TestRetentionIgnoresUnknownFilesAndSymlinks(t *testing.T) {
	t.Parallel()
	now := time.Now()
	directory := Directory(t.TempDir(), "/project")
	target := fixtureLog(t, directory, 1, now)
	unknown := filepath.Join(directory, "keep.txt")
	require.NoError(t, os.WriteFile(unknown, []byte("unmanaged"), 0600))
	link := filepath.Join(directory, "2026-10-01T00-00-00.000000000+0000_123_3.log")
	require.NoError(t, os.Symlink(target, link))
	expired := now.Add(-2 * maxAge)
	require.NoError(t, os.Chtimes(unknown, expired, expired))

	require.NoError(t, cleanup(filepath.Dir(directory), now))

	require.FileExists(t, unknown)
	require.FileExists(t, target)
	linked, err := os.Readlink(link)
	require.NoError(t, err)
	require.Equal(t, target, linked)
}

func TestRetentionIgnoresUnmanagedDirectories(t *testing.T) {
	t.Parallel()
	now := time.Now()
	root := t.TempDir()
	unmanaged := filepath.Join(root, "unmanaged")
	expired := fixtureLog(t, unmanaged, 1, now.Add(-2*maxAge))

	require.NoError(t, cleanup(root, now))

	require.FileExists(t, expired)
}

func TestRetentionCountIsPerWorkspace(t *testing.T) {
	t.Parallel()
	state := t.TempDir()
	now := time.Now()
	w := testWriter(t, state, "/active", now)
	other := Directory(state, "/other")
	var files []string
	for i := range maxFiles + 3 {
		files = append(files, fixtureLog(t, other, i, now.Add(time.Duration(i-maxFiles-3)*time.Minute)))
	}

	require.NoError(t, cleanup(filepath.Dir(w.directory), now))

	require.Len(t, logFiles(t, other), maxFiles)
	require.Len(t, logFiles(t, w.directory), 1)
	for _, oldest := range files[:3] {
		require.NoFileExists(t, oldest)
	}
}

func TestRetentionOrdersEqualTimestampsByPath(t *testing.T) {
	t.Parallel()
	now := time.Now()
	directory := Directory(t.TempDir(), "/project")
	first := fixtureLog(t, directory, 0, now)
	for i := 1; i <= maxFiles; i++ {
		fixtureLog(t, directory, i, now)
	}

	require.NoError(t, cleanup(filepath.Dir(directory), now))

	require.NoFileExists(t, first, "lexically first path breaks ties between equally old segments")
	require.Len(t, logFiles(t, directory), maxFiles)
}
