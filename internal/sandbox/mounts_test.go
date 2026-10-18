package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"opencode-sandbox/internal/config"

	"github.com/stretchr/testify/require"
	m "github.com/superradcompany/microsandbox/sdk/go"
)

func TestConfiguredMountsAreReadOnly(t *testing.T) {
	t.Parallel()
	settings := config.DefaultSettings()
	settings.Mounts = []config.MountSettings{{Source: t.TempDir(), Target: "/opt/skills"}}
	mounts := map[string]m.MountConfig{}
	addReadOnlyMounts(mounts, settings)
	require.Equal(t, settings.Mounts[0].Source, mounts["/opt/skills"].Bind)
	require.True(t, mounts["/opt/skills"].Readonly)
	require.Equal(t, &m.MountOwner{UID: 0, GID: 0}, mounts["/opt/skills"].Owner)
}

func TestMountsCannotOverlapManagedMounts(t *testing.T) {
	t.Parallel()
	paths := worktreePaths{root: "/host/repo", common: "/host/main/.git"}
	for _, target := range []string{"/host", "/host/repo", "/host/repo/subdir", "/host/main/.git", "/workspace", "/var/lib", "/var/lib/docker/subdir", "/proc/file", "/sys", "/dev"} {
		t.Run(target, func(t *testing.T) {
			settings := config.DefaultSettings()
			settings.Mounts = []config.MountSettings{{Source: t.TempDir(), Target: target}}
			require.ErrorContains(t, validateMounts(paths, settings), "launcher/runtime mount")
		})
	}
}

func TestMissingMountFailsBeforeRuntimePreparation(t *testing.T) {
	t.Parallel()
	settings := config.DefaultSettings()
	source := filepath.Join(t.TempDir(), "missing")
	settings.Mounts = []config.MountSettings{{Source: source, Target: "/opt/file"}}
	// Metadata validation can run without any Microsandbox runtime.
	require.ErrorIs(t, settings.ValidateMountSources(), os.ErrNotExist)
	// Validate before worktree discovery as well, so callers get the missing path.
	_, err := Prepare(context.Background(), t.TempDir(), nil, settings)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.ErrorContains(t, err, source)
}
