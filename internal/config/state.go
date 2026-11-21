package config

import (
	"fmt"
	"regexp"
)

type StateSettings struct {
	Name   string `toml:"name" description:"Stable storage name, unique per workspace. Use lowercase letters, digits, and hyphens."`
	Target string `toml:"target" description:"Exact absolute guest directory to mount writable. Must not overlap other mappings or launcher-managed paths."`
}

var stateNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

func validateState(entries []StateSettings, files []FileSettings) error {
	for i, entry := range entries {
		if !stateNamePattern.MatchString(entry.Name) {
			return fmt.Errorf("state[%d].name: %q must be 1–63 lowercase letters, digits, or hyphens and start with a letter or digit", i, entry.Name)
		}
		if !validGuestTarget(entry.Target) {
			return fmt.Errorf("state[%d].target: %q must be a clean absolute guest path other than /", i, entry.Target)
		}
		for j, previous := range entries[:i] {
			if entry.Name == previous.Name {
				return fmt.Errorf("state[%d].name: %q duplicates state[%d].name", i, entry.Name, j)
			}
			if MountTargetsOverlap(entry.Target, previous.Target) {
				return fmt.Errorf("state[%d].target %q overlaps state[%d].target %q", i, entry.Target, j, previous.Target)
			}
		}
		for j, file := range files {
			if MountTargetsOverlap(entry.Target, file.Target) {
				return fmt.Errorf("state[%d].target %q overlaps files[%d].target %q", i, entry.Target, j, file.Target)
			}
		}
	}
	return nil
}
