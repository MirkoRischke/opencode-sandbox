package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// LoadCLIConfig selects an explicit file, then sandbox CLI settings, then host
// CLI settings. No file means OpenCode should use its own defaults.
func LoadCLIConfig(path string) ([]byte, error) {
	if path != "" {
		return readCLIConfig(path)
	}
	dir, err := userConfigDir()
	if err != nil {
		return nil, fmt.Errorf("locate CLI config: %w", err)
	}
	for _, name := range []string{"opencode-sandbox", "opencode"} {
		config, err := readCLIConfig(filepath.Join(dir, name, "cli.json"))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		return config, err
	}
	return nil, nil
}

func readCLIConfig(path string) ([]byte, error) {
	config, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read CLI config: %w", err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(config, &object); err != nil || object == nil {
		return nil, fmt.Errorf("CLI config must be a JSON object: %s", path)
	}
	return config, nil
}

// SyncTUIConfig copies only the selected CLI settings into the guest, without
// mounting or modifying the source file. Call before Open.
func (s *Sandbox) SyncTUIConfig(ctx context.Context, config []byte) error {
	if config == nil {
		return s.Exec(ctx, nil, nil, nil, "bash", "-eu", "-c", `
dir="${XDG_CONFIG_HOME:-$HOME/.config}/opencode"
if test -f "$dir/.opencode-sandbox-cli-managed"; then
  rm -f "$dir/cli.json" "$dir/.opencode-sandbox-cli-managed"
fi`)
	}
	if err := s.Exec(ctx, bytes.NewReader(config), nil, nil, "bash", "-eu", "-c", `
umask 077
dir="${XDG_CONFIG_HOME:-$HOME/.config}/opencode"
mkdir -p "$dir"
temp=$(mktemp "$dir/.opencode-sandbox-cli.XXXXXX")
trap 'rm -f "$temp"' EXIT
cat > "$temp"
mv "$temp" "$dir/cli.json"
touch "$dir/.opencode-sandbox-cli-managed"`); err != nil {
		return fmt.Errorf("copy guest TUI config: %w", err)
	}
	return nil
}
