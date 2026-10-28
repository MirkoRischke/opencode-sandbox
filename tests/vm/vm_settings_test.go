package sandbox_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
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
	settings := testSettings(t)
	settings.VM.Image = "opencode-sandbox:test-custom"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	s, err := sandbox.Prepare(ctx, newWorktree(t), nil, settings)
	require.NoError(t, err, "prepare test images with 'mise run test:prepare' before running VM tests")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		assert.NoError(t, s.Remove(ctx))
	})
	require.Equal(t, "custom-image", execInSandbox(t, s, "cat", "/opencode-sandbox-image-marker"))
	output, err := exec.CommandContext(ctx, "docker", "image", "inspect", "--format", "{{.Id}}", settings.VM.Image).Output()
	require.NoError(t, err)
	cacheRef := "opencode-sandbox-cache:sha256-" + strings.TrimPrefix(strings.TrimSpace(string(output)), "sha256:")
	_, err = m.Image.Get(ctx, cacheRef)
	require.NoError(t, err, "Docker image must be imported under its identity-derived reference")
	var config m.SandboxConfig
	require.NoError(t, json.Unmarshal([]byte(storedSandboxConfig(t, s)), &config))
	require.Equal(t, cacheRef, config.Image)
}

func TestVMMissingImagePullFails(t *testing.T) {
	t.Parallel()
	settings := testSettings(t)
	// A closed loopback port exercises pull failure without a public registry.
	settings.VM.Image = fmt.Sprintf("127.0.0.1:1/opencode-sandbox-missing-test:%d", time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	s, err := sandbox.Prepare(ctx, newWorktree(t), nil, settings)
	if s != nil {
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			assert.NoError(t, s.Remove(ctx))
		})
	}
	require.ErrorContains(t, err, "prepare sandbox")
	require.ErrorContains(t, err, "docker pull image")
	require.NoError(t, ctx.Err(), "missing image must fail before the deadline")
}

func TestVMResourcesFollowConfigAndPersistAcrossResume(t *testing.T) {
	t.Parallel()
	settings := testSettings(t)
	settings.VM = launcherconfig.VMSettings{Image: settings.VM.Image, CPUs: 1, MemoryMiB: 1024, RootDiskMiB: 5120, DockerDiskMiB: 2048}
	s := startSandboxWithSettings(t, settings)
	var config m.SandboxConfig
	require.NoError(t, json.Unmarshal([]byte(storedSandboxConfig(t, s)), &config))
	require.EqualValues(t, 1, config.CPUs)
	require.EqualValues(t, 1024, config.MemoryMiB)
	require.NotNil(t, config.RootDisk)
	requireRootDiskCapacity(t, s, settings.VM.RootDiskMiB)
	require.Equal(t, "2147483648\n", execInSandbox(t, s, "sh", "-ec", `blockdev --getsize64 "$(findmnt -n -o SOURCE /var/lib/docker)"`))
	require.Equal(t, "1\n", execInSandbox(t, s, "nproc"))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	require.NoError(t, s.Stop(ctx))
	resumeSettings := testSettings(t)
	resumeSettings.VM.Image = "127.0.0.1:1/must-not-resolve:resume"
	resumed, err := sandbox.Prepare(ctx, s.Worktree, nil, resumeSettings)
	require.NoError(t, err)
	defer func() { require.NoError(t, resumed.Close()) }()
	require.NoError(t, json.Unmarshal([]byte(storedSandboxConfig(t, resumed)), &config))
	require.EqualValues(t, 1, config.CPUs)
	require.EqualValues(t, 1024, config.MemoryMiB)
	require.NotNil(t, config.RootDisk)
	requireRootDiskCapacity(t, resumed, settings.VM.RootDiskMiB)
	require.Equal(t, "2147483648\n", execInSandbox(t, resumed, "sh", "-ec", `blockdev --getsize64 "$(findmnt -n -o SOURCE /var/lib/docker)"`))
}

func requireRootDiskCapacity(t *testing.T, s *sandbox.Sandbox, sizeMiB uint32) {
	t.Helper()
	// Restored root-disk metadata can leave SizeMiB unspecified (MSB tracker).
	// Check both the actual device and the filesystem mounted on it instead.
	require.Equal(t, fmt.Sprintf("%d\n", uint64(sizeMiB)*1024*1024),
		execInSandbox(t, s, "sh", "-ec", `
# The root is overlayfs; its backing mount is hidden after pivoting root.
# This fixture has two writable disks: the root upper and the Docker disk.
docker=$(findmnt -n -o SOURCE /var/lib/docker)
root=$(lsblk -dnpo NAME,RO | awk -v docker="$docker" '$2 == 0 && $1 != docker {print $1}')
blockdev --getsize64 "$root"`))
	filesystem := execInSandbox(t, s, "sh", "-ec", `df -B1 --output=size / | tail -n 1`)
	var filesystemBytes uint64
	_, err := fmt.Sscan(filesystem, &filesystemBytes)
	require.NoError(t, err)
	// Filesystem metadata consumes some capacity; it must still span the disk.
	require.Greater(t, filesystemBytes, uint64(sizeMiB)*1024*1024*9/10)
	require.LessOrEqual(t, filesystemBytes, uint64(sizeMiB)*1024*1024)
}
