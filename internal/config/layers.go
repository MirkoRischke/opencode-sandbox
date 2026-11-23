package config

import (
	"fmt"
	"path/filepath"
	"slices"

	"github.com/BurntSushi/toml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
)

// mergeSettings overlays a TOML layer onto inherited settings. Koanf merges
// tables recursively and replaces scalars and commands. Collections append;
// named secrets replace as atomic entries.
func mergeSettings(settings Settings, data []byte, origin string) (Settings, error) {
	// Keep TOML's strict type/range checks before koanf's struct decoding.
	// Secret entries replace inherited definitions rather than merging fields.
	var partial Settings
	metadata, err := toml.Decode(string(data), &partial)
	if err != nil {
		return settings, err
	}
	if keys := metadata.Undecoded(); len(keys) != 0 {
		return settings, fmt.Errorf("unknown launcher setting: %s", keys[0])
	}
	var overlay map[string]any
	if _, err := toml.Decode(string(data), &overlay); err != nil {
		return settings, err
	}
	if err := resolveLayerPaths(&partial, metadata, origin); err != nil {
		return settings, err
	}
	delete(overlay, "secrets")
	delete(overlay, "files")
	delete(overlay, "state")
	if network, ok := overlay["network"].(map[string]any); ok {
		delete(network, "allowed_hosts")
		delete(network, "host_to_vm")
		delete(network, "vm_to_host")
	}
	k := koanf.New(".")
	base := settings
	base.Secrets = nil // Preserve named entries and empty keyring tables outside koanf.
	if err := k.Load(structs.Provider(base, "toml"), nil); err != nil {
		return settings, err
	}
	if err := k.Load(confmap.Provider(overlay, ""), nil); err != nil {
		return settings, err
	}
	var merged Settings
	if err := k.UnmarshalWithConf("", &merged, koanf.UnmarshalConf{Tag: "toml"}); err != nil {
		return settings, err
	}
	merged.Secrets = mergeSecretEntries(settings.Secrets, partial.Secrets)
	merged.Network.AllowedHosts = appendCollection(settings.Network.AllowedHosts, partial.Network.AllowedHosts)
	merged.Network.HostToVM = appendCollection(settings.Network.HostToVM, partial.Network.HostToVM)
	merged.Network.VMToHost = settings.Network.VMToHost
	if metadata.IsDefined("network", "vm_to_host", "ports") {
		ports := settings.Network.VMToHost.Ports
		if ports == nil {
			ports = []uint16{} // An explicit empty list also opts out of the built-in fallback.
		}
		merged.Network.VMToHost.Ports = appendCollection(ports, partial.Network.VMToHost.Ports)
	}
	merged.Files = appendCollection(settings.Files, partial.Files)
	merged.State = appendCollection(settings.State, partial.State)
	return merged, merged.Validate()
}

func appendCollection[T any](inherited, added []T) []T {
	if len(added) == 0 {
		return inherited
	}
	return slices.Concat(inherited, added)
}

// Resolve only paths supplied by this layer, preserving inherited origins.
func resolveLayerPaths(partial *Settings, metadata toml.MetaData, origin string) error {
	for name, secret := range partial.Secrets {
		if secret.Source.File != "" && !filepath.IsAbs(secret.Source.File) {
			secret.Source.File = filepath.Join(origin, secret.Source.File)
		}
		partial.Secrets[name] = secret
	}
	if !metadata.IsDefined("files") {
		return nil
	}
	// Decode array tables through TOML, not koanf's weakly typed map decoder.
	for i := range partial.Files {
		source, err := resolveFileSource(partial.Files[i].Source, origin)
		if err != nil {
			return fmt.Errorf("files[%d].source: %w", i, err)
		}
		partial.Files[i].Source = source
	}
	return nil
}

// Named secrets replace whole entries; omitted fields never leak from inheritance.
func mergeSecretEntries(inherited, overlay map[string]SecretSettings) map[string]SecretSettings {
	if inherited == nil && overlay == nil {
		return nil
	}
	merged := make(map[string]SecretSettings, len(inherited)+len(overlay))
	for name, secret := range inherited {
		merged[name] = secret
	}
	for name, secret := range overlay {
		merged[name] = secret
	}
	return merged
}
