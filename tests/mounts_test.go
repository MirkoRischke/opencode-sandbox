package sandbox_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	launcherconfig "opencode-sandbox/internal/config"
	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/require"
)

func TestReadOnlyFileAndDirectoryMounts(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	config := filepath.Join(dir, "cli.json")
	require.NoError(t, os.WriteFile(config, []byte(`{"mouse":false}`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "host-only"), []byte("private"), 0600))
	skills := filepath.Join(dir, "skills")
	require.NoError(t, os.MkdirAll(filepath.Join(skills, "example"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(skills, "example", "SKILL.md"), []byte("skill contents"), 0600))
	settings := testSettings(t)
	settings.Mounts = []launcherconfig.MountSettings{
		{Source: config, Target: "/root/.config/opencode/cli.json"},
		{Source: skills, Target: "/root/.config/opencode/skills"},
	}
	s := prepareSandboxWithSettings(t, newWorktree(t), settings)
	require.Equal(t, `{"mouse":false}`, execInSandbox(t, s, "cat", "/root/.config/opencode/cli.json"))
	require.Equal(t, "skill contents", execInSandbox(t, s, "cat", "/root/.config/opencode/skills/example/SKILL.md"))
	execInSandbox(t, s, "test", "!", "-e", "/root/.config/opencode/host-only")
	execInSandbox(t, s, "bash", "-eu", "-c", `
if printf 'changed' > /root/.config/opencode/cli.json; then exit 1; fi
if touch /root/.config/opencode/skills/new; then exit 1; fi
if rm /root/.config/opencode/skills/example/SKILL.md; then exit 1; fi
`)
	contents, err := os.ReadFile(config)
	require.NoError(t, err)
	require.Equal(t, `{"mouse":false}`, string(contents))
	// An editor may atomically replace the host file rather than updating it in place.
	replacement := filepath.Join(dir, "replacement")
	require.NoError(t, os.WriteFile(replacement, []byte(`{"mouse":true}`), 0600))
	require.NoError(t, os.Rename(replacement, config))
	require.Equal(t, `{"mouse":true}`, execInSandbox(t, s, "cat", "/root/.config/opencode/cli.json"))
	require.NoError(t, os.WriteFile(filepath.Join(skills, "new"), []byte("new skill"), 0600))
	require.Equal(t, "new skill", execInSandbox(t, s, "cat", "/root/.config/opencode/skills/new"))
	// The mappings also survive stop/start without any copying.
	require.NoError(t, s.Shutdown())
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	resumed, err := sandbox.Prepare(ctx, s.Worktree, nil, settings)
	require.NoError(t, err)
	defer func() { require.NoError(t, resumed.Shutdown()) }()
	require.Equal(t, `{"mouse":true}`, execInSandbox(t, resumed, "cat", "/root/.config/opencode/cli.json"))
}

func TestNoImplicitOpenCodeMounts(t *testing.T) {
	t.Parallel()
	s := startSandbox(t)
	for _, target := range []string{"/etc/opencode/opencode.json", "/root/.config/opencode/cli.json", "/root/.config/opencode/AGENTS.md", "/root/.config/opencode/skills"} {
		execInSandbox(t, s, "test", "!", "-e", target)
	}
}
