package sandbox_test

import "testing"

func TestDefaultToolsAreAvailable(t *testing.T) {
	s := startSandbox(t)
	tools := []string{"mise", "node", "git", "gh", "aws", "opencode", "docker", "dockerd"}
	for _, tool := range tools {
		t.Run(tool, func(t *testing.T) {
			execInSandbox(t, s, tool, "--version")
		})
	}
}
