package sandbox

import (
	"fmt"
	"os"
	"path/filepath"

	"opencode-sandbox/internal/workspace"

	m "github.com/superradcompany/microsandbox/sdk/go"
)

const openCodeDatabaseDir = "/var/lib/opencode-sandbox/opencode"
const openCodeDatabase = openCodeDatabaseDir + "/opencode.db"

// Only workspace VMs mount history. Disposable preparation VMs and baselines
// never access or capture the workspace's live OpenCode database.
func prepareOpenCodeStorage(w workspace.Workspace) (string, error) {
	stateDir := os.Getenv("XDG_STATE_HOME")
	if stateDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("locate OpenCode host storage: %w", err)
		}
		stateDir = filepath.Join(home, ".local", "state")
	}
	dir := filepath.Join(stateDir, "opencode-sandbox", "opencode", w.ID())
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create OpenCode host storage: %w", err)
	}
	return dir, nil
}

func addOpenCodeMount(mounts map[string]m.MountConfig, source string) {
	// Mount the directory, not only the DB: SQLite also writes WAL/SHM sidecars.
	mounts[openCodeDatabaseDir] = m.Mount.Bind(source, m.MountOptions{
		Owner: &m.MountOwner{UID: 0, GID: 0},
	})
}
