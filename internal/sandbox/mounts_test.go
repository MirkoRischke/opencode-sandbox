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

func TestConfiguredMountModes(t *testing.T) {
	t.Parallel()
	settings := config.DefaultSettings()
	settings.Files = []config.FileSettings{
		{Source: t.TempDir(), Target: "/opt/default"},
		{Source: t.TempDir(), Target: "/opt/legacy", Mode: "mount"},
		{Source: t.TempDir(), Target: "/opt/read-only", Mode: "mount-ro"},
		{Source: t.TempDir(), Target: "/opt/writable", Mode: "mount-rw"},
		{Source: t.TempDir(), Target: "/opt/copy", Mode: "copy"},
	}
	require.NoError(t, settings.Validate())
	mounts := map[string]m.MountConfig{}
	addFileMounts(mounts, settings)
	for _, file := range settings.Files[:4] {
		require.Equal(t, file.Source, mounts[file.Target].Bind)
		require.Equal(t, &m.MountOwner{UID: 0, GID: 0}, mounts[file.Target].Owner)
	}
	require.NotContains(t, mounts, "/opt/copy")
	require.True(t, mounts["/opt/default"].Readonly)
	require.True(t, mounts["/opt/legacy"].Readonly)
	require.True(t, mounts["/opt/read-only"].Readonly)
	require.False(t, mounts["/opt/writable"].Readonly)
}

func TestMountsCannotOverlapManagedMounts(t *testing.T) {
	t.Parallel()
	w, err := workspace.Resolve(context.Background(), t.TempDir())
	require.NoError(t, err)
	for _, target := range []string{"/workspace", "/workspace/subdir", "/var/lib", "/var/lib/docker/subdir", "/proc/file", "/sys", "/dev", "/etc", sandboxConfigDir, sandboxInstructions, "/run/opencode-sandbox", "/var/run/opencode-sandbox", "/var/log/opencode-sandbox"} {
		t.Run(target, func(t *testing.T) {
			settings := config.DefaultSettings()
			settings.Files = []config.FileSettings{{Source: t.TempDir(), Target: target}}
			require.ErrorContains(t, validateMounts(w, settings), "launcher/runtime mount")
			settings.Files[0].Mode = "copy"
			require.ErrorContains(t, validateMounts(w, settings), "launcher/runtime mount")
			settings.Files = nil
			settings.State = []config.StateSettings{{Name: "history", Target: target}}
			require.ErrorContains(t, validateMounts(w, settings), "state[0].target")
		})
	}
}

func TestMountsAllowUserSuppliedOpenCodeSystemConfig(t *testing.T) {
	t.Parallel()
	w, err := workspace.Resolve(t.Context(), t.TempDir())
	require.NoError(t, err)
	settings := config.DefaultSettings()
	settings.Files = []config.FileSettings{{Source: t.TempDir(), Target: "/etc/opencode"}}
	require.NoError(t, validateMounts(w, settings))
}

func TestMissingMountFailsBeforeRuntimePreparation(t *testing.T) {
	t.Parallel()
	settings := config.DefaultSettings()
	source := filepath.Join(t.TempDir(), "missing")
	settings.Files = []config.FileSettings{{Source: source, Target: "/opt/file"}}
	// Metadata validation can run without any Microsandbox runtime.
	require.ErrorIs(t, settings.ValidateFileSources(), os.ErrNotExist)
	// Missing sources fail before preparing any runtime resources.
	_, err := Prepare(context.Background(), t.TempDir(), nil, settings)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.ErrorContains(t, err, source)
}
