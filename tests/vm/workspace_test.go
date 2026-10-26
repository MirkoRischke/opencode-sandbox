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

func TestNonGitWorkspaceSnapshotAndResume(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	settings := testSettings(t)
	settings.Snapshot.Command = "echo prepared > /root/workspace-baseline; echo host-change > prepared-on-host"
	t.Cleanup(func() { require.NoError(t, sandbox.RemoveSnapshot(context.Background(), root)) })
	_, err := sandbox.CreateSnapshot(ctx, root, nil, settings)
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(root, "prepared-on-host"))
	s := prepareSnapshotChild(t, ctx, root, settings)
	require.Equal(t, "prepared\n", execInSandbox(t, s, "cat", "/root/workspace-baseline"))
	execInSandbox(t, s, "bash", "-eu", "-c", "echo writable > guest-file; echo persistent > /root/workspace-persistent")
	require.FileExists(t, filepath.Join(root, "guest-file"))
	require.NoError(t, s.Stop(ctx))
	resumed, err := sandbox.Prepare(ctx, root, nil, settings)
	require.NoError(t, err)
	defer func() { require.NoError(t, resumed.Close()) }()
	require.Equal(t, s.Name, resumed.Name)
	require.Equal(t, "persistent\n", execInSandbox(t, resumed, "cat", "/root/workspace-persistent"))
	listing, err := sandbox.ListSnapshots(ctx, root)
	require.NoError(t, err)
	require.Len(t, listing.Current.Snapshots, 1)
	require.NoError(t, sandbox.Remove(ctx, root))
}

func TestCheckoutAndSubdirectoryMountScope(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runGit(t, "init", root)
	require.NoError(t, os.WriteFile(filepath.Join(root, "parent-only"), []byte("private"), 0600))
	checkout := prepareSandbox(t, root)
	execInSandbox(t, checkout, "git", "status", "--porcelain")
	subdir := filepath.Join(root, "nested")
	require.NoError(t, os.Mkdir(subdir, 0700))
	nested := prepareSandbox(t, subdir)
	require.NotEqual(t, checkout.Name, nested.Name)
	execInSandbox(t, nested, "bash", "-eu", "-c", "test ! -e ../parent-only; ! git rev-parse --show-toplevel >/dev/null 2>&1; echo scoped > guest-file")
	require.FileExists(t, filepath.Join(subdir, "guest-file"))
}
