// Package config loads and documents host-side launcher settings independently
// of the sandbox runtime and interactive terminal.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Settings is the host-side launcher configuration, separate from OpenCode's
// guest JSON configuration. Add new launcher settings as fields or sections.
type Settings struct {
	VM      VMSettings                `toml:"vm" description:"VM resources. Recreate the VM after changing these settings."`
	Network NetworkSettings           `toml:"network" description:"Allowed HTTPS destinations. Recreate the VM after policy changes."`
	Secrets map[string]SecretSettings `toml:"secrets" description:"Optional file-backed secrets, substituted by the host proxy. Uncomment and adjust the example to enable."`
}

type VMSettings struct {
	Image         string `toml:"image" description:"VM image already loaded into Microsandbox; images are never pulled automatically."`
	CPUs          uint8  `toml:"cpus" description:"Number of virtual CPUs (1-255)."`
	MemoryMiB     uint32 `toml:"memory_mib" description:"Guest memory in MiB."`
	RootDiskMiB   uint32 `toml:"root_disk_mib" description:"Persistent guest disk size in MiB; excludes mounted host worktrees."`
	DockerDiskMiB uint32 `toml:"docker_disk_mib" description:"Private ext4 Docker data disk capacity in MiB; deleted with the VM."`
}

type NetworkSettings struct {
	Diagnostics            bool     `toml:"diagnostics" description:"Collect cumulative denied-target diagnostics per worktree. Enables full rotating runtime debug logs and HTTP denial responses; recreate the VM after changing this setting."`
	AllowedHosts           []string `toml:"allowed_hosts" description:"Baseline exact DNS hostnames; HTTPS/WSS on TCP port 443 only."`
	AdditionalAllowedHosts []string `toml:"additional_allowed_hosts" description:"Extra hostnames, combined with allowed_hosts after merging. Both lists must be empty to deny internet access. Each list replaces its inherited value when specified."`
}

// DefaultSettings denies application internet access.
func DefaultSettings() Settings {
	return Settings{
		VM:      VMSettings{Image: "opencode-sandbox:poc", CPUs: 2, MemoryMiB: 2048, RootDiskMiB: 4096, DockerDiskMiB: 10240},
		Network: NetworkSettings{AllowedHosts: []string{}, AdditionalAllowedHosts: []string{}},
	}
}

// LoadSettings overlays the user's config.toml onto restrictive defaults.
// A missing file uses defaults; an invalid file is an error.
func LoadSettings(configDir string) (Settings, error) {
	settings := DefaultSettings()
	path := filepath.Join(configDir, "opencode-sandbox", "config.toml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return settings, nil
	}
	if err != nil {
		return settings, fmt.Errorf("read launcher settings %s: %w", path, err)
	}
	settings, err = mergeSettings(settings, data, filepath.Dir(path))
	if err != nil {
		return settings, fmt.Errorf("decode launcher settings %s: %w", path, err)
	}
	return settings, nil
}

// Validate rejects invalid launcher settings before runtime preparation.
func (s Settings) Validate() error {
	if strings.TrimSpace(s.VM.Image) == "" {
		return fmt.Errorf("vm.image must not be empty")
	}
	if s.VM.CPUs == 0 {
		return fmt.Errorf("vm.cpus must be greater than zero")
	}
	if s.VM.MemoryMiB == 0 {
		return fmt.Errorf("vm.memory_mib must be greater than zero")
	}
	if s.VM.RootDiskMiB == 0 {
		return fmt.Errorf("vm.root_disk_mib must be greater than zero")
	}
	if s.VM.DockerDiskMiB == 0 {
		return fmt.Errorf("vm.docker_disk_mib must be greater than zero")
	}
	if err := s.Network.validate(); err != nil {
		return err
	}
	return ValidateSecrets(s.Secrets)
}

var hostnamePattern = regexp.MustCompile(`(?i)^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func (n NetworkSettings) validate() error {
	if n.AllowedHosts == nil {
		return fmt.Errorf("network.allowed_hosts is required (use [] for an empty baseline)")
	}
	for _, list := range []struct {
		name  string
		hosts []string
	}{
		{"allowed_hosts", n.AllowedHosts},
		{"additional_allowed_hosts", n.AdditionalAllowedHosts},
	} {
		for _, host := range list.hosts {
			if len(host) > 253 || !hostnamePattern.MatchString(host) {
				return fmt.Errorf("network.%s: %q must be an exact DNS hostname, not a URL, wildcard, IP, or port", list.name, host)
			}
		}
	}
	return nil
}
