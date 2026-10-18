package sandbox_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	launcherconfig "opencode-sandbox/internal/config"
	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	m "github.com/superradcompany/microsandbox/sdk/go"
)

func TestVMUsesCustomLocalImage(t *testing.T) {
	t.Parallel()
	settings := launcherconfig.DefaultSettings()
	settings.VM.Image = "opencode-sandbox:test-custom"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s, err := sandbox.Prepare(ctx, newWorktree(t), nil, nil, settings)
	require.NoError(t, err, "prepare test images with 'mise run test:prepare' before running VM tests")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		assert.NoError(t, s.Remove(ctx))
	})
	require.Equal(t, "custom-image", execInSandbox(t, s, "cat", "/opencode-sandbox-image-marker"))
}

func TestVMMissingLocalImageFails(t *testing.T) {
	t.Parallel()
	settings := launcherconfig.DefaultSettings()
	settings.VM.Image = fmt.Sprintf("opencode-sandbox-missing-test:%d", time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	s, err := sandbox.Prepare(ctx, newWorktree(t), nil, nil, settings)
	if s != nil {
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			assert.NoError(t, s.Remove(ctx))
		})
	}
	require.ErrorContains(t, err, "prepare sandbox")
	require.NoError(t, ctx.Err(), "missing image must fail before the deadline")
}

func TestVMResourcesFollowConfigAndPersistAcrossResume(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeSettings(t, "[vm]\ncpus = 1\nmemory_mib = 1024\nroot_disk_mib = 5120\ndocker_disk_mib = 2048\n")
	settings, err := launcherconfig.LoadSettings()
	require.NoError(t, err)
	s := startSandboxWithSettings(t, settings)
	var config m.SandboxConfig
	require.NoError(t, json.Unmarshal([]byte(storedSandboxConfig(t, s)), &config))
	require.EqualValues(t, 1, config.CPUs)
	require.EqualValues(t, 1024, config.MemoryMiB)
	require.NotNil(t, config.RootDisk)
	require.EqualValues(t, 5120, config.RootDisk.SizeMiB)
	require.Equal(t, "2147483648\n", execInSandbox(t, s, "sh", "-ec", `blockdev --getsize64 "$(findmnt -n -o SOURCE /var/lib/docker)"`))
	require.Equal(t, "1\n", execInSandbox(t, s, "nproc"))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
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
	require.Equal(t, "2147483648\n", execInSandbox(t, resumed, "sh", "-ec", `blockdev --getsize64 "$(findmnt -n -o SOURCE /var/lib/docker)"`))
}
