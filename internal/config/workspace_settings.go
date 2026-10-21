package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const workspaceConfigName = ".opencode-sandbox.toml"

// Paths contains host directories resolved by CLI setup, not environment lookups.
type Paths struct {
	ConfigDir string
	StateDir  string
}

// LoadWorkspaceSettings layers exact-directory settings over global settings.
// Approval is required for each new directory/content pair before credentials are read.
// approve receives the exact file bytes and the effective settings for review.
func (p Paths) LoadWorkspaceSettings(directory string, approve func(string, []byte, Settings) error) (Settings, error) {
	settings, err := LoadSettings(p.ConfigDir)
	if err != nil {
		return settings, err
	}
	root, err := filepath.Abs(directory)
	if err != nil {
		return settings, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return settings, err
	}
	path := filepath.Join(root, workspaceConfigName)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return settings, settings.ValidateFileSources()
	}
	if err != nil {
		return settings, fmt.Errorf("read workspace settings: %w", err)
	}
	settings, err = mergeSettings(settings, data, root)
	if err != nil {
		return settings, fmt.Errorf("workspace settings %s: %w", path, err)
	}
	if err := settings.ValidateFileSources(); err != nil {
		return settings, fmt.Errorf("workspace settings %s: %w", path, err)
	}
	if err := trustWorkspaceSettings(p.StateDir, root, data, func() error {
		if approve == nil {
			return fmt.Errorf("untrusted workspace config %s; launch interactively to approve it", path)
		}
		return approve(path, data, settings)
	}); err != nil {
		return settings, err
	}
	return settings, nil
}

type trustRecord struct {
	ConfigSHA256 string `json:"config_sha256"`
}

type trustStore struct {
	Version    int                    `json:"version"`
	Workspaces map[string]trustRecord `json:"workspaces"`
}

func trustWorkspaceSettings(stateDir, root string, data []byte, approve func() error) error {
	dir := filepath.Join(stateDir, "opencode-sandbox")
	path := filepath.Join(dir, "trust.json")
	store := trustStore{Version: 1, Workspaces: map[string]trustRecord{}}
	stored, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read workspace trust: %w", err)
	}
	if err == nil {
		store = trustStore{}
		decoder := json.NewDecoder(bytes.NewReader(stored))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&store); err != nil {
			return fmt.Errorf("decode workspace trust: %w", err)
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			return fmt.Errorf("invalid trailing data in workspace trust")
		}
		if store.Version != 1 || store.Workspaces == nil {
			return fmt.Errorf("invalid workspace trust store %s", path)
		}
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	if store.Workspaces[root].ConfigSHA256 == hash {
		return nil
	}
	if err := approve(); err != nil {
		return err
	}
	store.Workspaces[root] = trustRecord{ConfigSHA256: hash}
	encoded, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create workspace trust directory: %w", err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".trust-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // Best-effort cleanup; a successful rename already removes this path.
	_, writeErr := tmp.Write(append(encoded, '\n'))
	closeErr := tmp.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(tmp.Name(), path)
}
