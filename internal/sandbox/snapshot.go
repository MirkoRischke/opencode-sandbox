package sandbox

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"opencode-sandbox/internal/config"

	m "github.com/superradcompany/microsandbox/sdk/go"
)

const snapshotWorktree = "/workspace"

type repoSnapshot struct {
	common string
	name   string
	id     string
	group  string
}

func snapshotRepo(ctx context.Context, directory string) (repoSnapshot, error) {
	common, err := commonGitDir(ctx, directory)
	if err != nil {
		return repoSnapshot{}, err
	}
	return snapshotIdentity(common), nil
}

func snapshotIdentity(common string) repoSnapshot {
	hash := sha256.Sum256([]byte(common))
	name := filepath.Base(filepath.Dir(common))
	return repoSnapshot{common: common, name: name, id: fmt.Sprintf("%x", hash),
		group: fmt.Sprintf("oc-repo-%s-%x", namePart(name), hash[:8])}
}

func (r repoSnapshot) labels() map[string]string {
	return map[string]string{"opencode-sandbox": "", "kind": "repo-baseline", "repo-name": r.name, "repo-id": r.id, "repo-path": filepath.Dir(r.common)}
}

func (r repoSnapshot) owns(labels map[string]string) bool {
	_, managed := labels["opencode-sandbox"]
	return managed && labels["kind"] == "repo-baseline" && labels["repo-id"] == r.id
}

// Only a lock file is stored here, never a snapshot index or head pointer.
// Serialize publication, deletion and restore so cleanup cannot race a reader.
func (r repoSnapshot) lock() (func(), error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	return r.lockIn(dir)
}

func (r repoSnapshot) lockIn(cacheDir string) (func(), error) {
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
	if err := settings.Validate(); err != nil {
		return "", err
	}
	if strings.TrimSpace(settings.Snapshot.Command) == "" {
		return "", fmt.Errorf("snapshot.command is required to create a repository baseline")
	}
	diagnosticOptions, err := resolveDiagnostics(settings.Network.Diagnostics, diagnostics)
	if err != nil {
		return "", err
	}
	paths, err := discoverWorktree(ctx, directory)
	if err != nil {
		return "", err
	}
	repo := snapshotIdentity(paths.common)
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
	prep, err := createPreparationVM(ctx, paths, "oc-prepare-"+generation, secrets, settings, diagnosticOptions)
	if err != nil {
		return "", err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err = errors.Join(err, prep.Remove(cleanup))
	}()
	if err := prep.Exec(ctx, nil, os.Stdout, os.Stderr, "bash", "-eu", "-c", settings.Snapshot.Command); err != nil {
		return "", fmt.Errorf("snapshot preparation: %w", err)
	}
	if err := prep.Exec(ctx, nil, os.Stdout, os.Stderr, "bash", "-eu", "-c", stopDockerForSnapshot); err != nil {
		return "", fmt.Errorf("quiesce snapshot: %w", err)
	}
	if err := prep.Stop(ctx); err != nil {
		return "", fmt.Errorf("stop preparation VM: %w", err)
	}
	snapshot, err := m.Snapshot.Create(ctx, m.SnapshotCreateOptions{
		Name: generation, Group: repo.group, FromSandbox: prep.Name, Labels: repo.labels(),
	})
	if err != nil {
		return "", fmt.Errorf("capture repository snapshot: %w", err)
	}
	// Independent fresh preparations diverge; native HEAD must be selected.
	if _, err := m.Snapshot.GroupHead(ctx, repo.group+":"+generation); err != nil {
		return "", fmt.Errorf("select repository snapshot (captured as %s): %w", snapshot.Reference(), err)
	}
	if err := repo.removeMembers(ctx, snapshot.ID()); err != nil {
		return snapshot.Reference(), fmt.Errorf("new snapshot is current, but previous snapshot cleanup failed: %w", err)
	}
	return snapshot.Reference(), nil
}

// The caller has validated settings, resolved paths and configured the runtime.
// Preparation always creates a disposable VM, never resumes/restores one.
func createPreparationVM(ctx context.Context, paths worktreePaths, name string, secrets []Secret, settings config.Settings, diagnostics DiagnosticsOptions) (*Sandbox, error) {
	opts := sandboxOptions(paths, snapshotWorktree, settings)
	for _, secret := range secrets {
		opts = append(opts, m.WithSecrets(secret.entry))
	}
	started := time.Now().UTC()
	vm, err := m.CreateSandbox(ctx, name, opts...)
	if err != nil {
		return nil, fmt.Errorf("create preparation VM (runtime details suppressed)")
	}
	prep := &Sandbox{Name: name, Worktree: snapshotWorktree, vm: vm}
	if settings.Network.Diagnostics {
		prep.diagnostics = startNetworkDiagnostics(vm, paths.root, started, diagnostics)
	}
	if err := prep.initializeProject(ctx, secrets); err != nil {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return nil, errors.Join(err, prep.Remove(cleanup))
	}
	return prep, nil
}

// RemoveSnapshot removes this repository's members, not any restored VMs.
func RemoveSnapshot(ctx context.Context, directory string) error {
	repo, err := snapshotRepo(ctx, directory)
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
	head, err := repo.head(ctx)
	if err != nil {
		return err
	}
	keep := ""
	if head != nil {
		keep = head.ID()
	}
	// Native deletion refuses to remove HEAD while other members still exist.
	if err := repo.removeMembers(ctx, keep); err != nil {
		return err
	}
	if head != nil {
		if err := head.Remove(ctx, false); err != nil {
			return fmt.Errorf("remove repository snapshot: %w", err)
		}
	}
	return nil
}

func (r repoSnapshot) head(ctx context.Context) (*m.SnapshotHandle, error) {
	head, err := m.Snapshot.Get(ctx, r.group)
	if m.IsKind(err, m.ErrSnapshotNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find repository snapshot: %w", err)
	}
	artifact, err := head.Open(ctx)
	if err != nil {
		return nil, err
	}
	if !r.owns(artifact.Labels()) || artifact.Scope() != m.SnapshotScopeDisk {
		return nil, fmt.Errorf("snapshot group %s is not this repository's managed disk baseline", r.group)
	}
	return head, nil
}

func (r repoSnapshot) removeMembers(ctx context.Context, keepID string) error {
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
		if err := member.Remove(ctx, true); err != nil {
			return fmt.Errorf("remove old repository snapshot: %w", err)
		}
	}
	return nil
}

func connectOrRestore(ctx context.Context, name string, paths worktreePaths, secrets []Secret, settings config.Settings, opts []m.SandboxOption) (*m.Sandbox, error) {
	_, err := m.GetSandbox(ctx, name)
	if err == nil {
		return connectWithSecrets(ctx, name, secrets, opts)
	}
	if !m.IsKind(err, m.ErrSandboxNotFound) {
		return nil, fmt.Errorf("find worktree VM (runtime details suppressed)")
	}
	repo := snapshotIdentity(paths.common)
	unlock, err := repo.lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	// A concurrent launch may have created this VM before we took the lock.
	if _, err := m.GetSandbox(ctx, name); err == nil {
		return connectWithSecrets(ctx, name, secrets, opts)
	} else if !m.IsKind(err, m.ErrSandboxNotFound) {
		return nil, fmt.Errorf("find worktree VM (runtime details suppressed)")
	}
	head, err := repo.head(ctx)
	if err != nil {
		return nil, err
	}
	if head == nil {
		return connectWithSecrets(ctx, name, secrets, opts)
	}
	return restoreWorktree(ctx, head, name, paths, secrets, settings)
}

func restoreWorktree(ctx context.Context, head *m.SnapshotHandle, name string, paths worktreePaths, secrets []Secret, settings config.Settings) (*m.Sandbox, error) {
	owner := m.MountOptions{Owner: &m.MountOwner{UID: 0, GID: 0}}
	policy := networkPolicy(settings.Network)
	// Restore accepts host policy only, not HTTP/TLS creation settings.
	policy.HTTP = nil
	restore := m.RestoreConfig{
		CPUs: &settings.VM.CPUs, MemoryMiB: &settings.VM.MemoryMiB, NetworkPolicy: policy,
		Volumes: map[string]m.MountConfig{
			snapshotWorktree: m.Mount.Bind(paths.root, owner),
			paths.root:       m.Mount.Bind(paths.root, owner),
			paths.common:     m.Mount.Bind(paths.common, owner),
		},
	}
	if settings.Network.Diagnostics {
		restore.LogLevel = m.LogLevelDebug
	}
	restored, err := m.RestoreSandbox(ctx, head, name, m.WithRestoreConfig(restore))
	if err != nil {
		return nil, fmt.Errorf("restore repository snapshot (runtime details suppressed)")
	}
	defer restored.Close() //nolint:errcheck // Release the original handle on every path; the restarted handle belongs to the caller.
	handle, err := configureRestoredWorktree(ctx, restored, name, paths.root, secrets)
	if err != nil {
		return nil, discardRestoredVM(restored, err)
	}
	restarted, err := handle.StartDetached(ctx)
	if err != nil {
		return nil, discardRestoredVM(restored, fmt.Errorf("start configured restored VM (runtime details suppressed)"))
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
		return nil, fmt.Errorf("find restored VM (runtime details suppressed)")
	}
	plan, err := handle.Modify(ctx, patch)
	if err != nil || !plan.Applied {
		return nil, fmt.Errorf("configure restored VM credentials and labels (runtime details suppressed)")
	}
	if len(secrets) == 0 {
		plan, err = handle.Modify(ctx, m.ModifyOptions{Policy: m.ModificationPolicyNextStart, SecretsRemove: []string{tlsBootstrap}})
		if err != nil || !plan.Applied {
			return nil, fmt.Errorf("finalize restored VM TLS inspection (runtime details suppressed)")
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
