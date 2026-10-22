package sandbox_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"opencode-sandbox/internal/config"
	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/require"
	m "github.com/superradcompany/microsandbox/sdk/go"
)

func TestRepositorySnapshotWithoutPreparationCommand(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	worktree := newWorktree(t)
	settings := config.DefaultSettings()
	settings.VM.CPUs, settings.VM.MemoryMiB = 1, 768
	settings.VM.RootDiskMiB, settings.VM.DockerDiskMiB = 512, 256
	source := filepath.Join(t.TempDir(), "instructions.md")
	require.NoError(t, os.WriteFile(source, []byte("before snapshot"), 0600))
	settings.Files = []config.FileSettings{{Source: source, Target: "/opt/instructions.md"}}
	t.Cleanup(func() {
		cleanup, done := context.WithTimeout(context.Background(), time.Minute)
		defer done()
		require.NoError(t, sandbox.RemoveSnapshot(cleanup, worktree))
	})
	reference, err := sandbox.CreateSnapshot(ctx, worktree, nil, settings)
	require.NoError(t, err)
	require.NotEmpty(t, reference)
	require.NoError(t, os.WriteFile(source, []byte("current host contents"), 0600))
	skills := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(skills, "SKILL.md"), []byte("current skill"), 0600))
	settings.Files = append(settings.Files, config.FileSettings{Source: skills, Target: "/opt/skills"})
	settings.Network.AllowedHosts = []string{"example.com"}
	settings.Network.AdditionalAllowedHosts = nil
	child := prepareSnapshotChild(t, ctx, worktree, settings)
	instructions := execInSandbox(t, child, "cat", guestInstructions)
	require.Contains(t, instructions, `"/opt/instructions.md"`)
	require.Contains(t, instructions, `"/opt/skills"`)
	require.Contains(t, instructions, "- example.com")
	require.NotContains(t, instructions, "- github.com")
	execInSandbox(t, child, "docker", "info")
	require.Equal(t, "current host contents", execInSandbox(t, child, "cat", "/opt/instructions.md"))
	require.Equal(t, "current skill", execInSandbox(t, child, "cat", "/opt/skills/SKILL.md"))
	execInSandbox(t, child, "bash", "-eu", "-c", `if printf changed > /opt/instructions.md; then exit 1; fi; if touch /opt/skills/new; then exit 1; fi`)
}

func TestSnapshotListingIncludesBaselineMetadataAndSeparatesWorkspaces(t *testing.T) {
	t.Parallel()
	f := newSnapshotFixture(t, "true")
	head, err := m.Snapshot.Get(f.ctx, f.reference)
	require.NoError(t, err)
	artifact, err := head.Open(f.ctx)
	require.NoError(t, err)
	require.Equal(t, "worktree", artifact.Labels()["workspace-name"])
	require.Equal(t, m.SnapshotScopeDisk, artifact.Scope())

	listing, err := sandbox.ListSnapshots(f.ctx, f.worktree)
	require.NoError(t, err)
	require.NotNil(t, listing.Current)
	require.Equal(t, f.worktree, listing.Current.Path)
	require.Len(t, listing.Current.Snapshots, 1)
	require.Equal(t, f.reference, listing.Current.Snapshots[0].Reference)
	require.True(t, listing.Current.Snapshots[0].Baseline)
	require.Equal(t, head.SizeBytes(), listing.Current.Snapshots[0].SizeBytes)
	require.Equal(t, head.CreatedAt(), listing.Current.Snapshots[0].CreatedAt)

	second := filepath.Join(filepath.Dir(f.worktree), "second")
	runGit(t, "-C", filepath.Join(filepath.Dir(f.worktree), "repo"), "worktree", "add", "-b", "second", second)
	listing, err = sandbox.ListSnapshots(f.ctx, second)
	require.NoError(t, err)
	require.NotNil(t, listing.Current)
	require.Empty(t, listing.Current.Snapshots)
	listing, err = sandbox.ListSnapshots(f.ctx, t.TempDir())
	require.NoError(t, err)
	require.NotNil(t, listing.Current)
	require.Empty(t, listing.Current.Snapshots)
	index := slices.IndexFunc(listing.Workspaces, func(w sandbox.SnapshotWorkspace) bool { return w.Path == f.worktree })
	require.NotEqual(t, -1, index, "baseline remains visible in the other-workspaces listing")
	require.Len(t, listing.Workspaces[index].Snapshots, 1)
	require.Equal(t, f.reference, listing.Workspaces[index].Snapshots[0].Reference)
}

func TestFailedSnapshotPreparationKeepsPreviousBaseline(t *testing.T) {
	t.Parallel()
	f := newSnapshotFixture(t, "true")
	head, err := m.Snapshot.Get(f.ctx, f.reference)
	require.NoError(t, err)
	f.settings.Snapshot.Command = "exit 23"

	_, err = sandbox.CreateSnapshot(f.ctx, f.worktree, nil, f.settings)

	require.ErrorContains(t, err, "snapshot preparation")
	unchanged, err := m.Snapshot.Get(f.ctx, *head.Group())
	require.NoError(t, err)
	require.Equal(t, head.ID(), unchanged.ID())
}

func TestSnapshotRestoresGuestFilesDockerImagesAndVerifiedTLS(t *testing.T) {
	t.Parallel()
	f := newSnapshotFixture(t, `
test "$PWD" = /workspace
git rev-parse --show-toplevel >/dev/null
mkdir -p /root/.cache/snapshot-test
echo first > /root/.cache/snapshot-test/generation
docker volume create snapshot-baseline >/dev/null
printf 'FROM scratch\nLABEL snapshot-proof=first\n' | docker build --network=none -t snapshot-proof:local -
echo host-modified > prepared-on-host
`)
	require.FileExists(t, filepath.Join(f.worktree, "prepared-on-host"))

	restored := prepareSnapshotChild(t, f.ctx, f.worktree, f.settings)

	var stored struct {
		Network struct {
			TLS struct {
				Enabled        bool `json:"enabled"`
				VerifyUpstream bool `json:"verify_upstream"`
			} `json:"tls"`
		} `json:"network"`
	}
	require.NoError(t, json.Unmarshal([]byte(storedSandboxConfig(t, restored)), &stored))
	require.True(t, stored.Network.TLS.Enabled)
	require.True(t, stored.Network.TLS.VerifyUpstream)
	require.NotContains(t, storedSandboxConfig(t, restored), "OPENCODE_SANDBOX_TLS_BOOTSTRAP")
	require.Equal(t, "first\n", execInSandbox(t, restored, "cat", "/root/.cache/snapshot-test/generation"))
	require.Equal(t, "first\n", execInSandbox(t, restored, "docker", "image", "inspect", "snapshot-proof:local", "--format", `{{index .Config.Labels "snapshot-proof"}}`))
}

func TestSnapshotReplacementStartsFreshAndKeepsExistingVMs(t *testing.T) {
	t.Parallel()
	f := newSnapshotFixture(t, "echo first > /root/generation; touch /root/first-baseline-only")
	existing := prepareSnapshotChild(t, f.ctx, f.worktree, f.settings)
	execInSandbox(t, existing, "touch", "/root/only-a")
	execInSandbox(t, existing, "docker", "volume", "create", "only-a")
	f.settings.Snapshot.Command = "echo second > /root/generation; test ! -e /root/only-a; test ! -e /root/first-baseline-only"

	replacement, err := sandbox.CreateSnapshot(f.ctx, f.worktree, nil, f.settings)

	require.NoError(t, err)
	require.NotEqual(t, f.reference, replacement)
	_, err = m.Snapshot.Get(f.ctx, f.reference)
	require.True(t, m.IsKind(err, m.ErrSnapshotNotFound), "previous snapshot must be removed: %v", err)
	require.Equal(t, "first\n", execInSandbox(t, existing, "cat", "/root/generation"))
	require.NoError(t, sandbox.Remove(f.ctx, f.worktree))
	recreated := prepareSnapshotChild(t, f.ctx, f.worktree, f.settings)
	require.Equal(t, "second\n", execInSandbox(t, recreated, "cat", "/root/generation"))
	execInSandbox(t, recreated, "test", "!", "-e", "/root/only-a")
	execInSandbox(t, recreated, "bash", "-eu", "-c", "! docker volume inspect only-a >/dev/null 2>&1; git rev-parse --git-common-dir >/dev/null")
}

func TestRemovingSnapshotLeavesExistingVMDisksIntact(t *testing.T) {
	t.Parallel()
	f := newSnapshotFixture(t, "echo baseline > /root/generation")
	existing := prepareSnapshotChild(t, f.ctx, f.worktree, f.settings)

	require.NoError(t, sandbox.RemoveSnapshot(f.ctx, f.worktree))
	require.NoError(t, sandbox.RemoveSnapshot(f.ctx, f.worktree), "removal is idempotent")

	listing, err := sandbox.ListSnapshots(f.ctx, f.worktree)
	require.NoError(t, err)
	require.NotNil(t, listing.Current)
	require.Empty(t, listing.Current.Snapshots)
	require.NoError(t, existing.Stop(f.ctx))
	resumed, err := sandbox.Prepare(f.ctx, f.worktree, nil, f.settings)
	require.NoError(t, err)
	defer func() { require.NoError(t, resumed.Close()) }()
	require.Equal(t, "baseline\n", execInSandbox(t, resumed, "cat", "/root/generation"))
}

func TestSnapshotsAreIsolatedBetweenLinkedWorktreesAndClones(t *testing.T) {
	t.Parallel()
	f := newSnapshotFixture(t, "touch /root/baseline-proof")
	second := filepath.Join(filepath.Dir(f.worktree), "second")
	runGit(t, "-C", filepath.Join(filepath.Dir(f.worktree), "repo"), "worktree", "add", "-b", "second", second)

	linked := prepareSnapshotChild(t, f.ctx, second, f.settings)
	clone := prepareSnapshotChild(t, f.ctx, newWorktree(t), f.settings)

	execInSandbox(t, linked, "test", "!", "-e", "/root/baseline-proof")
	execInSandbox(t, clone, "test", "!", "-e", "/root/baseline-proof")
}

type snapshotFixture struct {
	ctx                 context.Context
	worktree, reference string
	settings            config.Settings
}

func newSnapshotFixture(t *testing.T, command string) snapshotFixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	t.Cleanup(cancel)
	worktree := newWorktree(t)
	settings := config.DefaultSettings()
	settings.VM.CPUs, settings.VM.MemoryMiB = 1, 768
	settings.VM.RootDiskMiB, settings.VM.DockerDiskMiB = 512, 256
	settings.Snapshot.Command = command
	t.Cleanup(func() {
		cleanup, done := context.WithTimeout(context.Background(), time.Minute)
		defer done()
		require.NoError(t, sandbox.RemoveSnapshot(cleanup, worktree))
	})
	reference, err := sandbox.CreateSnapshot(ctx, worktree, nil, settings)
	require.NoError(t, err, "prepare baseline snapshot")
	return snapshotFixture{ctx: ctx, worktree: worktree, reference: reference, settings: settings}
}

func prepareSnapshotChild(t *testing.T, ctx context.Context, directory string, settings config.Settings, secrets ...sandbox.Secret) *sandbox.Sandbox {
	t.Helper()
	s, err := sandbox.Prepare(ctx, directory, secrets, settings)
	require.NoError(t, err)
	registerSandboxCleanup(t, s)
	return s
}

func TestSnapshotRestoreUsesCurrentHostSecrets(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	worktree := newWorktree(t)
	settings := config.DefaultSettings()
	settings.VM.CPUs, settings.VM.MemoryMiB = 1, 768
	settings.VM.RootDiskMiB, settings.VM.DockerDiskMiB = 512, 256
	settings.Snapshot.Command = `test "$GH_TOKEN" = '$MSB_GH_TOKEN'; touch /root/snapshot-preparation-proof`
	path := githubTokenPath(t)
	require.NoError(t, os.WriteFile(path, []byte("synthetic-preparation-credential"), 0600))
	settings.Secrets = githubSecretSettings(path).Secrets
	secrets, err := sandbox.LoadSecrets(settings)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sandbox.RemoveSnapshot(context.Background(), worktree)) })
	_, err = sandbox.CreateSnapshot(ctx, worktree, secrets, settings)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte("synthetic-current-credential"), 0600))
	settings.Secrets["github"] = config.SecretSettings{File: path, Env: "GH_TOKEN", AllowedHosts: []string{"api.github.com"}, Headers: []string{"authorization"}}
	secrets, err = sandbox.LoadSecrets(settings)
	require.NoError(t, err)
	s := prepareSnapshotChild(t, ctx, worktree, settings, secrets...)
	execInSandbox(t, s, "test", "-f", "/root/snapshot-preparation-proof")
	execInSandbox(t, s, "bash", "-eu", "-c", `test "$GH_TOKEN" = '$MSB_GH_TOKEN'`)
	stored := storedSandboxConfig(t, s)
	require.NotContains(t, stored, "synthetic-preparation-credential")
	require.Contains(t, stored, "synthetic-current-credential")
	require.Contains(t, stored, "api.github.com")
	require.NoError(t, sandbox.Remove(ctx, worktree))
	withoutSecrets := prepareSnapshotChild(t, ctx, worktree, settings)
	execInSandbox(t, withoutSecrets, "test", "-f", "/root/snapshot-preparation-proof")
	execInSandbox(t, withoutSecrets, "bash", "-eu", "-c", `test -z "${GH_TOKEN+x}"`)
	require.NotContains(t, storedSandboxConfig(t, withoutSecrets), "synthetic-preparation-credential")
}
