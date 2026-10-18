package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func validateRoot(root string, info os.FileInfo) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("locate home for workspace safety: %w", err)
	}
	home, err = filepath.EvalSymlinks(home)
	if err != nil {
		return fmt.Errorf("resolve home for workspace safety: %w", err)
	}
	if protectedPath(root, home) {
		return unsafeRoot(root)
	}
	parent, err := os.Stat(filepath.Dir(root))
	if err != nil {
		return fmt.Errorf("inspect workspace parent: %w", err)
	}
	// A device change identifies volume roots on both Linux and macOS.
	stat, ok := info.Sys().(*syscall.Stat_t)
	parentStat, parentOK := parent.Sys().(*syscall.Stat_t)
	if ok && parentOK && stat.Dev != parentStat.Dev {
		return unsafeRoot(root)
	}
	// Linux bind mounts can share the parent's device number.
	mounted, err := mountRoot(root)
	if err != nil {
		return fmt.Errorf("inspect workspace mount root: %w", err)
	}
	if mounted {
		return unsafeRoot(root)
	}
	return nil
}

func unsafeRoot(root string) error {
	return fmt.Errorf("unsafe workspace directory %q: choose a project subdirectory instead of a drive root, home, or system directory", root)
}

func protectedPath(root, home string) bool {
	if root == "/" || within(home, root) {
		return true
	}
	for _, path := range []string{
		"/home", "/Users", "/Users/Shared", "/root", "/Volumes", "/mnt", "/media",
		"/tmp", "/var/tmp", "/private/tmp", "/private/var/tmp",
		"/opt", "/srv", "/var", "/private/var", "/private", "/System", "/Library", "/Applications",
	} {
		if root == path {
			return true
		}
	}
	// Conventional user-home locations (including Linux's /media/$USER).
	for _, parent := range []string{"/home", "/Users", "/media"} {
		if filepath.Dir(root) == parent {
			return true
		}
	}
	for _, path := range []string{
		"/etc", "/dev", "/proc", "/sys", "/run", "/boot", "/bin", "/sbin", "/usr",
		"/System", "/private/etc", "/private/var/run",
	} {
		if within(root, path) {
			return true
		}
	}
	return false
}

func isMountRoot(root string, mountinfo []byte) bool {
	decode := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	for _, line := range strings.Split(string(mountinfo), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 5 && decode.Replace(fields[4]) == root {
			return true
		}
	}
	return false
}
