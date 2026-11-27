package sandbox

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"opencode-sandbox/internal/config"
	"opencode-sandbox/internal/workspace"

	m "github.com/superradcompany/microsandbox/sdk/go"
)

// StateRoot is the top-level directory for persistent workspace state.
func StateRoot(stateDir string) string {
	return filepath.Join(stateDir, "opencode-sandbox", "state")
}

// RepositoryStatePath groups persistent state for all worktrees of a repository.
func RepositoryStatePath(stateDir string, w workspace.Workspace) string {
	return filepath.Join(StateRoot(stateDir), namePart(w.RepositoryName())+"--"+w.RepositoryID())
}

// WorkspaceStatePath is the persistent state directory for a single checkout.
func WorkspaceStatePath(stateDir string, w workspace.Workspace) string {
	return filepath.Join(RepositoryStatePath(stateDir, w), namePart(filepath.Base(w.Root()))+"--"+w.ID())
}

func hostStateDirectory() (string, error) {
	stateDir := os.Getenv("XDG_STATE_HOME")
	if stateDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("locate host state directory: %w", err)
		}
		stateDir = filepath.Join(home, ".local", "state")
	}
	return filepath.Abs(stateDir)
}

func validateStateLocation(w workspace.Workspace, stateDir string) error {
	root, err := resolveStateLocation(StateRoot(stateDir))
	if err != nil {
		return fmt.Errorf("resolve managed state location: %w", err)
	}
	for _, mount := range w.Mounts(snapshotWorktree) {
		if config.MountTargetsOverlap(root, mount.Source) {
			return fmt.Errorf("managed state directory %q overlaps workspace mount source %q", root, mount.Source)
		}
	}
	return nil
}

// Resolve existing ancestors even when the managed directory doesn't exist yet.
func resolveStateLocation(dir string) (string, error) {
	_, err := os.Lstat(dir)
	if err == nil {
		return filepath.EvalSymlinks(dir)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	parent, err := resolveStateLocation(filepath.Dir(dir))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(dir)), nil
}

// Only workspace VMs mount live state. Baselines must never access or capture it.
func addStateMounts(mounts map[string]m.MountConfig, w workspace.Workspace, entries []config.StateSettings, stateDir string) error {
	if len(entries) == 0 {
		return nil
	}
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return fmt.Errorf("create host state directory: %w", err)
	}
	root := StateRoot(stateDir)
	workspaceDir := WorkspaceStatePath(stateDir, w)
	for _, dir := range []string{filepath.Dir(root), root, filepath.Dir(workspaceDir), workspaceDir} {
		if err := ensureStateDirectory(dir); err != nil {
			return err
		}
	}
	for _, entry := range entries {
		source := filepath.Join(workspaceDir, entry.Name)
		if err := ensureStateDirectory(source); err != nil {
			return err
		}
		mounts[entry.Target] = m.Mount.Bind(source, m.MountOptions{
			Owner: &m.MountOwner{UID: 0, GID: 0},
		})
	}
	return nil
}

func ensureStateDirectory(dir string) error {
	if err := os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
		return fmt.Errorf("create managed state directory: %w", err)
	}
	return checkStateDirectory(dir)
}

func checkStateDirectory(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("inspect managed state directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("managed state path %q is not a directory or is a symbolic link", dir)
	}
	return nil
}
