package sandbox_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"opencode-sandbox/internal/config"
	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/require"
)

func TestCopiedFilesAreWritableAndNotRefreshedOnResume(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	source := t.TempDir()
	gitconfig := filepath.Join(source, "gitconfig")
	require.NoError(t, os.WriteFile(gitconfig, []byte("[user]\nname = Sandbox Test\n"), 0600))
	tree := filepath.Join(source, "tree with ' spaces")
	require.NoError(t, os.MkdirAll(filepath.Join(tree, "nested"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(tree, "nested", ".file"), []byte("host"), 0600))
	require.NoError(t, os.Mkdir(filepath.Join(tree, "empty"), 0700))
	require.NoError(t, os.Symlink("nested/.file", filepath.Join(tree, "link")))
	settings := config.DefaultSettings()
	settings.VM.CPUs, settings.VM.MemoryMiB = 1, 768
	settings.Files = []config.FileSettings{
		{Source: gitconfig, Target: "/root/.gitconfig", Mode: "copy"},
		{Source: tree, Target: "/opt/copy's directory", Mode: "copy"},
	}
	s := prepareSandboxWithSettings(t, newWorktree(t), settings)
	require.Equal(t, "host", execInSandbox(t, s, "cat", "/opt/copy's directory/nested/.file"))
	execInSandbox(t, s, "test", "-d", "/opt/copy's directory/empty")
	require.Equal(t, "nested/.file\n", execInSandbox(t, s, "readlink", "/opt/copy's directory/link"))
	// The original failure: gh must be able to write to the copied global config.
	execInSandbox(t, s, "env", "GH_TOKEN=synthetic-test-token", "gh", "auth", "setup-git", "--hostname", "github.com")
	execInSandbox(t, s, "bash", "-eu", "-c", `printf guest > "/opt/copy's directory/nested/.file"`)
	host, err := os.ReadFile(filepath.Join(tree, "nested", ".file"))
	require.NoError(t, err)
	require.Equal(t, "host", string(host))
	host, err = os.ReadFile(gitconfig)
	require.NoError(t, err)
	require.NotContains(t, string(host), "credential")
	require.NoError(t, s.Shutdown())
	require.NoError(t, os.WriteFile(filepath.Join(tree, "nested", ".file"), []byte("new host"), 0600))
	resumed, err := sandbox.Prepare(ctx, s.Worktree, nil, settings)
	require.NoError(t, err)
	defer func() { require.NoError(t, resumed.Shutdown()) }()
	require.Equal(t, "guest", execInSandbox(t, resumed, "cat", "/opt/copy's directory/nested/.file"))
}

func TestSnapshotCopiesMergeCurrentHostContentsIntoExactTarget(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	worktree := newWorktree(t)
	source := t.TempDir()
	tree := filepath.Join(source, "different-basename")
	require.NoError(t, os.MkdirAll(filepath.Join(tree, "nested"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(tree, "nested", "matching"), []byte("old host"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(tree, "removed-on-host"), []byte("keep from snapshot"), 0600))
	file := filepath.Join(source, "single")
	require.NoError(t, os.WriteFile(file, []byte("old single file"), 0600))
	settings := config.DefaultSettings()
	settings.VM.CPUs, settings.VM.MemoryMiB = 1, 768
	settings.VM.RootDiskMiB, settings.VM.DockerDiskMiB = 512, 256
	settings.Files = []config.FileSettings{
		{Source: tree, Target: "/opt/exact-target", Mode: "copy"},
		{Source: file, Target: "/opt/single", Mode: "copy"},
	}
	settings.Snapshot.Command = `
test "$(cat /opt/exact-target/nested/matching)" = 'old host'
test "$(cat /opt/single)" = 'old single file'
printf snapshot-only > /opt/exact-target/guest-only
`
	t.Cleanup(func() {
		cleanup, done := context.WithTimeout(context.Background(), time.Minute)
		defer done()
		require.NoError(t, sandbox.RemoveSnapshot(cleanup, worktree))
	})
	_, err := sandbox.CreateSnapshot(ctx, worktree, nil, settings)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(tree, "nested", "matching"), []byte("current host"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(tree, "new"), []byte("new file"), 0600))
	require.NoError(t, os.Remove(filepath.Join(tree, "removed-on-host")))
	require.NoError(t, os.WriteFile(file, []byte("new"), 0600))
	child := prepareSnapshotChild(t, ctx, worktree, settings)
	require.Equal(t, "current host", execInSandbox(t, child, "cat", "/opt/exact-target/nested/matching"))
	require.Equal(t, "new file", execInSandbox(t, child, "cat", "/opt/exact-target/new"))
	require.Equal(t, "snapshot-only", execInSandbox(t, child, "cat", "/opt/exact-target/guest-only"))
	require.Equal(t, "keep from snapshot", execInSandbox(t, child, "cat", "/opt/exact-target/removed-on-host"))
	require.Equal(t, "new", execInSandbox(t, child, "cat", "/opt/single"))
	execInSandbox(t, child, "test", "!", "-e", "/opt/exact-target/different-basename")
	// Copies must not appear as read-only host mounts in generated instructions.
	require.NotContains(t, execInSandbox(t, child, "cat", guestInstructions), `"/opt/exact-target"`)
}

func TestFailedCopyDiscardsNewVM(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	worktree := newWorktree(t)
	t.Cleanup(func() { require.NoError(t, sandbox.Remove(context.Background(), worktree)) })
	source := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(source, "file"), []byte("retry"), 0600))
	settings := config.DefaultSettings()
	settings.VM.CPUs, settings.VM.MemoryMiB = 1, 768
	// A directory cannot be merged onto an image's existing regular file.
	settings.Files = []config.FileSettings{{Source: source, Target: "/etc/debian_version", Mode: "copy"}}
	_, err := sandbox.Prepare(ctx, worktree, nil, settings)
	require.ErrorContains(t, err, "copy files[0]")
	require.ErrorContains(t, err, "merge guest copy")
	settings.Files[0].Target = "/opt/retry"
	retried := prepareSandboxWithSettings(t, worktree, settings)
	// A leftover failed VM would resume without performing this new copy.
	require.Equal(t, "retry", execInSandbox(t, retried, "cat", "/opt/retry/file"))
}
