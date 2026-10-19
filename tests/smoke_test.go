package sandbox_test

import "testing"

func TestDefaultToolsAreAvailable(t *testing.T) {
	t.Parallel()
	s := startSandbox(t)
	tools := []string{
		"mise", "git", "gh", "aws", "opencode", "docker", "dockerd",
		"jq", "rg", "file", "patch", "less", "tree",
		"ps", "pgrep", "pkill", "ip", "ss", "dig", "nslookup", "nc", "ping", "lsof",
	}
	for _, tool := range tools {
		t.Run(tool, func(t *testing.T) {
			execInSandbox(t, s, "sh", "-ec", `command -v "$1"`, "sh", tool)
		})
	}
}
