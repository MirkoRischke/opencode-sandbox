package config

import (
	"fmt"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// LoadLoggingSettings reads only logging policy, without loading credentials or
// validating VM settings. Policy is used only when already approved;
// ordinary management commands must not prompt to trust configuration.
func (p Paths) LoadLoggingSettings(directory string) (LoggingSettings, error) {
	settings := DefaultSettings().Logging
	layers, err := p.configLayers(directory)
	if err != nil || len(layers) == 0 {
		return settings, err
	}
	store, err := loadTrustStore(filepath.Join(p.StateDir, "opencode-sandbox", "trust.json"))
	if err != nil {
		return settings, err
	}
	for _, layer := range layers {
		if !store.approves(layer.path, layer.data) {
			continue
		}
		settings, err = mergeLoggingSettings(settings, layer.data)
		if err != nil {
			return settings, fmt.Errorf("config %q: %w", layer.path, err)
		}
	}
	return settings, nil
}

func mergeLoggingSettings(settings LoggingSettings, data []byte) (LoggingSettings, error) {
	partial := struct {
		Logging LoggingSettings `toml:"logging"`
	}{Logging: settings}
	if _, err := toml.Decode(string(data), &partial); err != nil {
		return settings, fmt.Errorf("decode logging settings: %w", err)
	}
	return partial.Logging, partial.Logging.Validate()
}
