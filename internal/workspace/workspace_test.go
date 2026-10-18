package workspace

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func git(t *testing.T, args ...string) {
	t.Helper()
	out, err := exec.Command("git", args...).CombinedOutput()
	require.NoError(t, err, "%s", out)
}

func resolve(t *testing.T, path string) Workspace {
	t.Helper()
	w, err := Resolve(context.Background(), path)
	require.NoError(t, err)
	return w
}

func TestWorkspaceTypesAndMounts(t *testing.T) {
	t.Parallel()
	parent := resolve(t, t.TempDir()).Root()
	repo, tree := filepath.Join(parent, "repo"), filepath.Join(parent, "tree")
	git(t, "init", "--initial-branch=main", repo)
	git(t, "-C", repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-m", "initial")
	git(t, "-C", repo, "worktree", "add", "-b", "other", tree)
	plain := resolve(t, parent)
	require.IsType(t, Directory{}, plain)
	require.Equal(t, []Mount{{Source: parent, Target: "/workspace"}}, plain.Mounts("/workspace"))
	checkout := resolve(t, repo)
	require.IsType(t, GitRepository{}, checkout)
	require.Equal(t, []Mount{{Source: repo, Target: repo}}, checkout.Mounts(repo))
	common := filepath.Join(repo, ".git")
	require.Equal(t, []Mount{{Source: repo, Target: "/workspace"}, {Source: common, Target: common}}, checkout.Mounts("/workspace"))
	linked := resolve(t, tree)
	require.IsType(t, GitWorktree{}, linked)
	require.Equal(t, []Mount{{Source: tree, Target: tree}, {Source: common, Target: common}}, linked.Mounts(tree))
	require.Equal(t, []Mount{{Source: tree, Target: "/workspace"}, {Source: common, Target: common}}, linked.Mounts("/workspace"))
	require.NotEqual(t, checkout.ID(), linked.ID())
	for _, root := range []string{repo, tree} {
		subdir := filepath.Join(root, "subdir")
		require.NoError(t, os.Mkdir(subdir, 0700))
		w := resolve(t, subdir)
		require.IsType(t, Directory{}, w)
		require.Equal(t, subdir, w.Root())
		require.Equal(t, []Mount{{Source: subdir, Target: subdir}}, w.Mounts(subdir))
	}
}

func TestCanonicalIdentityAndFreshMountSlices(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	root := filepath.Join(parent, "project")
	require.NoError(t, os.Mkdir(root, 0700))
	alias := filepath.Join(parent, "alias")
	require.NoError(t, os.Symlink(root, alias))
	w := resolve(t, root)
	other := resolve(t, alias)
	require.Equal(t, w.Root(), other.Root())
	require.Equal(t, w.ID(), other.ID())
	require.Len(t, w.ID(), 64)
	mounts := w.Mounts("/workspace")
	mounts[0].Source = "/changed"
	require.Equal(t, w.Root(), w.Mounts("/workspace")[0].Source)
}

func TestInvalidGitMetadataIsNotPlainDirectory(t *testing.T) {
	t.Parallel()
	for _, contents := range []string{"broken", "gitdir: /nonexistent/opencode-test-git-dir"} {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, ".git"), []byte(contents), 0600))
		_, err := Resolve(context.Background(), root)
		require.ErrorContains(t, err, "Git metadata")
	}
}

func TestSeparateGitDirectory(t *testing.T) {
	t.Parallel()
	parent := resolve(t, t.TempDir()).Root()
	root, metadata := filepath.Join(parent, "project"), filepath.Join(parent, "metadata")
	git(t, "init", "--separate-git-dir", metadata, root)
	w := resolve(t, root)
	require.IsType(t, GitRepository{}, w)
	require.Equal(t, []Mount{{Source: root, Target: root}, {Source: metadata, Target: metadata}}, w.Mounts(root))
}

func TestGitEnvironmentCannotRedirectDiscovery(t *testing.T) {
	root := t.TempDir()
	git(t, "init", root)
	t.Setenv("GIT_DIR", "/does/not/exist")       //nolint:forbidigo // Exercise process-environment sanitization; Setenv prevents parallel execution.
	t.Setenv("GIT_WORK_TREE", "/does/not/exist") //nolint:forbidigo // Exercise process-environment sanitization; Setenv prevents parallel execution.
	require.IsType(t, GitRepository{}, resolve(t, root))
}

func TestUnsafePaths(t *testing.T) {
	t.Parallel()
	home := "/home/alice"
	for _, root := range []string{
		"/", "/home", home, "/home/bob", "/Users/bob", "/root", "/Users/Shared",
		"/tmp", "/private/tmp", "/var/tmp", "/private/var/tmp", "/Volumes", "/mnt", "/media",
		"/opt", "/srv", "/var", "/private/var", "/private", "/System/Library", "/Library", "/Applications",
		"/etc", "/etc/ssh", "/private/etc/ssh", "/dev", "/proc/1", "/sys", "/run/user", "/boot", "/usr/local", "/bin", "/sbin",
	} {
		require.True(t, protectedPath(root, home), root)
	}
	for _, root := range []string{home + "/project", "/tmp/project", "/private/tmp/project", "/Volumes/Drive/project", "/mnt/drive/project", "/opt/project", "/srv/project", "/etcetera/project"} {
		require.False(t, protectedPath(root, home), root)
	}
	require.True(t, protectedPath("/custom", "/custom/alice"))
}

func TestResolveRejectsProtectedDirectoriesAndAliases(t *testing.T) {
	t.Parallel()
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	for _, root := range []string{"/", home, "/var", "/tmp", "/etc"} {
		t.Run(root, func(t *testing.T) {
			t.Parallel()
			if _, err := os.Stat(root); os.IsNotExist(err) {
				t.Skip("system directory does not exist on this host")
			} else {
				require.NoError(t, err)
			}
			alias := filepath.Join(t.TempDir(), "alias")
			require.NoError(t, os.Symlink(root, alias))
			for _, path := range []string{root, alias} {
				_, err := Resolve(context.Background(), path)
				require.ErrorContains(t, err, "unsafe workspace directory")
			}
		})
	}
}

func TestMountRootDetection(t *testing.T) {
	t.Parallel()
	data := []byte("36 25 8:1 / /mnt/drive rw - ext4 /dev/sda1 rw\n37 25 8:1 / /mnt/My\\040Drive rw - ext4 /dev/sda1 rw\n")
	require.True(t, isMountRoot("/mnt/drive", data))
	require.True(t, isMountRoot("/mnt/My Drive", data))
	require.False(t, isMountRoot("/mnt/drive/project", data))
}
