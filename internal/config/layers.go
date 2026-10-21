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
	// Resolve only paths supplied by this layer, preserving inherited origins.
	for name, secret := range partial.Secrets {
		if secret.File != "" && !filepath.IsAbs(secret.File) {
			secret.File = filepath.Join(origin, secret.File)
		}
		partial.Secrets[name] = secret
	}
	delete(overlay, "secrets")
	if metadata.IsDefined("files") {
		for i := range partial.Files {
			source, err := resolveFileSource(partial.Files[i].Source, origin)
			if err != nil {
				return settings, fmt.Errorf("files[%d].source: %w", i, err)
			}
			partial.Files[i].Source = source
		}
		// Decode array tables through TOML, not koanf's weakly typed map decoder.
		delete(overlay, "files")
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
	merged.Secrets = nil
	if settings.Secrets != nil || partial.Secrets != nil {
		merged.Secrets = make(map[string]SecretSettings)
		for name, secret := range settings.Secrets {
			merged.Secrets[name] = secret
		}
		for name, secret := range partial.Secrets {
			merged.Secrets[name] = secret
		}
	}
	merged.Files = settings.Files
	if metadata.IsDefined("files") {
		merged.Files = partial.Files
	}
	return merged, merged.Validate()
}
