package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
)

const workspaceConfigName = ".opencode-sandbox.toml"

// Paths contains host directories resolved by CLI setup, not environment lookups.
type Paths struct {
	ConfigDir string
	StateDir  string
	Logger    *slog.Logger
}

func (p Paths) log(message string, args ...any) {
	if p.Logger != nil {
		p.Logger.Debug(message, args...)
	}
}

// LoadWorkspaceSettings layers ancestor and workspace settings over global settings.
// Each file requires approval at its canonical location before credentials are read.
// approve receives the exact file bytes and the final merged settings.
func (p Paths) LoadWorkspaceSettings(directory string, approve func(string, []byte, Settings) error) (Settings, error) {
	settings, _, err := p.loadSettings(directory, func(path string, data []byte, settings Settings) error {
		err := trustConfig(p.StateDir, path, data, func() error {
			if approve == nil {
				return fmt.Errorf("untrusted config %q; launch interactively to approve it", path)
			}
			return approve(path, data, settings)
		})
		p.log("Configuration trust", "path", path, "accepted", err == nil)
		return err
	})
	return settings, err
}

// ValidateWorkspaceSettings checks configuration without reading or changing trust
// records. File sources are checked using metadata only; credentials are not read.
func (p Paths) ValidateWorkspaceSettings(directory string) (Settings, error) {
	settings, _, err := p.InspectWorkspaceSettings(directory)
	return settings, err
}

// InspectWorkspaceSettings returns validated settings and the config file paths
// in merge order, without accessing trust records or credentials.
func (p Paths) InspectWorkspaceSettings(directory string) (Settings, []string, error) {
	return p.loadSettings(directory, nil)
}

type configLayer struct {
	path      string
	data      []byte
	workspace bool
}

func (p Paths) loadSettings(directory string, approve func(string, []byte, Settings) error) (Settings, []string, error) {
	settings := DefaultSettings()
	p.log("Default configuration", "settings", settings)
	defaultPorts := settings.Network.VMToHost.Ports
	settings.Network.VMToHost.Ports = nil // No explicit ports yet.
	layers, err := p.configLayers(directory)
	if err != nil {
		return settings, nil, err
	}
	sources := make([]string, 0, len(layers))
	for _, layer := range layers {
		inheritedFiles := len(settings.Files)
		settings, err = mergeSettings(settings, layer.data, filepath.Dir(layer.path))
		if err != nil {
			return settings, nil, fmt.Errorf("config %q: %w", layer.path, err)
		}
		if !layer.workspace {
			for i, file := range settings.Files[inheritedFiles:] {
				if file.IsWritableMount() {
					return settings, nil, fmt.Errorf("config %q: files[%d].mode: mount-rw is only allowed in the workspace's own %s", layer.path, i, workspaceConfigName)
				}
			}
		}
		sources = append(sources, layer.path)
	}
	if settings.Network.VMToHost.Ports == nil {
		settings.Network.VMToHost.Ports = defaultPorts
	}
	if directory != "" {
		if err := settings.ValidateFileSources(); err != nil {
			return settings, nil, err
		}
	}
	if approve != nil {
		for _, layer := range layers {
			if err := approve(layer.path, layer.data, settings); err != nil {
				return settings, nil, err
			}
		}
	}
	return settings, sources, nil
}

func (p Paths) configLayers(directory string) ([]configLayer, error) {
	global, err := filepath.Abs(filepath.Join(p.ConfigDir, "opencode-sandbox", "config.toml"))
	if err != nil {
		return nil, err
	}
	var paths []string
	var workspaceConfig string
	if directory != "" {
		root, err := filepath.Abs(directory)
		if err != nil {
			return nil, err
		}
		root, err = filepath.EvalSymlinks(root)
		if err != nil {
			return nil, err
		}
		workspaceConfig = filepath.Join(root, workspaceConfigName)
		for dir := root; ; dir = filepath.Dir(dir) {
			paths = append(paths, filepath.Join(dir, workspaceConfigName))
			if filepath.Dir(dir) == dir {
				break
			}
		}
		slices.Reverse(paths)
	}
	var layers []configLayer
	for _, path := range append([]string{global}, paths...) {
		isWorkspace := path == workspaceConfig
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read config %q: %w", path, err)
		}
		// The containing directory also determines relative source paths.
		if path == global {
			dir, err := filepath.EvalSymlinks(filepath.Dir(path))
			if err != nil {
				return nil, fmt.Errorf("resolve config location %q: %w", path, err)
			}
			path = filepath.Join(dir, filepath.Base(path))
		}
		p.log("Configuration source", "path", path, "raw", string(data))
		layers = append(layers, configLayer{path: path, data: data, workspace: isWorkspace})
	}
	return layers, nil
}

type trustRecord struct {
	ConfigSHA256 string `json:"config_sha256"`
}

type trustStore struct {
	Version int                    `json:"version"`
	Configs map[string]trustRecord `json:"workspaces"` // Existing on-disk field; new entries use config file paths.
}

func (s trustStore) approves(path string, data []byte) bool {
	return s.Configs[path].ConfigSHA256 == configHash(data)
}

func configHash(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func trustConfig(stateDir, configPath string, data []byte, approve func() error) error {
	dir := filepath.Join(stateDir, "opencode-sandbox")
	path := filepath.Join(dir, "trust.json")
	store, err := loadTrustStore(path)
	if err != nil {
		return err
	}
	if store.approves(configPath, data) {
		return nil
	}
	if err := approve(); err != nil {
		return err
	}
	store.Configs[configPath] = trustRecord{ConfigSHA256: configHash(data)}
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

func loadTrustStore(path string) (trustStore, error) {
	store := trustStore{Version: 1, Configs: map[string]trustRecord{}}
	stored, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return store, nil
	}
	if err != nil {
		return store, fmt.Errorf("read workspace trust: %w", err)
	}
	store = trustStore{}
	decoder := json.NewDecoder(bytes.NewReader(stored))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&store); err != nil {
		return store, fmt.Errorf("decode workspace trust: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		if err != nil {
			return store, fmt.Errorf("invalid trailing data in workspace trust: %w", err)
		}
		return store, fmt.Errorf("invalid trailing data in workspace trust")
	}
	if store.Version != 1 || store.Configs == nil {
		return store, fmt.Errorf("invalid workspace trust store %s", path)
	}
	return store, nil
}
