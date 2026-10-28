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
	settings := testSettings(t)
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
	settings.VM.Image = "127.0.0.1:1/must-not-resolve:snapshot-restore"
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

func TestSnapshotPreparationHasPublicAccessButRestoredVMIsRestricted(t *testing.T) {
	t.Parallel()
	for _, automatic := range []bool{false, true} {
		name := "manual"
		if automatic {
			name = "automatic"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
			defer cancel()
			worktree := newWorktree(t)
			settings := testSettings(t)
			settings.Network.AllowedHosts = []string{}
			settings.Network.AdditionalAllowedHosts = nil
			settings.Snapshot.Command = `
curl --noproxy '*' --fail --silent --show-error --max-time 15 https://example.com > /root/preparation-download
if curl --noproxy '*' --insecure --silent --max-time 3 https://host.microsandbox.internal; then
  echo 'preparation unexpectedly reached host' >&2
  exit 1
fi
`
			if !automatic {
				_, err := sandbox.CreateSnapshot(ctx, worktree, nil, settings)
				require.NoError(t, err)
			}
			child := prepareSnapshotChild(t, ctx, worktree, settings)
			require.Contains(t, execInSandbox(t, child, "cat", "/root/preparation-download"), "Example Domain")
			require.False(t, settings.Network.Unrestricted, "preparation must not mutate caller settings")
			requireNetworkBlocked(t, child, "https://example.com")
			requireNetworkBlocked(t, child, "https://host.microsandbox.internal")
		})
	}
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
	f.settings.Snapshot.Command = "exit 42"
	resumed, err := sandbox.Prepare(f.ctx, f.worktree, nil, f.settings)
	require.NoError(t, err)
	defer func() { require.NoError(t, resumed.Close()) }()
	require.Equal(t, "baseline\n", execInSandbox(t, resumed, "cat", "/root/generation"))
	listing, err = sandbox.ListSnapshots(f.ctx, f.worktree)
	require.NoError(t, err)
	require.Empty(t, listing.Current.Snapshots, "an existing VM must not recreate a removed snapshot")
}

func TestSnapshotsAreIsolatedBetweenLinkedWorktreesAndClones(t *testing.T) {
	t.Parallel()
	f := newSnapshotFixture(t, "touch /root/baseline-proof")
	second := filepath.Join(filepath.Dir(f.worktree), "second")
	runGit(t, "-C", filepath.Join(filepath.Dir(f.worktree), "repo"), "worktree", "add", "-b", "second", second)

	f.settings.Snapshot.Command = ""
	linked := prepareSnapshotChild(t, f.ctx, second, f.settings)
	clone := prepareSnapshotChild(t, f.ctx, newWorktree(t), f.settings)

	execInSandbox(t, linked, "test", "!", "-e", "/root/baseline-proof")
	execInSandbox(t, clone, "test", "!", "-e", "/root/baseline-proof")
}

func TestFirstLaunchPreparesManagedSnapshotOnlyOnce(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	worktree := newWorktree(t)
	settings := testSettings(t)
	// A deterministic offline mise substitute detects every provisioning call,
	// including trust/config discovery, without downloading a toolchain.
	mise := filepath.Join(t.TempDir(), "mise")
	require.NoError(t, os.WriteFile(mise, []byte(`#!/bin/bash
set -eu
echo "$1" >> /root/mise-calls
case "$1" in
  trust) ;;
  config) echo "$PWD/mise.toml" ;;
  install) touch /root/tools-installed ;;
  settings) ;;
  *) exit 99 ;;
esac
`), 0700))
	settings.Files = []config.FileSettings{{Source: mise, Target: "/usr/local/bin/mise", Mode: "copy"}}
	settings.Snapshot.Command = `test -f /root/tools-installed; echo prepared > /root/baseline; echo prepared >> preparation-count`
	s := prepareSnapshotChild(t, ctx, worktree, settings)
	wantCalls := "trust\nconfig\ninstall\nsettings\n"
	require.Equal(t, wantCalls, execInSandbox(t, s, "cat", "/root/mise-calls"))
	require.Equal(t, "prepared\n", execInSandbox(t, s, "cat", "/root/baseline"))
	execInSandbox(t, s, "docker", "info")
	listing, err := sandbox.ListSnapshots(ctx, worktree)
	require.NoError(t, err)
	require.Len(t, listing.Current.Snapshots, 1)
	require.True(t, listing.Current.Snapshots[0].Baseline)
	reference := listing.Current.Snapshots[0].Reference

	// Neither config changes nor a restart invalidate the VM or its baseline.
	settings.Snapshot.Command = "exit 42"
	settings.VM.Image = "127.0.0.1:1/must-not-resolve:existing-baseline"
	require.NoError(t, s.Stop(ctx))
	resumed := prepareSnapshotChild(t, ctx, worktree, settings)
	require.Equal(t, wantCalls, execInSandbox(t, resumed, "cat", "/root/mise-calls"))
	require.NoError(t, sandbox.Remove(ctx, worktree))
	restored := prepareSnapshotChild(t, ctx, worktree, settings)
	require.Equal(t, wantCalls, execInSandbox(t, restored, "cat", "/root/mise-calls"))
	require.Equal(t, "prepared\n", execInSandbox(t, restored, "cat", "/root/baseline"))
	count, err := os.ReadFile(filepath.Join(worktree, "preparation-count"))
	require.NoError(t, err)
	require.Equal(t, "prepared\n", string(count))
	listing, err = sandbox.ListSnapshots(ctx, worktree)
	require.NoError(t, err)
	require.Len(t, listing.Current.Snapshots, 1)
	require.Equal(t, reference, listing.Current.Snapshots[0].Reference)
	require.NoError(t, sandbox.RemoveSnapshot(ctx, worktree))
	listing, err = sandbox.ListSnapshots(ctx, worktree)
	require.NoError(t, err)
	require.Empty(t, listing.Current.Snapshots)
}

func TestFailedAutomaticSnapshotPreparationDoesNotCreateWorkspaceVM(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	worktree := newWorktree(t)
	settings := testSettings(t)
	settings.Snapshot.Command = "exit 23"
	t.Cleanup(func() {
		require.NoError(t, sandbox.Remove(context.Background(), worktree))
		require.NoError(t, sandbox.RemoveSnapshot(context.Background(), worktree))
	})
	s, err := sandbox.Prepare(ctx, worktree, nil, settings)
	require.Nil(t, s)
	require.ErrorContains(t, err, "snapshot preparation")
	listing, err := sandbox.ListSnapshots(ctx, worktree)
	require.NoError(t, err)
	require.Empty(t, listing.Current.Snapshots)
	removed := false
	require.NoError(t, sandbox.Remove(ctx, worktree, func(string, *uint64) { removed = true }))
	require.False(t, removed, "failed preparation must not leave a reusable workspace VM")
	settings.Snapshot.Command = "echo recovered > /root/baseline"
	recovered := prepareSnapshotChild(t, ctx, worktree, settings)
	require.Equal(t, "recovered\n", execInSandbox(t, recovered, "cat", "/root/baseline"))
}

func TestAutomaticSnapshotStopsOnMiseInstallationFailure(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	worktree := newWorktree(t)
	settings := testSettings(t)
	mise := filepath.Join(t.TempDir(), "mise")
	require.NoError(t, os.WriteFile(mise, []byte(`#!/bin/bash
case "$1" in
  trust) ;;
  config) echo "$PWD/mise.toml" ;;
  install) exit 24 ;;
esac
`), 0700))
	settings.Files = []config.FileSettings{{Source: mise, Target: "/usr/local/bin/mise", Mode: "copy"}}
	settings.Snapshot.Command = "touch custom-command-ran"
	t.Cleanup(func() {
		require.NoError(t, sandbox.Remove(context.Background(), worktree))
		require.NoError(t, sandbox.RemoveSnapshot(context.Background(), worktree))
	})
	s, err := sandbox.Prepare(ctx, worktree, nil, settings)
	require.Nil(t, s)
	require.ErrorContains(t, err, "initialize project tools")
	require.NoFileExists(t, filepath.Join(worktree, "custom-command-ran"))
	listing, err := sandbox.ListSnapshots(ctx, worktree)
	require.NoError(t, err)
	require.Empty(t, listing.Current.Snapshots)
	removed := false
	require.NoError(t, sandbox.Remove(ctx, worktree, func(string, *uint64) { removed = true }))
	require.False(t, removed)
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
	settings := testSettings(t)
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
	settings := testSettings(t)
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
