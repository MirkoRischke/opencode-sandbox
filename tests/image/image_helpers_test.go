package image_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	launcherconfig "opencode-sandbox/internal/config"

	"github.com/stretchr/testify/require"
)

type imageContainer string

func startImageContainer(t *testing.T) imageContainer {
	t.Helper()
	// Each test gets an isolated container with fixture files as its working directory.
	// No network, credentials, host mounts, privileged mode, or Docker socket.
	id := strings.TrimSpace(docker(t, "run", "--rm", "-d", "--network", "none",
		"--platform", "linux/amd64", "--workdir", "/tmp/image-smoke",
		launcherconfig.DefaultImage(), "sleep", "infinity"))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "docker", "rm", "--force", id).CombinedOutput()
		require.NoError(t, err, "%s", out)
	})
	docker(t, "cp", "testdata/.", id+":/tmp/image-smoke")
	return imageContainer(id)
}

func (c imageContainer) exec(t *testing.T, args ...string) string {
	t.Helper()
	return docker(t, append([]string{"exec", string(c)}, args...)...)
}

func docker(t *testing.T, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	require.NoError(t, err, "docker %v: %s", args, out)
	t.Logf("docker %v: %s", args, out)
	return string(out)
}
