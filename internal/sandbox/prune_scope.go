package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"opencode-sandbox/internal/logging"
	"opencode-sandbox/internal/workspace"
)

type pruneScope struct {
	stateDir     string
	all          bool
	repositoryID string // Shared baselines are only selected by --repo or --all.
	current      workspace.Workspace
	hostPaths    []string
}

func resolvePruneScope(ctx context.Context, options PruneOptions) (pruneScope, error) {
	scope := pruneScope{
		stateDir: options.StateDir,
		all:      options.All,
	}
	base := filepath.Join(options.StateDir, "opencode-sandbox")
	if options.All && options.Repository {
		return scope, fmt.Errorf("--all and --repo are mutually exclusive")
	}
	if options.All {
		scope.hostPaths = []string{filepath.Join(base, "logs"), filepath.Join(base, "diagnostics"), StateRoot(options.StateDir)}
		return scope, nil
	}
	current, err := workspace.Resolve(ctx, options.Directory)
	if err != nil {
		return scope, err
	}
	scope.current = current
	if !options.Repository {
		scope.hostPaths = []string{
			logging.Directory(options.StateDir, current.Root(), current.RepositoryID()),
			filepath.Dir(diagnosticsPath(options.StateDir, current.Root(), current.RepositoryID())),
			WorkspaceStatePath(options.StateDir, current),
		}
		return scope, nil
	}
	if _, nonGit := current.(workspace.Directory); nonGit {
		return scope, fmt.Errorf("--repo requires a Git checkout or worktree root")
	}
	scope.repositoryID = current.RepositoryID()
	scope.hostPaths = []string{
		logging.RepositoryDirectory(options.StateDir, scope.repositoryID),
		repositoryDiagnosticsDirectory(options.StateDir, scope.repositoryID),
		RepositoryStatePath(options.StateDir, current),
	}
	return scope, nil
}

func (s pruneScope) includesVM(vm pruneVM) bool {
	return s.all ||
		(s.repositoryID != "" && s.repositoryID == vm.repositoryID) ||
		vm.workspaceID == s.current.ID() ||
		vm.name == Name(s.current)
}

func planWorkspacePrune(scope pruneScope, inventory pruneResources) (*PrunePlan, error) {
	plan := &PrunePlan{}
	for _, vm := range inventory.vms {
		if scope.includesVM(vm) {
			plan.runtime = append(plan.runtime, pruneStep{name: "VM " + vm.name, run: vm.remove})
		}
	}
	for _, snapshot := range inventory.snapshots {
		if scope.all || (scope.repositoryID != "" && snapshot.workspace.id == scope.repositoryID) {
			plan.runtime = append(plan.runtime, pruneStep{name: "snapshots " + snapshot.workspace.group, run: snapshot.remove})
		}
	}
	for _, path := range scope.hostPaths {
		step, err := hostPruneStep(scope.stateDir, path)
		if err != nil {
			return nil, err
		}
		if step != nil {
			plan.local = append(plan.local, *step)
		}
	}
	return plan, nil
}

func hostPruneStep(stateDir, path string) (*pruneStep, error) {
	err := withStateParent(stateDir, path, func(root *os.Root, name string) error {
		_, err := root.Lstat(name)
		return err
	})
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect prune target %q: %w", path, err)
	}
	return &pruneStep{name: "host data " + path, run: func(context.Context) error {
		err := withStateParent(stateDir, path, func(root *os.Root, name string) error {
			return root.RemoveAll(name)
		})
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}}, nil
}

// Open each parent inside the preceding directory. Recheck at deletion time:
// the user may have taken time to confirm the preview.
func withStateParent(stateDir, path string, use func(*os.Root, string) error) error {
	rel, err := filepath.Rel(stateDir, path)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(stateDir)
	if err != nil {
		return err
	}
	defer root.Close() //nolint:errcheck // Read-only directory handle.
	return withStateChild(root, rel, use)
}

func withStateChild(root *os.Root, path string, use func(*os.Root, string) error) error {
	name, rest, found := strings.Cut(path, string(filepath.Separator))
	if !found {
		return use(root, name)
	}
	info, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("managed state path %q is not a directory or is a symbolic link", filepath.Join(root.Name(), name))
	}
	child, err := root.OpenRoot(name)
	if err != nil {
		return err
	}
	defer child.Close() //nolint:errcheck // Read-only directory handle.
	return withStateChild(child, rest, use)
}
