// Package logging owns private host-side launcher logs, not guest output.
package logging

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	maxBytes = 10 << 20
	maxFiles = 20
	maxAge   = 30 * 24 * time.Hour
)

type contextKey struct{}

var discard = slog.New(slog.NewJSONHandler(io.Discard, nil))

// WithLogger keeps concurrent invocations independent; no process-global logger.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, logger)
}

func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(contextKey{}).(*slog.Logger); ok && logger != nil {
		return logger
	}
	return discard
}

var unsafeName = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
var managedDirectory = regexp.MustCompile(`^[a-zA-Z0-9._-]+-[a-f0-9]{16}$`)
var managedFile = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}\.\d{9}[+-]\d{4}_\d+_\d+\.log$`)

// Directory is pure: workspaceRoot must already be canonical, as at VM launch.
func Directory(stateDir, workspaceRoot string) string {
	name := unsafeName.ReplaceAllString(filepath.Base(workspaceRoot), "_")
	if len(name) > 64 {
		name = name[:64]
	}
	hash := sha256.Sum256([]byte(workspaceRoot))
	return filepath.Join(stateDir, "opencode-sandbox", "logs", fmt.Sprintf("%s-%x", name, hash[:8]))
}

// Open starts a locked segment and prunes inactive logs across workspaces.
// Each JSON record is written immediately; there is no userspace buffer.
func Open(stateDir, workspaceRoot string, warnings io.Writer) (*slog.Logger, io.Closer, error) {
	if warnings == nil {
		warnings = io.Discard
	}
	w := &writer{directory: Directory(stateDir, workspaceRoot), warnings: warnings, now: time.Now, limit: maxBytes}
	if err := privateDirectory(filepath.Dir(w.directory)); err != nil {
		return nil, nil, err
	}
	if err := w.rotate(w.now()); err != nil {
		return nil, nil, err
	}
	return slog.New(slog.NewJSONHandler(w, nil)).With("workspace", workspaceRoot, "pid", os.Getpid()), w, nil
}

type writer struct {
	mu        sync.Mutex
	directory string
	warnings  io.Writer
	now       func() time.Time
	limit     int64
	file      *os.File
	day       string
	size      int64
	closed    bool
	warned    bool
}

func (w *writer) warn(err error) {
	if !w.warned {
		fmt.Fprintln(w.warnings, "warning: launcher logging failed:", err) //nolint:errcheck // Logging must never prevent launcher use.
		w.warned = true
	}
}

func (w *writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, os.ErrClosed
	}
	now := w.now()
	if now.Format("2006-01-02") != w.day || (w.size > 0 && w.size+int64(len(p)) > w.limit) {
		if err := w.rotate(now); err != nil {
			w.warn(err)
			return len(p), nil
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	if err != nil {
		w.warn(err)
	}
	return n, err
}

func (w *writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	err := w.file.Close() // Closing releases the segment lock, including on process exit.
	if err != nil {
		w.warn(err)
	}
	return err
}

func privateDirectory(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("log directory %q is not a directory", path)
	}
	return os.Chmod(path, 0700)
}

func (w *writer) rotate(now time.Time) error {
	root := filepath.Dir(w.directory)
	lock, err := os.OpenFile(filepath.Join(root, ".cleanup.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close() //nolint:errcheck // Closing releases the cleanup lock.
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	// Creation and locking happen under the cleanup lock, so another process
	// cannot prune a newly created segment before its writer locks it.
	if err := privateDirectory(w.directory); err != nil {
		return err
	}
	prefix := fmt.Sprintf("%s_%d_*.log", now.Format("2006-01-02T15-04-05.000000000-0700"), os.Getpid())
	file, err := os.CreateTemp(w.directory, prefix)
	if err != nil {
		return err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		_ = file.Close() //nolint:errcheck // Preserve the locking error.
		return err
	}
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			w.warn(err)
		}
	}
	w.file, w.day, w.size = file, now.Format("2006-01-02"), 0
	if err := cleanup(root, now); err != nil {
		w.warn(err)
	}
	return nil
}

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
		directory := filepath.Join(root, dir.Name())
		entries, err := os.ReadDir(directory)
		if err != nil {
			return err
		}
		var files []segment
		for _, entry := range entries {
			if !managedFile.MatchString(entry.Name()) || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				return err
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
		remaining := len(files)
		for _, f := range files {
			if remaining <= maxFiles && !f.modified.Before(now.Add(-maxAge)) {
				continue
			}
			file, err := os.OpenFile(f.path, os.O_RDWR|syscall.O_NOFOLLOW, 0)
			if err != nil {
				return err
			}
			lockErr := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
			if lockErr == nil {
				err = os.Remove(f.path)
				if err == nil {
					remaining--
				}
			}
			closeErr := file.Close()
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}
			if lockErr != nil && lockErr != syscall.EWOULDBLOCK && lockErr != syscall.EAGAIN {
				return lockErr
			}
		}
		entries, err = os.ReadDir(directory)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			if err := os.Remove(directory); err != nil {
				return err
			}
		}
	}
	return nil
}
