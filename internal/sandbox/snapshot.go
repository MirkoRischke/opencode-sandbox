package sandbox

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"opencode-sandbox/internal/config"
	"opencode-sandbox/internal/logging"
	"opencode-sandbox/internal/workspace"

	m "github.com/superradcompany/microsandbox/sdk/go"
)

const snapshotWorktree = "/workspace"

type workspaceSnapshot struct {
	root  string
	name  string
	id    string
	group string
}

func resolveSnapshotWorkspace(ctx context.Context, directory string) (workspaceSnapshot, error) {
	w, err := workspace.Resolve(ctx, directory)
	if err != nil {
		return workspaceSnapshot{}, err
	}
	return snapshotIdentity(w.Root()), nil
}

func snapshotIdentity(root string) workspaceSnapshot {
	hash := sha256.Sum256([]byte(root))
	name := filepath.Base(root)
	return workspaceSnapshot{root: root, name: name, id: fmt.Sprintf("%x", hash),
		group: fmt.Sprintf("oc-workspace-%s-%x", namePart(name), hash[:8])}
}

func (r workspaceSnapshot) labels() map[string]string {
	return map[string]string{"opencode-sandbox": "", "kind": "workspace-baseline", "workspace-name": r.name, "workspace-id": r.id, "workspace-path": r.root}
}

func (r workspaceSnapshot) owns(labels map[string]string) bool {
	_, managed := labels["opencode-sandbox"]
	return managed && labels["kind"] == "workspace-baseline" && labels["workspace-id"] == r.id
}

// Only a lock file is stored here, never a snapshot index or head pointer.
// Serialize publication, deletion and restore so cleanup cannot race a reader.
func (r workspaceSnapshot) lock() (func(), error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	return r.lockIn(dir)
}

func (r workspaceSnapshot) lockIn(cacheDir string) (func(), error) {
	dir := filepath.Join(cacheDir, "opencode-sandbox", "snapshot-locks")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, r.id+".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close() //nolint:errcheck // Read-only lock handle; preserve the locking error.
		return nil, fmt.Errorf("another snapshot operation is in progress for %s", r.name)
	}
	return func() { _ = f.Close() }, nil //nolint:errcheck // Closing releases the lock; no buffered data to flush.
}

// CreateSnapshot always prepares from the configured image, never the old head.
// Guest files and the owned Docker disk are captured; bind mounts are not.
func CreateSnapshot(ctx context.Context, directory string, secrets []Secret, settings config.Settings, diagnostics ...DiagnosticsOptions) (reference string, err error) {
	w, err := workspace.Resolve(ctx, directory)
	if err != nil {
		return "", err
	}
	return CreateWorkspaceSnapshot(ctx, w, secrets, settings, diagnostics...)
}

// CreateWorkspaceSnapshot captures a baseline for an already resolved workspace.
func CreateWorkspaceSnapshot(ctx context.Context, w workspace.Workspace, secrets []Secret, settings config.Settings, diagnostics ...DiagnosticsOptions) (reference string, err error) {
	logging.FromContext(ctx).Info("Preparing workspace snapshot")
	diagnosticOptions, err := validatePreparation(w, settings, diagnostics)
	if err != nil {
		return "", err
	}
	repo := snapshotIdentity(w.Root())
	unlock, err := repo.lock()
	if err != nil {
		return "", err
	}
	defer unlock()
	if err := configureRuntime(); err != nil {
		return "", err
	}
	// Refuse to overwrite unrelated artifacts, even at our deterministic name.
	if _, err := repo.head(ctx); err != nil {
		return "", err
	}
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	generation := fmt.Sprintf("prepared-%x", nonce)
	prep, err := createPreparationVM(ctx, w, "oc-prepare-"+generation, secrets, settings, diagnosticOptions)
	if err != nil {
		return "", err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err = errors.Join(err, prep.Remove(cleanup))
	}()
	if err := prep.quiesceSnapshot(ctx, settings.Snapshot.Command); err != nil {
		return "", err
	}
	return repo.publish(ctx, prep.Name, generation)
}

// Publication selects the captured generation before pruning the previous head.
func (repo workspaceSnapshot) publish(ctx context.Context, preparation, generation string) (string, error) {
	snapshot, err := m.Snapshot.Create(ctx, m.SnapshotCreateOptions{
		Name: generation, Group: repo.group, FromSandbox: preparation, Labels: repo.labels(),
	})
	if err != nil {
		return "", fmt.Errorf("capture workspace snapshot: %w", err)
	}
	// Independent fresh preparations diverge; native HEAD must be selected.
	if _, err := m.Snapshot.GroupHead(ctx, repo.group+":"+generation); err != nil {
		return "", fmt.Errorf("select workspace snapshot (captured as %s): %w", snapshot.Reference(), err)
	}
	if err := repo.removeMembers(ctx, snapshot.ID()); err != nil {
		return snapshot.Reference(), fmt.Errorf("new snapshot is current, but previous snapshot cleanup failed: %w", err)
	}
	return snapshot.Reference(), nil
}

func (s *Sandbox) quiesceSnapshot(ctx context.Context, command string) error {
	if strings.TrimSpace(command) != "" {
		if err := s.Exec(ctx, nil, os.Stdout, os.Stderr, "bash", "-eu", "-c", command); err != nil {
			return fmt.Errorf("snapshot preparation: %w", err)
		}
	}
	if err := s.Exec(ctx, nil, os.Stdout, os.Stderr, "bash", "-eu", "-c", stopDockerForSnapshot); err != nil {
		return fmt.Errorf("quiesce snapshot: %w", err)
	}
	if err := s.Stop(ctx); err != nil {
		return fmt.Errorf("stop preparation VM: %w", err)
	}
	return nil
}

// The caller has validated settings, resolved paths and configured the runtime.
// Preparation always creates a disposable VM, never resumes/restores one.
func createPreparationVM(ctx context.Context, w workspace.Workspace, name string, secrets []Secret, settings config.Settings, diagnostics DiagnosticsOptions) (*Sandbox, error) {
	opts := sandboxOptions(w, snapshotWorktree, settings)
	for _, secret := range secrets {
		opts = append(opts, m.WithSecrets(secret.entry))
	}
	started := time.Now().UTC()
	vm, err := m.CreateSandbox(ctx, name, opts...)
	if err != nil {
		return nil, fmt.Errorf("create preparation VM: %w", err)
	}
	prep := &Sandbox{Name: name, Worktree: snapshotWorktree, vm: vm, logger: logging.FromContext(ctx).With("vm", name)}
	prep.log().Info("Preparation VM created")
	if settings.Network.Diagnostics {
		prep.diagnostics = startNetworkDiagnostics(ctx, vm, w.Root(), started, diagnostics)
	}
	if err := prep.configureGuest(ctx, settings); err != nil {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return nil, errors.Join(err, prep.Remove(cleanup))
	}
	if err := prep.initializeProject(ctx, secrets); err != nil {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return nil, errors.Join(err, prep.Remove(cleanup))
	}
	return prep, nil
}

// RemoveSnapshot removes this directory's members, not any restored VMs.
func RemoveSnapshot(ctx context.Context, directory string, observers ...RemovalObserver) error {
	repo, err := resolveSnapshotWorkspace(ctx, directory)
	if err != nil {
		return err
	}
	unlock, err := repo.lock()
	if err != nil {
		return err
	}
	defer unlock()
	if err := configureRuntime(); err != nil {
		return err
	}
	return repo.remove(ctx, observers...)
}

// RemoveAllSnapshots removes managed baselines across workspaces, including
// directories that no longer exist. Restored VMs remain.
func RemoveAllSnapshots(ctx context.Context, observers ...RemovalObserver) error {
	if err := configureRuntime(); err != nil {
		return err
	}
	members, err := m.Snapshot.List(ctx)
	if err != nil {
		return fmt.Errorf("list workspace snapshots: %w", err)
	}
	repositories := map[string]workspaceSnapshot{}
	for _, member := range members {
		repo, managed, err := readSnapshotWorkspace(ctx, member)
		if err != nil {
			return err
		}
		if managed {
			repositories[repo.group] = repo
		}
	}
	groups := make([]string, 0, len(repositories))
	for group := range repositories {
		groups = append(groups, group)
	}
	sort.Strings(groups)
	var errs []error
	for _, group := range groups {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		if err := repositories[group].removeLocked(ctx, observers...); err != nil {
			errs = append(errs, fmt.Errorf("remove %s: %w", group, err))
		}
	}
	return errors.Join(errs...)
}

// Use the same ownership checks for discovery and bulk deletion.
func readSnapshotWorkspace(ctx context.Context, member *m.SnapshotHandle) (workspaceSnapshot, bool, error) {
	group := member.Group()
	if group == nil || !strings.HasPrefix(*group, "oc-workspace-") || member.Scope() != m.SnapshotScopeDisk {
		return workspaceSnapshot{}, false, nil
	}
	artifact, err := member.Open(ctx)
	if err != nil {
		return workspaceSnapshot{}, false, fmt.Errorf("read snapshot %s: %w", member.Reference(), err)
	}
	repo, managed := managedSnapshotWorkspace(*group, artifact.Labels())
	return repo, managed, nil
}

func managedSnapshotWorkspace(group string, labels map[string]string) (workspaceSnapshot, bool) {
	_, managed := labels["opencode-sandbox"]
	if !strings.HasPrefix(group, "oc-workspace-") || !managed || labels["kind"] != "workspace-baseline" || labels["workspace-id"] == "" {
		return workspaceSnapshot{}, false
	}
	return workspaceSnapshot{id: labels["workspace-id"], name: labels["workspace-name"], root: labels["workspace-path"], group: group}, true
}

func (r workspaceSnapshot) removeLocked(ctx context.Context, observers ...RemovalObserver) error {
	unlock, err := r.lock()
	if err != nil {
		return err
	}
	defer unlock()
	return r.remove(ctx, observers...)
}

func (repo workspaceSnapshot) remove(ctx context.Context, observers ...RemovalObserver) error {
	head, err := repo.head(ctx)
	if err != nil {
		return err
	}
	keep := ""
	if head != nil {
		keep = head.ID()
	}
	// Native deletion refuses to remove HEAD while other members still exist.
	if err := repo.removeMembers(ctx, keep, observers...); err != nil {
		return err
	}
	if head != nil {
		size := head.SizeBytes()
		if err := head.Remove(ctx, false); err != nil {
			return fmt.Errorf("remove workspace snapshot: %w", err)
		}
		notifyRemoval(observers, head.Reference(), size)
	}
	return nil
}

func (r workspaceSnapshot) head(ctx context.Context) (*m.SnapshotHandle, error) {
	head, err := m.Snapshot.Get(ctx, r.group)
	if m.IsKind(err, m.ErrSnapshotNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find workspace snapshot: %w", err)
	}
	artifact, err := head.Open(ctx)
	if err != nil {
		return nil, err
	}
	if !r.owns(artifact.Labels()) || artifact.Scope() != m.SnapshotScopeDisk {
		return nil, fmt.Errorf("snapshot group %s is not this workspace's managed disk baseline", r.group)
	}
	return head, nil
}

func (r workspaceSnapshot) removeMembers(ctx context.Context, keepID string, observers ...RemovalObserver) error {
	members, err := m.Snapshot.List(ctx)
	if err != nil {
		return err
	}
	var targets []*m.SnapshotHandle
	for _, member := range members {
		group := member.Group()
		if group == nil || *group != r.group || member.ID() == keepID {
			continue
		}
		artifact, err := member.Open(ctx)
		if err != nil {
			return err
		}
		if !r.owns(artifact.Labels()) {
			return fmt.Errorf("refusing to delete unrelated snapshot %s", member.Reference())
		}
		targets = append(targets, member)
	}
	for _, member := range targets {
		size := member.SizeBytes()
		if err := member.Remove(ctx, true); err != nil {
			return fmt.Errorf("remove old workspace snapshot: %w", err)
		}
		notifyRemoval(observers, member.Reference(), size)
	}
	return nil
}

func connectOrRestore(ctx context.Context, name string, w workspace.Workspace, secrets []Secret, settings config.Settings, opts []m.SandboxOption) (*m.Sandbox, bool, error) {
	logger := logging.FromContext(ctx).With("vm", name)
	_, err := m.GetSandbox(ctx, name)
	if err == nil {
		logger.Info("Reusing workspace VM")
		vm, err := connectWithSecrets(ctx, name, secrets, opts)
		return vm, false, err
	}
	if !m.IsKind(err, m.ErrSandboxNotFound) {
		return nil, false, fmt.Errorf("find worktree VM: %w", err)
	}
	repo := snapshotIdentity(w.Root())
	unlock, err := repo.lock()
	if err != nil {
		return nil, false, err
	}
	defer unlock()
	// A concurrent launch may have created this VM before we took the lock.
	if _, err := m.GetSandbox(ctx, name); err == nil {
		vm, err := connectWithSecrets(ctx, name, secrets, opts)
		return vm, false, err
	} else if !m.IsKind(err, m.ErrSandboxNotFound) {
		return nil, false, fmt.Errorf("find worktree VM: %w", err)
	}
	head, err := repo.head(ctx)
	if err != nil {
		return nil, false, err
	}
	if head == nil {
		logger.Info("Creating workspace VM")
		for _, secret := range secrets {
			opts = append(opts, m.WithSecrets(secret.entry))
		}
		vm, err := m.CreateSandbox(ctx, name, opts...)
		if err != nil {
			return nil, false, fmt.Errorf("create worktree VM: %w", err)
		}
		return vm, true, nil
	}
	logger.Info("Restoring workspace VM", "snapshot", head.Reference())
	vm, err := restoreWorktree(ctx, head, name, w, secrets, settings)
	return vm, err == nil, err
}

func restoreWorktree(ctx context.Context, head *m.SnapshotHandle, name string, w workspace.Workspace, secrets []Secret, settings config.Settings) (*m.Sandbox, error) {
	policy := networkPolicy(settings.Network)
	// Restore accepts host policy only, not HTTP/TLS creation settings.
	policy.HTTP = nil
	restore := m.RestoreConfig{
		CPUs: &settings.VM.CPUs, MemoryMiB: &settings.VM.MemoryMiB, NetworkPolicy: policy,
		Volumes: workspaceMounts(w, snapshotWorktree, w.Root()),
	}
	addReadOnlyMounts(restore.Volumes, settings)
	if settings.Network.Diagnostics {
		restore.LogLevel = m.LogLevelDebug
	}
	restored, err := m.RestoreSandbox(ctx, head, name, m.WithRestoreConfig(restore))
	if err != nil {
		return nil, fmt.Errorf("restore workspace snapshot: %w", err)
	}
	defer restored.Close() //nolint:errcheck // Release the original handle on every path; the restarted handle belongs to the caller.
	handle, err := configureRestoredWorktree(ctx, restored, name, w.Root(), secrets)
	if err != nil {
		return nil, discardRestoredVM(restored, err)
	}
	restarted, err := handle.StartDetached(ctx)
	if err != nil {
		return nil, discardRestoredVM(restored, fmt.Errorf("start configured restored VM: %w", err))
	}
	return restarted, nil
}

// Failed initialization must not leave a partially configured VM for a future
// launch to resume. Cleanup outlives cancellation of the launch context.
func discardRestoredVM(restored *m.Sandbox, cause error) error {
	cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return errors.Join(cause, restored.Destroy(cleanup))
}

func configureRestoredWorktree(ctx context.Context, restored *m.Sandbox, name, root string, secrets []Secret) (*m.SandboxHandle, error) {
	// Do not execute project commands until host credentials/labels/workdir are
	// configured. Snapshot restore itself has no secret creation interface.
	if err := restored.Stop(ctx); err != nil {
		return nil, fmt.Errorf("stop restored VM: %w", err)
	}
	patch := restoredWorktreePatch(root, secrets)
	// The restore interface cannot enable TLS inspection directly. A temporary
	// non-credential secret activates the runtime's supported TLS transition;
	// removing it leaves interception enabled. Never expose this to project code.
	const tlsBootstrap = "OPENCODE_SANDBOX_TLS_BOOTSTRAP"
	if len(secrets) == 0 {
		patch.Secrets[tlsBootstrap] = m.SecretModifySpec{Value: "not-a-credential", AllowedHosts: []string{"opencode-sandbox.invalid"}}
	}
	handle, err := m.GetSandbox(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("find restored VM: %w", err)
	}
	plan, err := handle.Modify(ctx, patch)
	if err != nil {
		return nil, fmt.Errorf("configure restored VM credentials and labels: %w", err)
	}
	if !plan.Applied {
		return nil, fmt.Errorf("configure restored VM credentials and labels: modification not applied")
	}
	if len(secrets) == 0 {
		plan, err = handle.Modify(ctx, m.ModifyOptions{Policy: m.ModificationPolicyNextStart, SecretsRemove: []string{tlsBootstrap}})
		if err != nil {
			return nil, fmt.Errorf("finalize restored VM TLS inspection: %w", err)
		}
		if !plan.Applied {
			return nil, fmt.Errorf("finalize restored VM TLS inspection: modification not applied")
		}
	}
	return handle, nil
}

func restoredWorktreePatch(root string, secrets []Secret) m.ModifyOptions {
	patch := m.ModifyOptions{
		Policy: m.ModificationPolicyNextStart, Workdir: root,
		Labels:  map[string]string{"opencode-sandbox": ""},
		Secrets: make(map[string]m.SecretModifySpec, len(secrets)),
	}
	for _, secret := range secrets {
		entry := secret.entry
		placeholder := entry.Placeholder
		if placeholder == "" {
			placeholder = "$MSB_" + entry.EnvVar
		}
		// v0.7.8 Modify cannot express HeaderFields. Substitution is therefore
		// all-header on the same allowed hosts; this accepted limitation is
		// documented in docs/config.md, not silently treated as equivalent.
		patch.Secrets[entry.EnvVar] = m.SecretModifySpec{Value: entry.Value, Placeholder: placeholder, AllowedHosts: entry.Allow}
	}
	return patch
}

const stopDockerForSnapshot = `
if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
  containers=$(docker ps -q)
  if [ -n "$containers" ]; then docker stop $containers; fi
  pid=$(cat /var/run/docker.pid)
  kill -TERM "$pid"
  deadline=$((SECONDS + 60))
  while kill -0 "$pid" 2>/dev/null; do
    if (( SECONDS >= deadline )); then echo 'Docker did not stop cleanly' >&2; exit 1; fi
    sleep 0.2
  done
fi
sync
`
