package sandbox

import (
	"fmt"

	"opencode-sandbox/internal/config"
	"opencode-sandbox/internal/workspace"

	m "github.com/superradcompany/microsandbox/sdk/go"
)

func validateMounts(w workspace.Workspace, settings config.Settings) error {
	reservedPaths := []string{w.Root(), snapshotWorktree, "/var/lib/docker", openCodeDatabaseDir, "/proc", "/sys", "/dev", "/etc/opencode", "/etc/opencode-sandbox"}
	for _, mount := range w.Mounts(snapshotWorktree) {
		reservedPaths = append(reservedPaths, mount.Target)
	}
	for i, mount := range settings.Files {
		for _, reserved := range reservedPaths {
			if config.MountTargetsOverlap(mount.Target, reserved) {
				return fmt.Errorf("files[%d].target %q overlaps launcher/runtime mount %q", i, mount.Target, reserved)
			}
		}
	}
	return nil
}

func workspaceMounts(w workspace.Workspace, guestRoots ...string) map[string]m.MountConfig {
	mounts := map[string]m.MountConfig{}
	owner := m.MountOptions{Owner: &m.MountOwner{UID: 0, GID: 0}}
	for _, root := range guestRoots {
		for _, mount := range w.Mounts(root) {
			mounts[mount.Target] = m.Mount.Bind(mount.Source, owner)
		}
	}
	return mounts
}

func addReadOnlyMounts(mounts map[string]m.MountConfig, settings config.Settings) {
	for _, mount := range settings.Files {
		if mount.IsCopy() {
			continue
		}
		mounts[mount.Target] = m.Mount.Bind(mount.Source, m.MountOptions{
			Readonly: true, Owner: &m.MountOwner{UID: 0, GID: 0},
		})
	}
}
