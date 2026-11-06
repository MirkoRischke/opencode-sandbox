package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"opencode-sandbox/internal/workspace"

	"github.com/stretchr/testify/require"
	m "github.com/superradcompany/microsandbox/sdk/go"
)

func TestOpenCodeStorageUsesXDGAndPreservesExistingData(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state) //nolint:forbidigo // Exercise host storage discovery; Setenv prevents parallel execution.
	w, err := workspace.Resolve(context.Background(), t.TempDir())
	require.NoError(t, err)
	dir, err := prepareOpenCodeStorage(w)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(state, "opencode-sandbox", "opencode", w.RepositoryID()), dir)
	info, err := os.Stat(dir)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0700), info.Mode().Perm())
	database := filepath.Join(dir, "opencode.db")
	require.NoError(t, os.WriteFile(database, []byte("existing history"), 0600))
	again, err := prepareOpenCodeStorage(w)
	require.NoError(t, err)
	require.Equal(t, dir, again)
	contents, err := os.ReadFile(database)
	require.NoError(t, err)
	require.Equal(t, "existing history", string(contents))
}

func TestOpenCodeStorageFallsBackToHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "") //nolint:forbidigo // Exercise host storage discovery; Setenv prevents parallel execution.
	home := t.TempDir()
	t.Setenv("HOME", home) //nolint:forbidigo // Keep host state inside the test directory.
	w, err := workspace.Resolve(context.Background(), t.TempDir())
	require.NoError(t, err)
	dir, err := prepareOpenCodeStorage(w)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, ".local", "state", "opencode-sandbox", "opencode", w.ID()), dir)
}

func TestOpenCodeStoragePreservesCreationErrors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0600))
	t.Setenv("XDG_STATE_HOME", file) //nolint:forbidigo // Exercise host storage errors; Setenv prevents parallel execution.
	w, err := workspace.Resolve(context.Background(), t.TempDir())
	require.NoError(t, err)
	_, err = prepareOpenCodeStorage(w)
	var pathError *os.PathError
	require.ErrorAs(t, err, &pathError)
	require.ErrorContains(t, err, "create OpenCode host storage")
}

func TestOpenCodeMountIsWritableAndIncludesSQLiteSidecars(t *testing.T) {
	t.Parallel()
	source := t.TempDir()
	mounts := map[string]m.MountConfig{}
	addOpenCodeMount(mounts, source)
	require.Len(t, mounts, 1)
	mount := mounts[openCodeDatabaseDir]
	require.Equal(t, source, mount.Bind)
	require.False(t, mount.Readonly)
	require.Equal(t, &m.MountOwner{UID: 0, GID: 0}, mount.Owner)
	require.Equal(t, openCodeDatabaseDir, filepath.Dir(openCodeDatabase))
}
