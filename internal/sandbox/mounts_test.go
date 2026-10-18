package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"opencode-sandbox/internal/config"
	"opencode-sandbox/internal/workspace"

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
	w, err := workspace.Resolve(context.Background(), t.TempDir())
	require.NoError(t, err)
	for _, target := range []string{filepath.Dir(w.Root()), w.Root(), filepath.Join(w.Root(), "subdir"), "/workspace", "/var/lib", "/var/lib/docker/subdir", "/proc/file", "/sys", "/dev", "/etc", "/etc/opencode", systemOpenCodeConfig, "/etc/opencode-sandbox", sandboxInstructions} {
		t.Run(target, func(t *testing.T) {
			settings := config.DefaultSettings()
			settings.Mounts = []config.MountSettings{{Source: t.TempDir(), Target: target}}
			require.ErrorContains(t, validateMounts(w, settings), "launcher/runtime mount")
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
	// Missing sources fail before preparing any runtime resources.
	_, err := Prepare(context.Background(), t.TempDir(), nil, settings)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.ErrorContains(t, err, source)
}
