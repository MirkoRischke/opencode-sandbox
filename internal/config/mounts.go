package config

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type MountSettings struct {
	Source string `toml:"source" description:"Existing host file or directory. Relative paths resolve against this config file; ~/ resolves against the host home."`
	Target string `toml:"target" description:"Absolute guest path. Mounts are always read-only; targets must not overlap other mounts."`
}

func resolveMountSource(source, origin string) (string, error) {
	if strings.HasPrefix(source, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		source = filepath.Join(home, source[2:])
	}
	if source != "" && !filepath.IsAbs(source) {
		source = filepath.Join(origin, source)
	}
	return source, nil
}

func validateMounts(mounts []MountSettings) error {
	for i, mount := range mounts {
		if mount.Source == "" || strings.ContainsRune(mount.Source, '\x00') || !filepath.IsAbs(mount.Source) {
			return fmt.Errorf("mounts[%d].source: %q must resolve to an absolute host path", i, mount.Source)
		}
		if !path.IsAbs(mount.Target) || strings.ContainsRune(mount.Target, '\x00') || path.Clean(mount.Target) != mount.Target || mount.Target == "/" {
			return fmt.Errorf("mounts[%d].target: %q must be a clean absolute guest path other than /", i, mount.Target)
		}
		for j := range i {
			if MountTargetsOverlap(mount.Target, mounts[j].Target) {
				return fmt.Errorf("mounts[%d].target %q overlaps mounts[%d].target %q", i, mount.Target, j, mounts[j].Target)
			}
		}
	}
	return nil
}

// MountTargetsOverlap compares guest paths at directory boundaries.
func MountTargetsOverlap(a, b string) bool {
	return a == b || strings.HasPrefix(a, strings.TrimSuffix(b, "/")+"/") || strings.HasPrefix(b, strings.TrimSuffix(a, "/")+"/")
}

// ValidateMountSources checks host metadata, not file contents. Call after all
// layers are merged and before credentials are loaded or a VM is started.
func (s Settings) ValidateMountSources() error {
	for i, mount := range s.Mounts {
		info, err := os.Stat(mount.Source)
		if err != nil {
			return fmt.Errorf("mounts[%d].source %q: %w", i, mount.Source, err)
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("mounts[%d].source %q must be a regular file or directory", i, mount.Source)
		}
	}
	return nil
}
