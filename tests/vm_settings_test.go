package sandbox_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	launcherconfig "opencode-sandbox/internal/config"
	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	m "github.com/superradcompany/microsandbox/sdk/go"
)

func TestVMUsesCustomLocalImage(t *testing.T) {
	dir := t.TempDir()
	image := fmt.Sprintf("opencode-sandbox-image-test:%d", time.Now().UnixNano())
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(`FROM opencode-sandbox:poc
RUN printf 'custom-image' > /opencode-sandbox-image-marker
`), 0600))
	run := func(args ...string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		output, err := exec.CommandContext(ctx, args[0], args[1:]...).CombinedOutput()
		require.NoError(t, err, "%v: %s", args, output)
	}
	cleanupImage := func(args ...string) {
		t.Helper()
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			output, err := exec.CommandContext(ctx, args[0], args[1:]...).CombinedOutput()
			assert.NoError(t, err, "%v: %s", args, output)
		})
	}
	run("docker", "build", "--pull=false", "-t", image, dir)
	cleanupImage("docker", "image", "rm", image)
	archive := filepath.Join(dir, "image.tar")
	run("docker", "save", "-o", archive, image)
	run("msb", "load", "--input", archive)
	cleanupImage("msb", "image", "rm", image)

	settings := launcherconfig.DefaultSettings()
	settings.VM.Image = image
	s := startSandboxWithSettings(t, settings)
	require.Equal(t, "custom-image", execInSandbox(t, s, "cat", "/opencode-sandbox-image-marker"))
}

func TestVMMissingLocalImageFails(t *testing.T) {
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
