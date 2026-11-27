package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"opencode-sandbox/internal/logging"
	"opencode-sandbox/internal/workspace"

	m "github.com/superradcompany/microsandbox/sdk/go"
)

// OrphanedVM describes a stopped managed VM whose linked worktree is gone.
type OrphanedVM struct {
	Name, WorktreePath string
}

// OrphanedVMs previews the stopped linked-worktree VMs eligible for removal.
func OrphanedVMs(ctx context.Context, directory string, all bool) ([]OrphanedVM, error) {
	candidates, repositoryID, err := orphanSearch(ctx, directory, all)
	if err != nil {
		return nil, err
	}
	var result []OrphanedVM
	for _, vm := range candidates {
		if path := orphanedWorktree(ctx, vm, repositoryID); path != "" {
			result = append(result, OrphanedVM{Name: vm.Name(), WorktreePath: path})
		}
	}
	return result, ctx.Err()
}

// RemoveOrphaned deletes only stopped VMs for absent, unregistered linked worktrees.
func RemoveOrphaned(ctx context.Context, directory string, all bool, observers ...RemovalObserver) error {
	candidates, repositoryID, err := orphanSearch(ctx, directory, all)
	if err != nil {
		return err
	}
	var errs []error
	for _, vm := range candidates {
		if ctx.Err() != nil {
			break
		}
		if orphanedWorktree(ctx, vm, repositoryID) == "" {
			continue
		}
		current, err := vm.Refresh(ctx)
		if err != nil {
			logging.FromContext(ctx).Warn("Could not recheck VM status", "vm", vm.Name(), "error", err)
			continue
		}
		if current.Status() != m.SandboxStatusStopped {
			continue
		}
		// Remove rechecks the exact VM's lifecycle. Destroy would stop a VM
		// that was started after the inventory, even without force.
		if err := current.Remove(ctx); err != nil {
			if m.IsKind(err, m.ErrSandboxStillRunning) || m.IsKind(err, m.ErrSandboxNotFound) || m.IsKind(err, m.ErrSandboxReplaced) {
				continue
			}
			errs = append(errs, fmt.Errorf("remove %s: %w", vm.Name(), err))
			continue
		}
		notifyRemoval(observers, vm.Name(), nil)
	}
	return errors.Join(append(errs, ctx.Err())...)
}

func orphanSearch(ctx context.Context, directory string, all bool) ([]*m.SandboxHandle, string, error) {
	var repositoryID string
	labels := map[string]string{"opencode-sandbox": "", "kind": "workspace-vm"}
	if !all {
		var err error
		repositoryID, err = workspace.CurrentGitRepositoryID(ctx, directory)
		if err != nil {
			return nil, "", fmt.Errorf("--orphaned requires a Git repository unless --all is used: %w", err)
		}
		labels["repository-id"] = repositoryID
	}
	vms, err := listVMs(ctx, labels)
	return vms, repositoryID, err
}

func orphanedWorktree(ctx context.Context, vm *m.SandboxHandle, repositoryID string) string {
	if vm.Status() != m.SandboxStatusStopped {
		return ""
	}
	config, err := vm.Config()
	if err != nil {
		logging.FromContext(ctx).Warn("Could not check VM labels", "vm", vm.Name(), "error", err)
		return ""
	}
	path, err := missingLinkedWorktree(ctx, config.Labels, repositoryID)
	if err != nil {
		logging.FromContext(ctx).Warn("Could not check former worktree", "vm", vm.Name(), "error", err)
	}
	return path
}

func missingLinkedWorktree(ctx context.Context, labels map[string]string, repositoryID string) (string, error) {
	path, commonDir, owner := labels["workspace-path"], labels["git-common-dir"], labels["repository-id"]
	if labels["kind"] != "workspace-vm" || !filepath.IsAbs(path) || !filepath.IsAbs(commonDir) ||
		owner != workspace.GitRepositoryID(commonDir) || (repositoryID != "" && owner != repositoryID) {
		return "", nil
	}
	if missing, err := missingWorktreePath(path); err != nil || !missing {
		return "", err
	}
	paths, err := workspace.ListedWorktrees(ctx, commonDir)
	if err != nil {
		return "", err
	}
	if paths[filepath.Clean(path)] {
		return "", nil
	}
	// A worktree may have been created while Git was listing its registry.
	if missing, err := missingWorktreePath(path); err != nil || !missing {
		return "", err
	}
	return path, nil
}

func missingWorktreePath(path string) (bool, error) {
	_, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect worktree path %q: %w", path, err)
	}
	return false, nil
}
