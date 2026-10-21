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
	Mounts   []MountSettings           `toml:"mounts" description:"Explicit read-only host file/directory mounts. No files are shared by default. Recreate the VM after changing mappings."`
	VM       VMSettings                `toml:"vm" description:"VM resources. Recreate the VM after changing these settings."`
	Network  NetworkSettings           `toml:"network" description:"Outbound network access. Recreate the VM after policy changes."`
	Snapshot SnapshotSettings          `toml:"snapshot" description:"Optional per-directory prepared disk baseline. Created and removed explicitly with the snapshot commands."`
	Secrets  map[string]SecretSettings `toml:"secrets" description:"Optional file- or OS-keyring-backed secrets, substituted by the host proxy. Workspace entries replace same-named global entries completely. Uncomment and adjust the example to enable."`
}

type SnapshotSettings struct {
	Command string `toml:"command" description:"Optional shell command run after project initialization in a fresh preparation VM before capture, from the mounted workspace. Empty skips the custom command."`
}

type VMSettings struct {
	Image         string `toml:"image" description:"VM image already loaded into Microsandbox; images are never pulled automatically."`
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
		Mounts: []MountSettings{},
		VM:     VMSettings{Image: "opencode-sandbox:poc", CPUs: 2, MemoryMiB: 2048, RootDiskMiB: 4096, DockerDiskMiB: 10240},
		Network: NetworkSettings{
			AllowedHosts: []string{
				// OpenAI inference and authentication.
				"chatgpt.com", "api.openai.com", "auth.openai.com",
				// Model catalog and remote documentation MCPs.
				"models.dev", "mcp.context7.com", "knowledge-mcp.global.api.aws",
				// GitHub source, APIs, and release downloads.
				"github.com", "api.github.com", "raw.githubusercontent.com",
				"codeload.github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com",
				// mise metadata, Go toolchains/modules, and npm packages.
				"mise-versions.jdx.dev", "dl.google.com", "proxy.golang.org",
				"sum.golang.org", "storage.googleapis.com", "registry.npmjs.org",
				// Docker Hub and Amazon ECR Public image pulls.
				"auth.docker.io", "registry-1.docker.io", "production.cloudflare.docker.com",
				"production.cloudfront.docker.com",
				"docker-images-prod.6aa30f8b08e16409b46e0173d6de2f56.r2.cloudflarestorage.com",
				"public.ecr.aws",
			},
			AdditionalAllowedHosts: []string{},
		},
	}
}

// LoadSettings overlays the user's config.toml onto built-in defaults.
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
	if err := validateMounts(s.Mounts); err != nil {
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
