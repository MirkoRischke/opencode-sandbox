package image_test

import (
	"testing"

	launcherconfig "opencode-sandbox/internal/config"

	"github.com/stretchr/testify/require"
)

const baseImage = "opencode-sandbox:base"

func TestToolsAreAvailable(t *testing.T) {
	t.Parallel()
	container := startImageContainer(t, baseImage)

	for _, tool := range []string{
		"mise", "git", "gh", "aws", "docker", "dockerd",
		"jq", "rg", "file", "patch", "less", "tree", "curl", "vim", "xz",
		"ps", "pgrep", "pkill", "ss", "findmnt",
		"gcc", // Go's Linux race detector requires a C compiler.
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

func TestAgentImages(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		image string
		agent string
	}{
		{name: "base", image: baseImage},
		{name: "opencode", image: launcherconfig.DefaultImage(), agent: "opencode"},
		{name: "codex", image: "opencode-sandbox:codex", agent: "codex"},
		{name: "copilot", image: "opencode-sandbox:copilot", agent: "copilot"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			container := startImageContainer(t, tt.image)
			for _, agent := range []string{"opencode", "codex", "copilot"} {
				if agent == tt.agent {
					container.exec(t, agent, "--version")
				} else {
					container.exec(t, "sh", "-ec", `! command -v "$1"`, "sh", agent)
				}
			}
		})
	}
}

func TestDockerCLIAndConfiguration(t *testing.T) {
	t.Parallel()
	container := startImageContainer(t, baseImage)

	container.exec(t, "docker", "buildx", "version")
	container.exec(t, "docker", "compose", "version")
	container.exec(t, "dockerd", "--validate", "--config-file=/etc/docker/daemon.json")
	container.exec(t, "docker", "compose", "config", "--quiet")
}

func TestMiseTask(t *testing.T) {
	t.Parallel()
	container := startImageContainer(t, baseImage)

	container.exec(t, "mise", "trust", "mise.toml")
	output := container.exec(t, "mise", "run", "--quiet", "smoke")
	require.Equal(t, "mise-smoke-ok", output)
}
