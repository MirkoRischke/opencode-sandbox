package sandbox

import (
	"fmt"

	"opencode-sandbox/internal/config"

	m "github.com/superradcompany/microsandbox/sdk/go"
)

func validateMounts(paths worktreePaths, settings config.Settings) error {
	for i, mount := range settings.Mounts {
		for _, reserved := range []string{paths.root, paths.common, snapshotWorktree, "/var/lib/docker", "/proc", "/sys", "/dev", "/etc/opencode", "/etc/opencode-sandbox"} {
			if config.MountTargetsOverlap(mount.Target, reserved) {
				return fmt.Errorf("mounts[%d].target %q overlaps launcher/runtime mount %q", i, mount.Target, reserved)
			}
		}
	}
	return nil
}

func addReadOnlyMounts(mounts map[string]m.MountConfig, settings config.Settings) {
	for _, mount := range settings.Mounts {
		mounts[mount.Target] = m.Mount.Bind(mount.Source, m.MountOptions{
			Readonly: true, Owner: &m.MountOwner{UID: 0, GID: 0},
		})
	}
}
