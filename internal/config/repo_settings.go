package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const repoConfigName = ".opencode-sandbox.toml"

// LoadRepoSettings layers repo-root settings over global settings. Approval is
// required for each new repo path/content pair, before any credentials are read.
// approve receives the exact file bytes and the effective settings for review.
func LoadRepoSettings(directory string, approve func(string, []byte, Settings) error) (Settings, error) {
	settings, err := LoadSettings()
	if err != nil {
		return settings, err
	}
	out, err := exec.Command("git", "-C", directory, "rev-parse", "--show-toplevel").CombinedOutput()
	if err != nil {
		return settings, fmt.Errorf("discover worktree: %w: %s", err, out)
	}
	root, err := filepath.EvalSymlinks(strings.TrimSpace(string(out)))
	if err != nil {
		return settings, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return settings, err
	}
	path := filepath.Join(root, repoConfigName)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return settings, nil
	}
	if err != nil {
		return settings, fmt.Errorf("read repo settings: %w", err)
	}
	settings, err = mergeRepoSettings(settings, data, root)
	if err != nil {
		return settings, fmt.Errorf("repo settings %s: %w", path, err)
	}
	if err := trustRepoSettings(root, data, func() error {
		if approve == nil {
			return fmt.Errorf("untrusted repo config %s; launch interactively to approve it", path)
		}
		return approve(path, data, settings)
	}); err != nil {
		return settings, err
	}
	return settings, nil
}

func mergeRepoSettings(settings Settings, data []byte, root string) (Settings, error) {
	// Check every repo key against the schema, even when the global layer has no
	// corresponding value. Partial secret definitions are validated after merging.
	var partial Settings
	metadata, err := toml.Decode(string(data), &partial)
	if err != nil {
		return settings, err
	}
	if keys := metadata.Undecoded(); len(keys) != 0 {
		return settings, fmt.Errorf("unknown launcher setting: %s", keys[0])
	}
	var base, overlay map[string]any
	var encoded bytes.Buffer
	if err := toml.NewEncoder(&encoded).Encode(settings); err != nil {
		return settings, err
	}
	if _, err := toml.Decode(encoded.String(), &base); err != nil {
		return settings, err
	}
	if _, err := toml.Decode(string(data), &overlay); err != nil {
		return settings, err
	}
	// Resolve only paths supplied by this layer; inherited paths retain their
	// global config directory origin when credentials are loaded by the sandbox.
	if secrets, ok := overlay["secrets"].(map[string]any); ok {
		for _, value := range secrets {
			if secret, ok := value.(map[string]any); ok {
				if file, ok := secret["file"].(string); ok && file != "" && !filepath.IsAbs(file) {
					secret["file"] = filepath.Join(root, file)
				}
			}
		}
	}
	mergeSettingsMaps(base, overlay)
	encoded.Reset()
	if err := toml.NewEncoder(&encoded).Encode(base); err != nil {
		return settings, err
	}
	var merged Settings
	if _, err := toml.Decode(encoded.String(), &merged); err != nil {
		return settings, err
	}
	return merged, merged.Validate()
}

// Tables merge recursively; every other value (including empty arrays) replaces.
func mergeSettingsMaps(base, overlay map[string]any) {
	for key, value := range overlay {
		src, srcOK := value.(map[string]any)
		dst, dstOK := base[key].(map[string]any)
		if srcOK && dstOK {
			mergeSettingsMaps(dst, src)
		} else {
			base[key] = value
		}
	}
}

type trustRecord struct {
	ConfigSHA256 string `json:"config_sha256"`
}

type trustStore struct {
	Version      int                    `json:"version"`
	Repositories map[string]trustRecord `json:"repositories"`
}

func trustRepoSettings(root string, data []byte, approve func() error) error {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dir = filepath.Join(home, ".local", "state")
	}
	dir = filepath.Join(dir, "opencode-sandbox")
	path := filepath.Join(dir, "trust.json")
	store := trustStore{Version: 1, Repositories: map[string]trustRecord{}}
	stored, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read repo trust: %w", err)
	}
	if err == nil {
		store = trustStore{}
		decoder := json.NewDecoder(bytes.NewReader(stored))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&store); err != nil {
			return fmt.Errorf("decode repo trust: %w", err)
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			return fmt.Errorf("invalid trailing data in repo trust")
		}
		if store.Version != 1 || store.Repositories == nil {
			return fmt.Errorf("invalid repo trust store %s", path)
		}
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	if store.Repositories[root].ConfigSHA256 == hash {
		return nil
	}
	if err := approve(); err != nil {
		return err
	}
	store.Repositories[root] = trustRecord{ConfigSHA256: hash}
	encoded, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create repo trust directory: %w", err)
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
