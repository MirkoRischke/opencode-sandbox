package sandbox_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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

func TestRepositorySnapshotLifecycle(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	worktree := newWorktree(t)
	repo := filepath.Join(filepath.Dir(worktree), "repo")
	second := filepath.Join(filepath.Dir(worktree), "second")
	runGit(t, "-C", repo, "worktree", "add", "-b", "second", second)
	settings := config.DefaultSettings()
	settings.VM.CPUs, settings.VM.MemoryMiB = 1, 768
	settings.VM.RootDiskMiB, settings.VM.DockerDiskMiB = 512, 256
	settings.Snapshot.Command = `
test "$PWD" = /workspace
git rev-parse --show-toplevel >/dev/null
mkdir -p /root/.cache/snapshot-test
echo first > /root/.cache/snapshot-test/generation
docker volume create snapshot-baseline >/dev/null
printf 'FROM scratch\nLABEL snapshot-proof=first\n' | docker build --network=none -t snapshot-proof:local -
echo host-modified > prepared-on-host
`
	t.Cleanup(func() {
		cleanup, done := context.WithTimeout(context.Background(), time.Minute)
		defer done()
		require.NoError(t, sandbox.RemoveSnapshot(cleanup, worktree))
	})
	first, err := sandbox.CreateSnapshot(ctx, worktree, nil, settings)
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(worktree, "prepared-on-host"))
	head, err := m.Snapshot.Get(ctx, first)
	require.NoError(t, err)
	group := *head.Group()
	artifact, err := head.Open(ctx)
	require.NoError(t, err)
	require.Equal(t, "worktree", artifact.Labels()["workspace-name"])
	require.Equal(t, m.SnapshotScopeDisk, artifact.Scope())
	listing, err := sandbox.ListSnapshots(ctx, worktree)
	require.NoError(t, err)
	require.NotNil(t, listing.Current)
	require.Equal(t, worktree, listing.Current.Path)
	require.Len(t, listing.Current.Snapshots, 1)
	require.Equal(t, first, listing.Current.Snapshots[0].Reference)
	require.True(t, listing.Current.Snapshots[0].Baseline)
	require.Equal(t, head.SizeBytes(), listing.Current.Snapshots[0].SizeBytes)
	require.Equal(t, head.CreatedAt(), listing.Current.Snapshots[0].CreatedAt)
	// Other worktrees and non-Git directories have independent baselines.
	listing, err = sandbox.ListSnapshots(ctx, second)
	require.NoError(t, err)
	require.Empty(t, listing.Current.Snapshots)
	listing, err = sandbox.ListSnapshots(ctx, t.TempDir())
	require.NoError(t, err)
	require.NotNil(t, listing.Current)
	require.Empty(t, listing.Current.Snapshots)
	require.Condition(t, func() bool {
		for _, r := range listing.Workspaces {
			if r.Path == worktree {
				return len(r.Snapshots) == 1 && r.Snapshots[0].Reference == first
			}
		}
		return false
	})

	// A failed fresh preparation must not replace the previous head.
	settings.Snapshot.Command = "exit 23"
	_, err = sandbox.CreateSnapshot(ctx, worktree, nil, settings)
	require.ErrorContains(t, err, "snapshot preparation")
	unchanged, err := m.Snapshot.Get(ctx, group)
	require.NoError(t, err)
	require.Equal(t, head.ID(), unchanged.ID())

	a := prepareSnapshotChild(t, ctx, worktree, settings)
	var stored struct {
		Network struct {
			TLS struct {
				Enabled        bool `json:"enabled"`
				VerifyUpstream bool `json:"verify_upstream"`
			} `json:"tls"`
		} `json:"network"`
	}
	require.NoError(t, json.Unmarshal([]byte(storedSandboxConfig(t, a)), &stored))
	require.True(t, stored.Network.TLS.Enabled)
	require.True(t, stored.Network.TLS.VerifyUpstream)
	require.NotContains(t, storedSandboxConfig(t, a), "OPENCODE_SANDBOX_TLS_BOOTSTRAP")
	require.Equal(t, "first\n", execInSandbox(t, a, "cat", "/root/.cache/snapshot-test/generation"))
	require.Equal(t, "first\n", execInSandbox(t, a, "docker", "image", "inspect", "snapshot-proof:local", "--format", `{{index .Config.Labels "snapshot-proof"}}`))
	execInSandbox(t, a, "docker", "volume", "create", "only-a")
	execInSandbox(t, a, "touch", "/root/only-a")

	// Fresh replacement must not inherit arbitrary writes from a worktree VM.
	settings.Snapshot.Command = "echo second > /root/generation; test ! -e /root/.cache/snapshot-test/generation"
	replacement, err := sandbox.CreateSnapshot(ctx, worktree, nil, settings)
	require.NoError(t, err)
	require.NotEqual(t, first, replacement)
	_, err = m.Snapshot.Get(ctx, first)
	require.True(t, m.IsKind(err, m.ErrSnapshotNotFound), "old snapshot should be removed: %v", err)
	// A different linked worktree must not inherit this directory's baseline.
	other := prepareSnapshotChild(t, ctx, second, settings)
	execInSandbox(t, other, "bash", "-eu", "-c", "test ! -e /root/generation; test ! -e /root/.cache/snapshot-test/generation")
	require.Equal(t, "first\n", execInSandbox(t, a, "cat", "/root/.cache/snapshot-test/generation"))
	// Existing VMs keep their disks; recreation restores the replacement baseline.
	require.NoError(t, sandbox.Remove(ctx, worktree))
	b := prepareSnapshotChild(t, ctx, worktree, settings)
	require.Equal(t, "second\n", execInSandbox(t, b, "cat", "/root/generation"))
	execInSandbox(t, b, "bash", "-eu", "-c", "test ! -e /root/only-a; ! docker volume inspect only-a >/dev/null 2>&1; git rev-parse --git-common-dir >/dev/null")

	// Removing the baseline does not affect either existing VM, even cold boot.
	require.NoError(t, sandbox.RemoveSnapshot(ctx, worktree))
	require.NoError(t, sandbox.RemoveSnapshot(ctx, worktree), "removal should be idempotent")
	listing, err = sandbox.ListSnapshots(ctx, worktree)
	require.NoError(t, err)
	require.NotNil(t, listing.Current)
	require.Empty(t, listing.Current.Snapshots)
	require.NoError(t, b.Stop(ctx))
	resumed, err := sandbox.Prepare(ctx, worktree, nil, settings)
	require.NoError(t, err)
	defer func() { require.NoError(t, resumed.Close()) }()
	require.Equal(t, "second\n", execInSandbox(t, resumed, "cat", "/root/generation"))
	// A separate clone must not accidentally discover this repository's group.
	clone := prepareSnapshotChild(t, ctx, newWorktree(t), settings)
	execInSandbox(t, clone, "bash", "-eu", "-c", "test ! -e /root/generation; test ! -e /root/.cache/snapshot-test/generation")
}

func prepareSnapshotChild(t *testing.T, ctx context.Context, directory string, settings config.Settings, secrets ...sandbox.Secret) *sandbox.Sandbox {
	t.Helper()
	s, err := sandbox.Prepare(ctx, directory, secrets, settings)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		handle, err := m.GetSandbox(cleanup, s.Name)
		if err == nil {
			require.NoError(t, handle.Destroy(cleanup))
		}
		if err := s.Close(); !m.IsKind(err, m.ErrInvalidHandle) {
			require.NoError(t, err)
		}
	})
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
