package config

import (
	"fmt"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
)

// mergeSettings overlays a TOML layer onto inherited settings. Koanf merges
// tables recursively and replaces other values, including false and empty lists.
// Named secrets are handled separately as atomic entries.
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
	if metadata.IsDefined("files") {
		delete(overlay, "files")
	}
	if network, ok := overlay["network"].(map[string]any); ok {
		delete(network, "host_to_vm") // Preserve TOML's strict array-table decoding.
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
	merged.Network.HostToVM = settings.Network.HostToVM
	if metadata.IsDefined("network", "host_to_vm") {
		merged.Network.HostToVM = partial.Network.HostToVM
	}
	merged.Network.VMToHost = settings.Network.VMToHost
	if metadata.IsDefined("network", "vm_to_host", "ports") {
		merged.Network.VMToHost.Ports = partial.Network.VMToHost.Ports
	}
	merged.Files = settings.Files
	if metadata.IsDefined("files") {
		merged.Files = partial.Files
	}
	return merged, merged.Validate()
}

// Resolve only paths supplied by this layer, preserving inherited origins.
func resolveLayerPaths(partial *Settings, metadata toml.MetaData, origin string) error {
	for name, secret := range partial.Secrets {
		if secret.File != "" && !filepath.IsAbs(secret.File) {
			secret.File = filepath.Join(origin, secret.File)
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
