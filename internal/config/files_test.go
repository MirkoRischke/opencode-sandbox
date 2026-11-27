package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFileLayersResolveOriginsAndAppendLists(t *testing.T) {
	t.Parallel()
	global, repo := t.TempDir(), t.TempDir()
	settings, err := mergeSettings(DefaultSettings(), []byte("[[files]]\nsource = 'skills'\ntarget = '/opt/skills'\n"), global)
	require.NoError(t, err)
	require.Equal(t, []FileSettings{{Source: filepath.Join(global, "skills"), Target: "/opt/skills"}}, settings.Files)
	require.False(t, settings.Files[0].IsCopy())
	inherited, err := mergeSettings(settings, []byte("[vm]\ncpus = 1\n"), repo)
	require.NoError(t, err)
	require.Equal(t, settings.Files, inherited.Files)
	added, err := mergeSettings(settings, []byte("[[files]]\nsource = 'AGENTS.md'\ntarget = '/opt/AGENTS.md'\nmode = 'copy'\n"), repo)
	require.NoError(t, err)
	require.Equal(t, []FileSettings{settings.Files[0], {Source: filepath.Join(repo, "AGENTS.md"), Target: "/opt/AGENTS.md", Mode: "copy"}}, added.Files)
	require.True(t, added.Files[1].IsCopy())
	require.Equal(t, filepath.Join(global, "skills"), settings.Files[0].Source)
	empty, err := mergeSettings(settings, []byte("files = []\n"), repo)
	require.NoError(t, err)
	require.Equal(t, settings.Files, empty.Files)
	_, err = mergeSettings(settings, []byte("[[files]]\nsource = '.'\ntarget = '/opt/skills/nested'\n"), repo)
	require.ErrorContains(t, err, "overlaps")
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
	require.ErrorContains(t, err, "untrusted config")
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

func TestWritableMountRequiresOwnWorkspaceConfigAndDirectory(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	root := filepath.Join(parent, "repo")
	require.NoError(t, os.Mkdir(root, 0700))
	source := filepath.Join(parent, "shared")
	require.NoError(t, os.Mkdir(source, 0700))
	data := []byte("[[files]]\nsource = '../shared'\ntarget = '/mnt/shared'\nmode = 'mount-rw'\n")
	paths := Paths{ConfigDir: t.TempDir(), StateDir: t.TempDir()}
	global := filepath.Join(paths.ConfigDir, "opencode-sandbox")
	require.NoError(t, os.Mkdir(global, 0700))
	for _, path := range []string{filepath.Join(global, "config.toml"), filepath.Join(parent, workspaceConfigName)} {
		require.NoError(t, os.WriteFile(path, data, 0600))
		_, err := paths.ValidateWorkspaceSettings(root)
		require.ErrorContains(t, err, "mount-rw is only allowed in the workspace's own")
		require.NoError(t, os.Remove(path))
	}

	require.NoError(t, os.WriteFile(filepath.Join(root, workspaceConfigName), data, 0600))
	_, err := paths.LoadWorkspaceSettings(root, nil)
	require.ErrorContains(t, err, "untrusted config")
	settings, err := paths.LoadWorkspaceSettings(root, func(_ string, _ []byte, settings Settings) error {
		require.Equal(t, FileSettings{Source: source, Target: "/mnt/shared", Mode: "mount-rw"}, settings.Files[0])
		return nil
	})
	require.NoError(t, err)
	require.True(t, settings.Files[0].IsWritableMount())

	require.NoError(t, os.Remove(source))
	require.NoError(t, os.WriteFile(source, nil, 0600))
	_, err = paths.LoadWorkspaceSettings(root, nil)
	require.ErrorContains(t, err, "must be a directory for mount-rw")
}
