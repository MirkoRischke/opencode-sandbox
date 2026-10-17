package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/BurntSushi/toml"
)

// Settings is the host-side launcher configuration, separate from OpenCode's
// guest JSON configuration. Add new launcher settings as fields or sections.
type Settings struct {
	Network NetworkSettings `toml:"network"`
}

type NetworkSettings struct {
	AllowedHosts []string `toml:"allowed_hosts"`
}

// DefaultSettings denies application internet access.
func DefaultSettings() Settings {
	return Settings{Network: NetworkSettings{AllowedHosts: []string{}}}
}

// LoadSettings overlays the user's config.toml onto restrictive defaults.
// A missing file uses defaults; an invalid file is an error.
func LoadSettings() (Settings, error) {
	settings := DefaultSettings()
	dir, err := userConfigDir()
	if err != nil {
		return settings, err
	}
	path := filepath.Join(dir, "opencode-sandbox", "config.toml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return settings, nil
	}
	if err != nil {
		return settings, fmt.Errorf("read launcher settings %s: %w", path, err)
	}
	metadata, err := toml.Decode(string(data), &settings)
	if err != nil {
		return settings, fmt.Errorf("decode launcher settings %s: %w", path, err)
	}
	if keys := metadata.Undecoded(); len(keys) != 0 {
		return settings, fmt.Errorf("unknown launcher setting: %s", keys[0])
	}
	if err := settings.Network.validate(); err != nil {
		return settings, fmt.Errorf("launcher settings %s: %w", path, err)
	}
	return settings, nil
}

var hostnamePattern = regexp.MustCompile(`(?i)^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func (n NetworkSettings) validate() error {
	if n.AllowedHosts == nil {
		return fmt.Errorf("network.allowed_hosts is required (use [] to deny internet access)")
	}
	for _, host := range n.AllowedHosts {
		if len(host) > 253 || !hostnamePattern.MatchString(host) {
			return fmt.Errorf("network.allowed_hosts: %q must be an exact DNS hostname, not a URL, wildcard, IP, or port", host)
		}
	}
	return nil
}
