package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"opencode-sandbox/internal/logging"
	"opencode-sandbox/internal/workspace"
)

// Stored canonical roots allow selection without reopening deleted worktrees.
type pruneScope struct {
	stateDir     string
	all          bool
	repositoryID string            // Shared baselines are only selected by --repo or --all.
	workspaces   map[string]string // workspace ID -> canonical root
	stateScope   string            // A workspace directory or the whole repository's state.
}

func resolvePruneScope(ctx context.Context, options PruneOptions) (pruneScope, error) {
	scope := pruneScope{
		stateDir:   options.StateDir,
		all:        options.All,
		workspaces: make(map[string]string),
	}
	if options.All && options.Repository {
		return scope, fmt.Errorf("--all and --repo are mutually exclusive")
	}
	if options.All {
		scope.stateScope = stateRoot(options.StateDir)
		return scope, nil
	}
	current, err := workspace.Resolve(ctx, options.Directory)
	if err != nil {
		return scope, err
	}
	scope.workspaces[current.ID()] = current.Root()
	if !options.Repository {
		scope.stateScope = workspaceStatePath(options.StateDir, current.RepositoryID(), current.ID())
		return scope, nil
	}
	if _, nonGit := current.(workspace.Directory); nonGit {
		return scope, fmt.Errorf("--repo requires a Git checkout or worktree root")
	}
	scope.repositoryID = current.RepositoryID()
	scope.stateScope = filepath.Join(stateRoot(options.StateDir), scope.repositoryID)
	records, err := workspace.Records(options.StateDir)
	if err != nil {
		return scope, err
	}
	for _, record := range records {
		if record.RepositoryID == current.RepositoryID() {
			scope.workspaces[record.ID()] = record.Root
		}
	}
	return scope, nil
}

func (s pruneScope) includesWorkspace(id string) bool {
	_, selected := s.workspaces[id]
	return s.all || selected
}

func (s pruneScope) includesVM(vm pruneVM) bool {
	if s.includesWorkspace(vm.workspaceID) {
		return true
	}
	// Workspace VMs have deterministic names, even without ownership labels.
	for _, root := range s.workspaces {
		identity := snapshotIdentity(root)
		name := fmt.Sprintf("oc-%s-%s", namePart(identity.name), identity.id[:8])
		if vm.name == name {
			return true
		}
	}
	return false
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
	data, records := scope.hostPaths()
	var err error
	plan.local, err = prunePaths(data)
	if err != nil {
		return nil, err
	}
	if scope.stateScope != "" {
		step, err := scope.statePruneStep()
		if err != nil {
			return nil, err
		}
		if step != nil {
			plan.local = append(plan.local, *step)
		}
	}
	plan.metadata, err = prunePaths(records)
	if err != nil {
		return nil, err
	}
	return plan, nil
}

func (s pruneScope) statePruneStep() (*pruneStep, error) {
	err := s.withStateParent(func(root *os.Root, name string) error {
		_, err := root.Lstat(name)
		return err
	})
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect prune target %q: %w", s.stateScope, err)
	}
	return &pruneStep{name: "host data " + s.stateScope, run: func(context.Context) error {
		err := s.withStateParent(func(root *os.Root, name string) error {
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
func (s pruneScope) withStateParent(use func(*os.Root, string) error) error {
	rel, err := filepath.Rel(s.stateDir, s.stateScope)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(s.stateDir)
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

func (s pruneScope) hostPaths() (data, records []string) {
	base := filepath.Join(s.stateDir, "opencode-sandbox")
	if s.all {
		return []string{
			filepath.Join(base, "logs"),
			filepath.Join(base, "diagnostics"),
		}, []string{filepath.Join(base, "workspaces")}
	}
	for id, root := range s.workspaces {
		data = append(data,
			logging.Directory(s.stateDir, root),
			filepath.Dir(diagnosticsPath(s.stateDir, root)),
		)
		records = append(records, workspace.RecordPath(s.stateDir, id))
	}
	sort.Strings(data)
	sort.Strings(records)
	return data, records
}

func prunePaths(paths []string) ([]pruneStep, error) {
	var steps []pruneStep
	for _, path := range paths {
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, fmt.Errorf("inspect prune target %q: %w", path, err)
		}
		steps = append(steps, pruneStep{
			name: "host data " + path,
			run:  func(context.Context) error { return os.RemoveAll(path) },
		})
	}
	return steps, nil
}
