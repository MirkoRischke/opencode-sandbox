package sandbox_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	launcherconfig "opencode-sandbox/internal/config"
	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/require"
	m "github.com/superradcompany/microsandbox/sdk/go"
)

func TestVMResourcesFollowConfigAndPersistAcrossResume(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeSettings(t, "[vm]\ncpus = 1\nmemory_mib = 1024\nroot_disk_mib = 5120\n")
	settings, err := launcherconfig.LoadSettings()
	require.NoError(t, err)
	s := startSandboxWithSettings(t, settings)
	var config m.SandboxConfig
	require.NoError(t, json.Unmarshal([]byte(storedSandboxConfig(t, s)), &config))
	require.EqualValues(t, 1, config.CPUs)
	require.EqualValues(t, 1024, config.MemoryMiB)
	require.NotNil(t, config.RootDisk)
	require.EqualValues(t, 5120, config.RootDisk.SizeMiB)
	require.Equal(t, "1\n", execInSandbox(t, s, "nproc"))
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	require.NoError(t, s.Stop(ctx))
	resumed, err := sandbox.Prepare(ctx, s.Worktree, nil, nil, launcherconfig.DefaultSettings())
	require.NoError(t, err)
	defer func() { require.NoError(t, resumed.Close()) }()
	require.NoError(t, json.Unmarshal([]byte(storedSandboxConfig(t, resumed)), &config))
	require.EqualValues(t, 1, config.CPUs)
	require.EqualValues(t, 1024, config.MemoryMiB)
	require.NotNil(t, config.RootDisk)
	require.EqualValues(t, 5120, config.RootDisk.SizeMiB)
}
