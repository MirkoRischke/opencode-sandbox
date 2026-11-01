package image_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestToolsAreAvailable(t *testing.T) {
	t.Parallel()
	container := startImageContainer(t)

	for _, tool := range []string{
		"mise", "git", "gh", "aws", "opencode", "docker", "dockerd",
		"jq", "rg", "file", "patch", "less", "tree", "curl", "vim", "xz",
		"ps", "pgrep", "pkill", "ss", "findmnt",
	} {
		t.Run("version/"+tool, func(t *testing.T) {
			container.exec(t, tool, "--version")
		})
	}
	// These tools do not support --version; check PATH availability instead.
	for _, tool := range []string{"unzip", "ip", "dig", "nslookup", "nc", "ping", "lsof"} {
		t.Run("command/"+tool, func(t *testing.T) {
			container.exec(t, "sh", "-ec", `command -v "$1"`, "sh", tool)
		})
	}
}

func TestDockerCLIAndConfiguration(t *testing.T) {
	t.Parallel()
	container := startImageContainer(t)

	container.exec(t, "docker", "buildx", "version")
	container.exec(t, "docker", "compose", "version")
	container.exec(t, "dockerd", "--validate", "--config-file=/etc/docker/daemon.json")
	container.exec(t, "docker", "compose", "config", "--quiet")
}

func TestMiseTask(t *testing.T) {
	t.Parallel()
	container := startImageContainer(t)

	container.exec(t, "mise", "trust", "mise.toml")
	output := container.exec(t, "mise", "run", "--quiet", "smoke")
	require.Equal(t, "mise-smoke-ok", output)
}
