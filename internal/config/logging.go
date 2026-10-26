package config

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// LoadLoggingSettings reads only logging policy, without loading credentials or
// validating VM settings. Workspace policy is used only when already approved;
// ordinary management commands must not prompt to trust workspace configuration.
func (p Paths) LoadLoggingSettings(directory string) (LoggingSettings, error) {
	settings := DefaultSettings().Logging
	global := filepath.Join(p.ConfigDir, "opencode-sandbox", "config.toml")
	data, err := os.ReadFile(global)
	if err != nil && !os.IsNotExist(err) {
		return settings, fmt.Errorf("read launcher logging settings: %w", err)
	}
	settings, err = mergeLoggingSettings(settings, data)
	if err != nil || directory == "" {
		return settings, err
	}
	data, err = os.ReadFile(filepath.Join(directory, workspaceConfigName))
	if os.IsNotExist(err) {
		return settings, nil
	}
	if err != nil {
		return settings, fmt.Errorf("read workspace logging settings: %w", err)
	}
	store, err := loadTrustStore(filepath.Join(p.StateDir, "opencode-sandbox", "trust.json"))
	if err != nil {
		return settings, err
	}
	if store.Workspaces[directory].ConfigSHA256 != fmt.Sprintf("%x", sha256.Sum256(data)) {
		return settings, nil
	}
	return mergeLoggingSettings(settings, data)
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
