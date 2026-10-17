package sandbox_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/require"
)

func TestMissingUserConfigLeavesOpenCodeDefaultsUnchanged(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	config, err := sandbox.LoadConfig("")

	require.NoError(t, err)
	require.Nil(t, config)
	s := prepareSandboxWithConfig(t, newWorktree(t), config)
	execInSandbox(t, s, "test", "!", "-e", "/etc/opencode/opencode.json")
}

func TestUserCanProvideSandboxConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "opencode-sandbox", "opencode.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	contents := []byte(`{"model":"openai/gpt-6.1-sol"}`)
	require.NoError(t, os.WriteFile(path, contents, 0600))

	config, err := sandbox.LoadConfig("")

	require.NoError(t, err)
	require.Equal(t, contents, config)
}

func TestSandboxConfigDefaultsToHomeConfigDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	path := filepath.Join(home, ".config", "opencode-sandbox", "opencode.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	require.NoError(t, os.WriteFile(path, []byte(`{}`), 0600))

	config, err := sandbox.LoadConfig("")

	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(config))
}

func TestExplicitConfigReplacesUserConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	userPath := filepath.Join(dir, "opencode-sandbox", "opencode.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(userPath), 0700))
	require.NoError(t, os.WriteFile(userPath, []byte(`{"model":"other/model"}`), 0600))
	explicitPath := filepath.Join(dir, "custom.json")
	contents := []byte(`{"model":"openai/gpt-6.1-sol"}`)
	require.NoError(t, os.WriteFile(explicitPath, contents, 0600))

	config, err := sandbox.LoadConfig(explicitPath)

	require.NoError(t, err)
	require.Equal(t, contents, config)
}

func TestInvalidOrMissingExplicitConfigIsRejected(t *testing.T) {
	cases := []struct {
		name     string
		contents string
	}{
		{name: "empty", contents: ""},
		{name: "malformed", contents: "{broken"},
		{name: "array", contents: "[]"},
		{name: "null", contents: "null"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			require.NoError(t, os.WriteFile(path, []byte(tc.contents), 0600))
			_, err := sandbox.LoadConfig(path)
			require.Error(t, err)
		})
	}
	_, err := sandbox.LoadConfig(filepath.Join(t.TempDir(), "missing.json"))
	require.Error(t, err)
}

func TestGuestConfigUpdatesOnEveryPreparation(t *testing.T) {
	worktree := newWorktree(t)
	config := []byte(`{"model":"openai/gpt-6.1-sol"}`)
	s := prepareSandboxWithConfig(t, worktree, config)
	require.JSONEq(t, string(config), execInSandbox(t, s, "cat", "/etc/opencode/opencode.json"))
	_, err := os.Stat(filepath.Join(worktree, "opencode.json"))
	require.True(t, os.IsNotExist(err), "config must not be written into the shared checkout")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	updated := []byte(`{"model":"openai/gpt-6-luna"}`)
	resumed, err := sandbox.Prepare(ctx, worktree, updated, nil, testSettings(t))
	require.NoError(t, err)
	defer resumed.Close()
	require.JSONEq(t, string(updated), execInSandbox(t, resumed, "cat", "/etc/opencode/opencode.json"))
}

func TestRemovingCustomConfigClearsManagedGuestConfig(t *testing.T) {
	worktree := newWorktree(t)
	s := prepareSandboxWithConfig(t, worktree, []byte(`{"model":"openai/gpt-6.1-sol"}`))
	execInSandbox(t, s, "test", "-f", "/etc/opencode/opencode.json")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	withoutConfig, err := sandbox.Prepare(ctx, worktree, nil, nil, testSettings(t))
	require.NoError(t, err)
	defer withoutConfig.Close()
	execInSandbox(t, withoutConfig, "test", "!", "-e", "/etc/opencode/opencode.json")
}

func TestNoCustomConfigPreservesUnmanagedGuestConfig(t *testing.T) {
	s := startSandbox(t)
	execInSandbox(t, s, "bash", "-eu", "-c", `mkdir -p /etc/opencode; printf '{}' > /etc/opencode/opencode.json`)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	resumed, err := sandbox.Prepare(ctx, s.Worktree, nil, nil, testSettings(t))

	require.NoError(t, err)
	defer resumed.Close()
	require.JSONEq(t, `{}`, execInSandbox(t, resumed, "cat", "/etc/opencode/opencode.json"))
}
