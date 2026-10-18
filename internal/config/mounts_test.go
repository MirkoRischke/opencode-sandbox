package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMountLayersResolveOriginsAndReplaceLists(t *testing.T) {
	t.Parallel()
	global, repo := t.TempDir(), t.TempDir()
	settings, err := mergeSettings(DefaultSettings(), []byte("[[mounts]]\nsource = 'skills'\ntarget = '/opt/skills'\n"), global)
	require.NoError(t, err)
	require.Equal(t, []MountSettings{{Source: filepath.Join(global, "skills"), Target: "/opt/skills"}}, settings.Mounts)
	inherited, err := mergeSettings(settings, []byte("[vm]\ncpus = 1\n"), repo)
	require.NoError(t, err)
	require.Equal(t, settings.Mounts, inherited.Mounts)
	replaced, err := mergeSettings(settings, []byte("[[mounts]]\nsource = 'AGENTS.md'\ntarget = '/opt/AGENTS.md'\n"), repo)
	require.NoError(t, err)
	require.Equal(t, []MountSettings{{Source: filepath.Join(repo, "AGENTS.md"), Target: "/opt/AGENTS.md"}}, replaced.Mounts)
	require.Equal(t, filepath.Join(global, "skills"), settings.Mounts[0].Source)
	cleared, err := mergeSettings(settings, []byte("mounts = []\n"), repo)
	require.NoError(t, err)
	require.Empty(t, cleared.Mounts)
}

func TestMountHomeExpansion(t *testing.T) {
	t.Parallel()
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	settings, err := mergeSettings(DefaultSettings(), []byte("[[mounts]]\nsource = '~/.config/opencode/skills'\ntarget = '/opt/skills'\n"), t.TempDir())
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, ".config/opencode/skills"), settings.Mounts[0].Source)
}

func TestMountValidation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		mounts []MountSettings
	}{
		{"missing source", []MountSettings{{Target: "/opt/file"}}},
		{"relative source", []MountSettings{{Source: "file", Target: "/opt/file"}}},
		{"relative target", []MountSettings{{Source: "/host/file", Target: "opt/file"}}},
		{"root target", []MountSettings{{Source: "/host/file", Target: "/"}}},
		{"unclean target", []MountSettings{{Source: "/host/file", Target: "/opt/../file"}}},
		{"duplicate target", []MountSettings{{Source: "/host/a", Target: "/opt/file"}, {Source: "/host/b", Target: "/opt/file"}}},
		{"nested target", []MountSettings{{Source: "/host/a", Target: "/opt/skills"}, {Source: "/host/b", Target: "/opt/skills/file"}}},
		{"parent target", []MountSettings{{Source: "/host/a", Target: "/opt/skills/file"}, {Source: "/host/b", Target: "/opt/skills"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings := DefaultSettings()
			settings.Mounts = tc.mounts
			require.ErrorContains(t, settings.Validate(), "mounts[")
		})
	}
	require.False(t, MountTargetsOverlap("/opt/skills", "/opt/skills-other"))
	_, err := mergeSettings(DefaultSettings(), []byte("[[mounts]]\nsource = '/host/file'\ntarget = '/opt/file'\nreadonly = false\n"), t.TempDir())
	require.ErrorContains(t, err, "unknown launcher setting")
}

func TestMountSourcesMustExist(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	require.NoError(t, os.WriteFile(file, []byte("contents"), 0600))
	settings := DefaultSettings()
	settings.Mounts = []MountSettings{{Source: file, Target: "/opt/file"}, {Source: dir, Target: "/opt/directory"}}
	require.NoError(t, settings.ValidateMountSources())
	require.NoError(t, os.Remove(file))
	err := settings.ValidateMountSources()
	require.ErrorContains(t, err, "mounts[0].source")
	require.ErrorContains(t, err, file)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestRepoMountApprovalUsesResolvedPaths(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	output, err := exec.Command("git", "-C", root, "init").CombinedOutput()
	require.NoError(t, err, "%s", output)
	source := filepath.Join(root, "AGENTS.md")
	require.NoError(t, os.WriteFile(source, []byte("instructions"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(root, workspaceConfigName), []byte("[[mounts]]\nsource = 'AGENTS.md'\ntarget = '/opt/AGENTS.md'\n"), 0600))
	paths := Paths{ConfigDir: t.TempDir(), StateDir: t.TempDir()}
	_, err = paths.LoadWorkspaceSettings(root, nil)
	require.ErrorContains(t, err, "untrusted workspace config")
	approved := false
	_, err = paths.LoadWorkspaceSettings(root, func(_ string, _ []byte, settings Settings) error {
		approved = true
		require.Equal(t, source, settings.Mounts[0].Source)
		return nil
	})
	require.NoError(t, err)
	require.True(t, approved)
	require.NoError(t, os.Remove(source))
	_, err = paths.LoadWorkspaceSettings(root, nil)
	require.ErrorContains(t, err, fmt.Sprintf("mounts[0].source %q", source))
}
