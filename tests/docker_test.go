package sandbox_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/require"
	m "github.com/superradcompany/microsandbox/sdk/go"
)

func TestDockerBuildComposeAndPersistence(t *testing.T) {
	t.Parallel()
	s := startSandbox(t)
	execInSandbox(t, s, "docker", "buildx", "version")
	execInSandbox(t, s, "docker", "compose", "version")
	require.Equal(t, "overlay\n", execInSandbox(t, s, "findmnt", "-n", "-o", "FSTYPE", "/"))
	require.Equal(t, "ext4\n", execInSandbox(t, s, "findmnt", "-n", "-o", "FSTYPE", "/var/lib/docker"))
	require.Equal(t, "/var/lib/docker overlay2\n", execInSandbox(t, s, "docker", "info", "--format", "{{.DockerRootDir}} {{.Driver}}"))
	var config struct {
		Network struct {
			Ports []json.RawMessage `json:"ports"`
		} `json:"network"`
	}
	require.NoError(t, json.Unmarshal([]byte(storedSandboxConfig(t, s)), &config))
	require.NotNil(t, config.Network.Ports)
	require.Empty(t, config.Network.Ports, "container ports must not be forwarded to the host")

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	var output bytes.Buffer
	// Both pulls must work through the example's exact-host HTTPS policy and
	// intercepted TLS, without credentials or insecure-registry exceptions.
	for _, image := range []string{"busybox:1.37.0", "public.ecr.aws/docker/library/busybox:1.37.0"} {
		require.NoError(t, s.Exec(ctx, nil, &output, &output, "docker", "pull", image), "%s", output.String())
	}
	dir := filepath.Join(s.Worktree, "docker-test")
	require.NoError(t, os.Mkdir(dir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(`FROM busybox:1.37.0
RUN mkdir /www && printf 'docker-ok' > /www/index.html
CMD ["cat", "/www/index.html"]
`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(`services:
  server:
    image: sandbox-docker-test:local
    pull_policy: never
    command: ["httpd", "-f", "-p", "80", "-h", "/www"]
    ports: ["127.0.0.1:18080:80"]
    volumes: ["data:/data"]
  client:
    image: sandbox-docker-test:local
    pull_policy: never
    command: ["sh", "-ec", "for i in 1 2 3 4 5; do wget -qO- http://server && exit 0; sleep 1; done; exit 1"]
volumes:
  data:
`), 0600))
	output.Reset()
	require.NoError(t, s.Exec(ctx, nil, &output, &output, "docker", "buildx", "build", "--load", "-t", "sandbox-docker-test:local", dir), "%s", output.String())
	require.Equal(t, "docker-ok", execInSandbox(t, s, "docker", "run", "--rm", "sandbox-docker-test:local"))
	output.Reset()
	err := s.Exec(ctx, nil, &output, &output, "docker", "run", "--rm", "sandbox-docker-test:local", "wget", "-T", "5", "-O-", "http://example.com")
	require.Error(t, err, "nested containers must not bypass the VM's HTTPS-only allowlist")
	compose := []string{"docker", "compose", "-p", "sandbox-test", "-f", filepath.Join(dir, "compose.yaml")}
	execInSandbox(t, s, append(append([]string{}, compose...), "up", "-d", "server")...)
	require.Contains(t, execInSandbox(t, s, append(append([]string{}, compose...), "run", "--rm", "client")...), "docker-ok")
	require.Equal(t, "docker-ok", execInSandbox(t, s, "curl", "--fail", "--retry", "5", "--retry-connrefused", "--silent", "http://127.0.0.1:18080"))
	execInSandbox(t, s, append(append([]string{}, compose...), "exec", "-T", "server", "sh", "-c", "echo persisted > /data/proof")...)
	// EnsureDocker must leave a healthy daemon (and its log) untouched.
	pid := execInSandbox(t, s, "cat", "/var/run/docker.pid")
	require.NoError(t, s.EnsureDocker(ctx))
	require.Equal(t, pid, execInSandbox(t, s, "cat", "/var/run/docker.pid"))
	execInSandbox(t, s, "sh", "-c", "echo log-marker >> /var/log/opencode-sandbox/dockerd.log")
	execInSandbox(t, s, "docker", "create", "--name", "retained", "sandbox-docker-test:local")
	execInSandbox(t, s, append(append([]string{}, compose...), "down")...)
	require.NoError(t, s.Stop(ctx))
	resumed, err := sandbox.Prepare(ctx, s.Worktree, nil, nil, testSettings(t))
	require.NoError(t, err)
	defer func() { require.NoError(t, resumed.Close()) }()
	require.Equal(t, "docker-ok", execInSandbox(t, resumed, "docker", "run", "--rm", "sandbox-docker-test:local"))
	require.Equal(t, "docker-ok", execInSandbox(t, resumed, "docker", "start", "--attach", "retained"))
	require.Equal(t, "persisted\n", execInSandbox(t, resumed, "docker", "run", "--rm", "-v", "sandbox-test_data:/data", "sandbox-docker-test:local", "cat", "/data/proof"))
	require.Contains(t, execInSandbox(t, resumed, "cat", "/var/log/opencode-sandbox/dockerd.log"), "log-marker")
}

func TestDockerStartupWithoutDockerd(t *testing.T) {
	t.Parallel()
	s := startSandbox(t)
	// Rename only this VM's writable copy, never the shared base image. A cold
	// restart makes sure no old daemon can disguise the missing-binary case.
	execInSandbox(t, s, "sh", "-ec", `mv "$(command -v dockerd)" /usr/local/dockerd-disabled`)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	require.NoError(t, s.Stop(ctx))
	resumed, err := sandbox.Prepare(ctx, s.Worktree, nil, nil, testSettings(t))
	require.NoError(t, err, "missing Docker is optional, not a launch failure")
	defer func() { require.NoError(t, resumed.Close()) }()
	execInSandbox(t, resumed, "sh", "-ec", "! command -v dockerd; ! docker info >/dev/null 2>&1")
	require.NoError(t, resumed.EnsureDocker(ctx))
}

func TestDockerStartupFailureReportsLogs(t *testing.T) {
	t.Parallel()
	s := startSandbox(t)
	execInSandbox(t, s, "sh", "-ec", `printf '{invalid-json' > /etc/docker/daemon.json`)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, s.Stop(ctx))
	_, err := sandbox.Prepare(ctx, s.Worktree, nil, nil, testSettings(t))
	require.NoError(t, ctx.Err(), "Docker startup failures must be reported before the deadline")
	require.ErrorContains(t, err, "initialize guest Docker")
	require.ErrorContains(t, err, "Docker exited before becoming ready")
	require.ErrorContains(t, err, "daemon.json")
	require.ErrorContains(t, err, "invalid character")
}

func TestDockerRestartClearsPreviousBootPIDFiles(t *testing.T) {
	t.Parallel()
	s := startSandbox(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	require.NoError(t, s.Stop(ctx))
	// Boot without launcher initialization and force PID reuse deterministically:
	// PID 1 is alive, but is neither Docker nor its managed containerd.
	vm, err := m.ConnectOrCreateSandbox(ctx, s.Name)
	require.NoError(t, err)
	defer func() { require.NoError(t, vm.Close()) }()
	out, err := vm.Exec(ctx, "sh", []string{"-ec", "echo 1 > /var/run/docker.pid; echo 1 > /var/run/docker/containerd/containerd.pid"})
	require.NoError(t, err)
	require.Zero(t, out.ExitCode())
	resumed, err := sandbox.Prepare(ctx, s.Worktree, nil, nil, testSettings(t))
	require.NoError(t, err, "previous-boot PID files must not prevent startup")
	defer func() { require.NoError(t, resumed.Close()) }()
	execInSandbox(t, resumed, "docker", "info")
}

func TestDockerDiskIsPrivateAndRemovedWithVM(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	worktree := newWorktree(t)
	settings := testSettings(t)
	s, err := sandbox.Prepare(ctx, worktree, nil, nil, settings)
	require.NoError(t, err)
	t.Cleanup(func() {
		if s != nil {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cleanupCancel()
			require.NoError(t, s.Remove(cleanupCtx))
		}
	})
	// Inspect the persisted mount contract, not the SDK's creation-only config
	// decoder (which does not decode mounts).
	var config struct {
		Mounts []struct {
			Type    string `json:"type"`
			Guest   string `json:"guest"`
			Storage struct {
				Kind        string `json:"kind"`
				CapacityMiB uint32 `json:"capacity_mib"`
			} `json:"storage"`
		} `json:"mounts"`
	}
	require.NoError(t, json.Unmarshal([]byte(storedSandboxConfig(t, s)), &config))
	found := false
	for _, mount := range config.Mounts {
		if mount.Guest == "/var/lib/docker" {
			found = true
			require.Equal(t, "Owned", mount.Type)
			require.Equal(t, "disk", mount.Storage.Kind)
			require.Equal(t, settings.VM.DockerDiskMiB, mount.Storage.CapacityMiB)
		}
	}
	require.True(t, found, "Docker storage must be owned by this VM")
	execInSandbox(t, s, "touch", "/var/lib/docker/old-vm-proof")
	proof := filepath.Join(worktree, "host-proof")
	require.NoError(t, os.WriteFile(proof, []byte("keep"), 0600))
	name := s.Name
	require.NoError(t, s.Remove(ctx))
	s = nil
	_, err = m.GetSandbox(ctx, name)
	require.Error(t, err)
	require.FileExists(t, proof)
	// Same worktree, new VM: no old disk data is reused.
	s, err = sandbox.Prepare(ctx, worktree, nil, nil, settings)
	require.NoError(t, err)
	execInSandbox(t, s, "test", "!", "-e", "/var/lib/docker/old-vm-proof")
}
