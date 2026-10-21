package config

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type FileSettings struct {
	Source string `toml:"source" description:"Existing host file or directory. Relative paths resolve against this config file; ~/ resolves against the host home."`
	Target string `toml:"target" description:"Exact absolute guest path. Targets must not overlap other mappings or launcher-managed paths."`
	Mode   string `toml:"mode" description:"mount (default): live read-only host access. copy: writable guest copy, refreshed on creation and snapshot restore; directories merge contents, preserving destination-only files."`
}

// IsCopy distinguishes copies from mounts, including the implicit mount default.
func (f FileSettings) IsCopy() bool { return f.Mode == "copy" }

func resolveFileSource(source, origin string) (string, error) {
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

func validateFiles(files []FileSettings) error {
	for i, mount := range files {
		if mount.Mode != "" && mount.Mode != "mount" && mount.Mode != "copy" {
			return fmt.Errorf("files[%d].mode: %q must be mount or copy", i, mount.Mode)
		}
		if mount.Source == "" || strings.ContainsRune(mount.Source, '\x00') || !filepath.IsAbs(mount.Source) {
			return fmt.Errorf("files[%d].source: %q must resolve to an absolute host path", i, mount.Source)
		}
		if !path.IsAbs(mount.Target) || strings.ContainsRune(mount.Target, '\x00') || path.Clean(mount.Target) != mount.Target || mount.Target == "/" {
			return fmt.Errorf("files[%d].target: %q must be a clean absolute guest path other than /", i, mount.Target)
		}
		for j := range i {
			if MountTargetsOverlap(mount.Target, files[j].Target) {
				return fmt.Errorf("files[%d].target %q overlaps files[%d].target %q", i, mount.Target, j, files[j].Target)
			}
		}
	}
	return nil
}

// MountTargetsOverlap compares guest paths at directory boundaries.
func MountTargetsOverlap(a, b string) bool {
	return a == b || strings.HasPrefix(a, strings.TrimSuffix(b, "/")+"/") || strings.HasPrefix(b, strings.TrimSuffix(a, "/")+"/")
}

// ValidateFileSources checks host metadata, not file contents. Call after all
// layers are merged and before credentials are loaded or a VM is started.
func (s Settings) ValidateFileSources() error {
	for i, mount := range s.Files {
		info, err := os.Stat(mount.Source)
		if err != nil {
			return fmt.Errorf("files[%d].source %q: %w", i, mount.Source, err)
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("files[%d].source %q must be a regular file or directory", i, mount.Source)
		}
	}
	return nil
}
