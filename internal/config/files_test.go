package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFileLayersResolveOriginsAndReplaceLists(t *testing.T) {
	t.Parallel()
	global, repo := t.TempDir(), t.TempDir()
	settings, err := mergeSettings(DefaultSettings(), []byte("[[files]]\nsource = 'skills'\ntarget = '/opt/skills'\n"), global)
	require.NoError(t, err)
	require.Equal(t, []FileSettings{{Source: filepath.Join(global, "skills"), Target: "/opt/skills"}}, settings.Files)
	require.False(t, settings.Files[0].IsCopy())
	inherited, err := mergeSettings(settings, []byte("[vm]\ncpus = 1\n"), repo)
	require.NoError(t, err)
	require.Equal(t, settings.Files, inherited.Files)
	replaced, err := mergeSettings(settings, []byte("[[files]]\nsource = 'AGENTS.md'\ntarget = '/opt/AGENTS.md'\nmode = 'copy'\n"), repo)
	require.NoError(t, err)
	require.Equal(t, []FileSettings{{Source: filepath.Join(repo, "AGENTS.md"), Target: "/opt/AGENTS.md", Mode: "copy"}}, replaced.Files)
	require.True(t, replaced.Files[0].IsCopy())
	require.Equal(t, filepath.Join(global, "skills"), settings.Files[0].Source)
	cleared, err := mergeSettings(settings, []byte("files = []\n"), repo)
	require.NoError(t, err)
	require.Empty(t, cleared.Files)
}

func TestFileHomeExpansion(t *testing.T) {
	t.Parallel()
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	settings, err := mergeSettings(DefaultSettings(), []byte("[[files]]\nsource = '~/.config/opencode/skills'\ntarget = '/opt/skills'\nmode = 'mount'\n"), t.TempDir())
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, ".config/opencode/skills"), settings.Files[0].Source)
	require.False(t, settings.Files[0].IsCopy())
}

func TestFileValidation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		files []FileSettings
	}{
		{"missing source", []FileSettings{{Target: "/opt/file"}}},
		{"relative source", []FileSettings{{Source: "file", Target: "/opt/file"}}},
		{"relative target", []FileSettings{{Source: "/host/file", Target: "opt/file"}}},
		{"root target", []FileSettings{{Source: "/host/file", Target: "/"}}},
		{"unclean target", []FileSettings{{Source: "/host/file", Target: "/opt/../file"}}},
		{"invalid mode", []FileSettings{{Source: "/host/file", Target: "/opt/file", Mode: "write"}}},
		{"duplicate target", []FileSettings{{Source: "/host/a", Target: "/opt/file"}, {Source: "/host/b", Target: "/opt/file", Mode: "copy"}}},
		{"nested target", []FileSettings{{Source: "/host/a", Target: "/opt/skills"}, {Source: "/host/b", Target: "/opt/skills/file"}}},
		{"parent target", []FileSettings{{Source: "/host/a", Target: "/opt/skills/file"}, {Source: "/host/b", Target: "/opt/skills"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings := DefaultSettings()
			settings.Files = tc.files
			require.ErrorContains(t, settings.Validate(), "files[")
		})
	}
	require.False(t, MountTargetsOverlap("/opt/skills", "/opt/skills-other"))
	for _, data := range []string{
		"[[files]]\nsource = '/host/file'\ntarget = '/opt/file'\nreadonly = false\n",
		"[[mounts]]\nsource = '/host/file'\ntarget = '/opt/file'\n",
	} {
		_, err := mergeSettings(DefaultSettings(), []byte(data), t.TempDir())
		require.ErrorContains(t, err, "unknown launcher setting")
	}
}

func TestFileSourcesMustExist(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	require.NoError(t, os.WriteFile(file, []byte("contents"), 0600))
	settings := DefaultSettings()
	settings.Files = []FileSettings{{Source: file, Target: "/opt/file", Mode: "copy"}, {Source: dir, Target: "/opt/directory"}}
	require.NoError(t, settings.ValidateFileSources())
	require.NoError(t, os.Remove(file))
	err := settings.ValidateFileSources()
	require.ErrorContains(t, err, "files[0].source")
	require.ErrorContains(t, err, file)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestRepoFileApprovalUsesResolvedPaths(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	output, err := exec.Command("git", "-C", root, "init").CombinedOutput()
	require.NoError(t, err, "%s", output)
	source := filepath.Join(root, "AGENTS.md")
	require.NoError(t, os.WriteFile(source, []byte("instructions"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(root, workspaceConfigName), []byte("[[files]]\nsource = 'AGENTS.md'\ntarget = '/opt/AGENTS.md'\nmode = 'copy'\n"), 0600))
	paths := Paths{ConfigDir: t.TempDir(), StateDir: t.TempDir()}
	_, err = paths.LoadWorkspaceSettings(root, nil)
	require.ErrorContains(t, err, "untrusted workspace config")
	approved := false
	_, err = paths.LoadWorkspaceSettings(root, func(_ string, _ []byte, settings Settings) error {
		approved = true
		require.Equal(t, source, settings.Files[0].Source)
		return nil
	})
	require.NoError(t, err)
	require.True(t, approved)
	require.NoError(t, os.Remove(source))
	_, err = paths.LoadWorkspaceSettings(root, nil)
	require.ErrorContains(t, err, fmt.Sprintf("files[0].source %q", source))
}
