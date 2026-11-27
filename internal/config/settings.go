// Package config loads and documents host-side launcher settings independently
// of the sandbox runtime and interactive terminal.
package config

import (
	"fmt"
	"regexp"
	"strings"
)

// Settings is the host-side launcher configuration, separate from OpenCode's
// guest JSON configuration. Add new launcher settings as fields or sections.
type Settings struct {
	Files    []FileSettings            `toml:"files" description:"Explicit host file/directory mappings. mount-ro is the default; mount-rw is only for directories in the workspace's own config; copy seeds writable guest files on creation and snapshot restore. Recreate the VM after changing mappings."`
	State    []StateSettings           `toml:"state" description:"Opt-in writable directories kept per workspace across VM removal. Workspace lists replace global lists. Recreate the VM after changing mappings."`
	AWS      AWSSettings               `toml:"aws" description:"Optional AWS role credentials for launches. CLI flags override individual fields; host credentials are never stored here."`
	VM       VMSettings                `toml:"vm" description:"VM resources. Recreate the VM after changing these settings."`
	Startup  StartupSettings           `toml:"startup" description:"Command run in the guest workspace when opening the terminal, not when preparing a snapshot. Changes apply on the next launch."`
	Network  NetworkSettings           `toml:"network" description:"Network access and directional port exposure. Recreate the VM after changes."`
	Snapshot SnapshotSettings          `toml:"snapshot" description:"Prepared disk baseline shared by linked Git worktrees; per-directory outside Git. Created automatically on first launch; rebuild and remove with the snapshot commands. Rebuild the snapshot and recreate the VM after provisioning changes."`
	Logging  LoggingSettings           `toml:"logging" description:"Host launcher logging. Console output goes to stderr; JSON logfiles retain Debug and above."`
	GitHub   *GitHubSettings           `toml:"github,omitempty" description:"Opt in to GitHub App authentication instead of a static GH_TOKEN."`
	Secrets  map[string]SecretSettings `toml:"secrets" description:"Optional file- or OS-keyring-backed secrets, substituted by the host proxy. Workspace entries replace same-named global entries completely. Uncomment and adjust the example to enable."`
}

type GitHubSettings struct {
	Owner string `toml:"owner" description:"GitHub username or organization that owns the repositories."`
	AppID int64  `toml:"app_id" description:"GitHub App ID. Store its base64-encoded PEM in the host keyring as github-app-<app_id>."`
}

var githubOwnerPattern = regexp.MustCompile(`(?i)^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func (s GitHubSettings) validate() error {
	if !githubOwnerPattern.MatchString(s.Owner) {
		return fmt.Errorf("github.owner must be a GitHub username or organization name")
	}
	if s.AppID <= 0 {
		return fmt.Errorf("github.app_id must be a positive GitHub App ID")
	}
	return nil
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

const DefaultStartupExecutable = "opencode"

type StartupSettings struct {
	Command []string `toml:"command" description:"Guest executable followed by literal arguments. Must not be empty. Interactive Bash opens after it exits."`
}

func (s StartupSettings) validate() error {
	if len(s.Command) == 0 || strings.TrimSpace(s.Command[0]) == "" {
		return fmt.Errorf("startup.command must begin with a guest executable")
	}
	for i, arg := range s.Command {
		if strings.ContainsRune(arg, '\x00') {
			return fmt.Errorf("startup.command[%d] must not contain NUL bytes", i)
		}
	}
	return nil
}

type VMSettings struct {
	Image         string `toml:"image" description:"Docker image for fresh VMs. Uses the local Docker image when present; otherwise pulls it. Imported into Microsandbox automatically. Existing VMs and snapshots are unchanged."`
	CPUs          uint8  `toml:"cpus" description:"Number of virtual CPUs (1-255)."`
	MemoryMiB     uint32 `toml:"memory_mib" description:"Guest memory in MiB."`
	RootDiskMiB   uint32 `toml:"root_disk_mib" description:"Persistent guest disk size in MiB; excludes mounted host workspaces."`
	DockerDiskMiB uint32 `toml:"docker_disk_mib" description:"Private ext4 Docker data disk capacity in MiB; deleted with the VM."`
}

type NetworkSettings struct {
	Unrestricted bool             `toml:"unrestricted" description:"Allow public internet on any port/protocol, ignoring allowed_hosts. Protected destinations remain blocked except explicit vm_to_host ports; secret scopes are unchanged."`
	Diagnostics  bool             `toml:"diagnostics" description:"Collect cumulative denied-target diagnostics per workspace. Enables full rotating runtime debug logs and HTTP denial responses; recreate the VM after changing this setting."`
	AllowedHosts []string         `toml:"allowed_hosts" description:"Public DNS hosts, added across config files. example.com matches exactly; .example.com includes the base domain and all subdomains. HTTPS/WSS on TCP port 443 only."`
	HostToVM     []PortSettings   `toml:"host_to_vm" description:"TCP mappings allowing the host to reach VM services, bound to host 127.0.0.1 only. Mappings are added across config files; none are exposed by default."`
	VMToHost     VMToHostSettings `toml:"vm_to_host" description:"Allow the VM to reach explicitly selected host-local services via host.microsandbox.internal. No port mapping."`
}

type VMToHostSettings struct {
	Ports []uint16 `toml:"ports" description:"Host TCP ports (1-65535) reachable from the VM. Explicit lists are combined across config files. If omitted everywhere, [4000] allows the LLM gateway."`
}

// DefaultSettings denies public guest traffic unless configured otherwise.
func DefaultSettings() Settings {
	return Settings{
		Logging: LoggingSettings{ConsoleLevel: "info"},
		Files:   []FileSettings{},
		State:   []StateSettings{},
		VM:      VMSettings{Image: DefaultImage(), CPUs: 2, MemoryMiB: 2048, RootDiskMiB: 4096, DockerDiskMiB: 10240},
		Startup: StartupSettings{Command: []string{DefaultStartupExecutable}},
		Network: NetworkSettings{
			HostToVM:     []PortSettings{},
			VMToHost:     VMToHostSettings{Ports: []uint16{4000}},
			AllowedHosts: []string{},
		},
	}
}

// LoadSettings overlays the user's config.toml onto built-in defaults.
// A missing file uses defaults; an invalid file is an error.
func LoadSettings(configDir string) (Settings, error) {
	settings, _, err := (Paths{ConfigDir: configDir}).loadSettings("", nil)
	return settings, err
}

// Validate rejects invalid launcher settings before runtime preparation.
func (s Settings) Validate() error {
	if err := s.Logging.Validate(); err != nil {
		return err
	}
	if err := s.Startup.validate(); err != nil {
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
	if err := validateState(s.State, s.Files); err != nil {
		return err
	}
	if s.GitHub != nil {
		if err := s.GitHub.validate(); err != nil {
			return err
		}
	}
	return ValidateSecrets(s.Secrets)
}

var hostnamePattern = regexp.MustCompile(`(?i)^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func (n NetworkSettings) validate() error {
	if err := validatePorts(n.HostToVM); err != nil {
		return err
	}
	seen := make(map[uint16]bool, len(n.VMToHost.Ports))
	for i, port := range n.VMToHost.Ports {
		if port == 0 {
			return fmt.Errorf("network.vm_to_host.ports[%d]: port must be between 1 and 65535", i)
		}
		if seen[port] {
			return fmt.Errorf("network.vm_to_host.ports[%d]: duplicate host TCP port %d", i, port)
		}
		seen[port] = true
	}
	if n.AllowedHosts == nil {
		return fmt.Errorf("network.allowed_hosts is required (use [] for an empty baseline)")
	}
	for _, host := range n.AllowedHosts {
		name := strings.TrimPrefix(host, ".")
		if len(name) > 253 || !hostnamePattern.MatchString(name) {
			return fmt.Errorf("network.allowed_hosts: %q must be a DNS hostname or .prefixed domain suffix, not a URL, wildcard, IP, or port", host)
		}
	}
	return nil
}
