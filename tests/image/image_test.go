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
		"jq", "rg", "file", "patch", "less", "tree", "curl", "unzip", "vim", "xz",
		"ps", "pgrep", "pkill", "ip", "ss", "dig", "nslookup", "nc", "ping", "lsof", "findmnt",
	} {
		t.Run(tool, func(t *testing.T) {
			container.exec(t, "sh", "-ec", `command -v "$1"`, "sh", tool)
		})
	}
	for _, tool := range []string{"git", "gh", "aws", "mise", "opencode", "vim"} {
		container.exec(t, tool, "--version")
	}
}

func TestDockerCLIAndConfiguration(t *testing.T) {
	t.Parallel()
	container := startImageContainer(t)

	container.exec(t, "docker", "--version")
	container.exec(t, "dockerd", "--version")
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
