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
func mergeSettings(settings Settings, data []byte, origin string) (Settings, error) {
	// Keep TOML's strict type/range checks before koanf's struct decoding.
	// Partial secrets are valid here; validate their merged definitions below.
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
	if secrets, ok := overlay["secrets"].(map[string]any); ok {
		for _, value := range secrets {
			if secret, ok := value.(map[string]any); ok {
				if file, ok := secret["file"].(string); ok && file != "" && !filepath.IsAbs(file) {
					secret["file"] = filepath.Join(origin, file)
				}
			}
		}
	}
	if metadata.IsDefined("mounts") {
		for i := range partial.Mounts {
			source, err := resolveMountSource(partial.Mounts[i].Source, origin)
			if err != nil {
				return settings, fmt.Errorf("mounts[%d].source: %w", i, err)
			}
			partial.Mounts[i].Source = source
		}
		// Decode array tables through TOML, not koanf's weakly typed map decoder.
		delete(overlay, "mounts")
	}
	k := koanf.New(".")
	if err := k.Load(structs.Provider(settings, "toml"), nil); err != nil {
		return settings, err
	}
	if err := k.Load(confmap.Provider(overlay, ""), nil); err != nil {
		return settings, err
	}
	var merged Settings
	if err := k.UnmarshalWithConf("", &merged, koanf.UnmarshalConf{Tag: "toml"}); err != nil {
		return settings, err
	}
	if settings.Secrets == nil && partial.Secrets == nil {
		merged.Secrets = nil
	}
	merged.Mounts = settings.Mounts
	if metadata.IsDefined("mounts") {
		merged.Mounts = partial.Mounts
	}
	return merged, merged.Validate()
}
