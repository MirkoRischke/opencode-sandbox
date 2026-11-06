package sandbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"opencode-sandbox/internal/logging"
	"opencode-sandbox/internal/workspace"
)

// Stored canonical roots allow selection without reopening deleted worktrees.
type pruneScope struct {
	stateDir   string
	all        bool
	workspaces map[string]string // workspace ID -> canonical root
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
		return scope, nil
	}
	current, err := workspace.Resolve(ctx, options.Directory)
	if err != nil {
		return scope, err
	}
	scope.workspaces[current.ID()] = current.Root()
	if !options.Repository {
		return scope, nil
	}
	if _, nonGit := current.(workspace.Directory); nonGit {
		return scope, fmt.Errorf("--repo requires a Git checkout or worktree root")
	}
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
		if scope.includesWorkspace(snapshot.workspace.id) {
			plan.runtime = append(plan.runtime, pruneStep{name: "snapshots " + snapshot.workspace.group, run: snapshot.remove})
		}
	}
	data, records := scope.hostPaths()
	var err error
	plan.local, err = prunePaths(data)
	if err != nil {
		return nil, err
	}
	plan.metadata, err = prunePaths(records)
	if err != nil {
		return nil, err
	}
	return plan, nil
}

func (s pruneScope) hostPaths() (data, records []string) {
	base := filepath.Join(s.stateDir, "opencode-sandbox")
	if s.all {
		return []string{
			filepath.Join(base, "opencode"),
			filepath.Join(base, "logs"),
			filepath.Join(base, "diagnostics"),
		}, []string{filepath.Join(base, "workspaces")}
	}
	for id, root := range s.workspaces {
		data = append(data,
			filepath.Join(base, "opencode", id),
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
