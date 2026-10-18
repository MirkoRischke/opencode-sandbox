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
	return merged, merged.Validate()
}
