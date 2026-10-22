package logging

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

type segment struct {
	path     string
	modified time.Time
}

// cleanup is called with the root cleanup lock held. Unknown files, symlinks,
// and active segments are never removed. Limits are best-effort with active writers.
func cleanup(root string, now time.Time) error {
	dirs, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		if !dir.IsDir() || !managedDirectory.MatchString(dir.Name()) {
			continue
		}
		if err := cleanupDirectory(filepath.Join(root, dir.Name()), now); err != nil {
			return err
		}
	}
	return nil
}

func cleanupDirectory(directory string, now time.Time) error {
	files, err := managedSegments(directory)
	if err != nil {
		return err
	}
	remaining := len(files)
	for _, file := range files {
		if remaining <= maxFiles && !file.modified.Before(now.Add(-maxAge)) {
			continue
		}
		removed, err := removeInactiveSegment(file.path)
		if err != nil {
			return err
		}
		if removed {
			remaining--
		}
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return os.Remove(directory)
	}
	return nil
}

// managedSegments returns only regular managed files, oldest first. Path order
// breaks timestamp ties so retention is deterministic across directory listings.
func managedSegments(directory string) ([]segment, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	var files []segment
	for _, entry := range entries {
		if !managedFile.MatchString(entry.Name()) || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		if info.Mode().IsRegular() {
			files = append(files, segment{filepath.Join(directory, entry.Name()), info.ModTime()})
		}
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].modified.Equal(files[j].modified) {
			return strings.Compare(files[i].path, files[j].path) < 0
		}
		return files[i].modified.Before(files[j].modified)
	})
	return files, nil
}

// A busy segment belongs to an active writer; skipping it is not an error.
// Keep the lock until removal finishes, then close the handle on every path.
func removeInactiveSegment(path string) (bool, error) {
	file, err := os.OpenFile(path, os.O_RDWR|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return false, err
	}
	lockErr := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if lockErr == nil {
		err = os.Remove(path)
	}
	closeErr := file.Close()
	if err != nil {
		return false, err
	}
	if closeErr != nil {
		return false, closeErr
	}
	if lockErr == syscall.EWOULDBLOCK || lockErr == syscall.EAGAIN {
		return false, nil
	}
	return lockErr == nil, lockErr
}
