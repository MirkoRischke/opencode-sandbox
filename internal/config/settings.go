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
	Ports    []PortSettings            `toml:"ports" description:"TCP mappings from host 127.0.0.1 to guest ports. Empty exposes nothing. Workspace lists replace global lists; recreate the VM after changes."`
	Files    []FileSettings            `toml:"files" description:"Explicit host file/directory mappings. Default mode mount is read-only; copy seeds writable guest files on creation and snapshot restore. Recreate the VM after changing mappings."`
	AWS      AWSSettings               `toml:"aws" description:"Optional AWS role credentials for launches. CLI flags override individual fields; host credentials are never stored here."`
	VM       VMSettings                `toml:"vm" description:"VM resources. Recreate the VM after changing these settings."`
	Network  NetworkSettings           `toml:"network" description:"Outbound network access. Recreate the VM after policy changes."`
	Snapshot SnapshotSettings          `toml:"snapshot" description:"Per-directory prepared disk baseline. Created automatically on first launch; rebuild and remove with the snapshot commands. Rebuild the snapshot and recreate the VM after provisioning changes."`
	Logging  LoggingSettings           `toml:"logging" description:"Host launcher logging. Console output goes to stderr; JSON logfiles retain Info and above."`
	Secrets  map[string]SecretSettings `toml:"secrets" description:"Optional file- or OS-keyring-backed secrets, substituted by the host proxy. Workspace entries replace same-named global entries completely. Uncomment and adjust the example to enable."`
}

type LoggingSettings struct {
	ConsoleLevel string `toml:"console_level" description:"Minimum console severity: debug, info, warn, or error. Overridden by --console-log-level."`
}

func (s LoggingSettings) Validate() error {
	switch s.ConsoleLevel {
	case "debug", "info", "warn", "error":
		return nil
	default:
		return fmt.Errorf("logging.console_level must be debug, info, warn, or error (got %q)", s.ConsoleLevel)
	}
}

type AWSSettings struct {
	Role    string `toml:"role" description:"IAM role ARN to assume on the host. Empty disables AWS when profile is also empty."`
	Profile string `toml:"profile" description:"Optional host AWS source profile. Requires a role after config and CLI overrides are combined."`
}

type SnapshotSettings struct {
	Command string `toml:"command" description:"Optional shell command run after project initialization in a fresh preparation VM before capture, from the mounted workspace. Empty skips the custom command."`
}

type VMSettings struct {
	Image         string `toml:"image" description:"Docker image for fresh VMs. Uses the local Docker image when present; otherwise pulls it. Imported into Microsandbox automatically. Existing VMs and snapshots are unchanged."`
	CPUs          uint8  `toml:"cpus" description:"Number of virtual CPUs (1-255)."`
	MemoryMiB     uint32 `toml:"memory_mib" description:"Guest memory in MiB."`
	RootDiskMiB   uint32 `toml:"root_disk_mib" description:"Persistent guest disk size in MiB; excludes mounted host workspaces."`
	DockerDiskMiB uint32 `toml:"docker_disk_mib" description:"Private ext4 Docker data disk capacity in MiB; deleted with the VM."`
}

type NetworkSettings struct {
	Unrestricted           bool     `toml:"unrestricted" description:"Allow public internet on any port/protocol, ignoring both host lists. Protected destinations remain blocked; secret scopes are unchanged."`
	Diagnostics            bool     `toml:"diagnostics" description:"Collect cumulative denied-target diagnostics per workspace. Enables full rotating runtime debug logs and HTTP denial responses; recreate the VM after changing this setting."`
	AllowedHosts           []string `toml:"allowed_hosts" description:"Baseline DNS hosts: example.com matches exactly; .example.com includes the base domain and all subdomains. Passed unchanged to Microsandbox; HTTPS/WSS on TCP port 443 only."`
	AdditionalAllowedHosts []string `toml:"additional_allowed_hosts" description:"Extra hostnames, combined with allowed_hosts after merging. With unrestricted disabled, both lists must be empty to deny internet access. Each list replaces its inherited value when specified."`
}

// DefaultSettings allows common development services over HTTPS.
func DefaultSettings() Settings {
	return Settings{
		Logging: LoggingSettings{ConsoleLevel: "info"},
		Files:   []FileSettings{},
		Ports:   []PortSettings{},
		VM:      VMSettings{Image: "opencode-sandbox:poc", CPUs: 2, MemoryMiB: 2048, RootDiskMiB: 4096, DockerDiskMiB: 10240},
		Network: NetworkSettings{
			AllowedHosts: []string{
				// OpenAI inference and authentication.
				"chatgpt.com", "api.openai.com", "auth.openai.com",
				// Model catalog and remote documentation MCPs.
				"models.dev", "mcp.context7.com", "knowledge-mcp.global.api.aws",
				// GitHub Git operations, APIs, and raw files.
				"github.com", "api.github.com", "raw.githubusercontent.com",
			},
			AdditionalAllowedHosts: []string{},
		},
	}
}

// LoadSettings overlays the user's config.toml onto built-in defaults.
// A missing file uses defaults; an invalid file is an error.
func LoadSettings(configDir string) (Settings, error) {
	return (Paths{ConfigDir: configDir}).loadGlobalSettings()
}

func (p Paths) loadGlobalSettings() (Settings, error) {
	settings := DefaultSettings()
	p.log("Default configuration", "settings", settings)
	path := filepath.Join(p.ConfigDir, "opencode-sandbox", "config.toml")
	data, err := os.ReadFile(path)
	p.log("Configuration source", "source", "global", "path", path, "present", !os.IsNotExist(err), "raw", string(data))
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
	if err := s.Logging.Validate(); err != nil {
		return err
	}
	if strings.ContainsRune(s.Snapshot.Command, '\x00') {
		return fmt.Errorf("snapshot.command must not contain NUL bytes")
	}
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
	if err := validateFiles(s.Files); err != nil {
		return err
	}
	if err := validatePorts(s.Ports); err != nil {
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
			name := strings.TrimPrefix(host, ".")
			if len(name) > 253 || !hostnamePattern.MatchString(name) {
				return fmt.Errorf("network.%s: %q must be a DNS hostname or .prefixed domain suffix, not a URL, wildcard, IP, or port", list.name, host)
			}
		}
	}
	return nil
}
