package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// LoadConfig selects an explicit file, then the user's sandbox configuration.
// A missing implicit file means OpenCode should use its own defaults.
func LoadConfig(path, configDir string) ([]byte, error) {
	explicit := path != ""
	if !explicit {
		path = filepath.Join(configDir, "opencode-sandbox", "opencode.json")
	}
	data, err := os.ReadFile(path)
	if !explicit && os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read sandbox config: %w", err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil || object == nil {
		return nil, fmt.Errorf("sandbox config must be a JSON object: %s", path)
	}
	return data, nil
}

func userConfigDir() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".config")
	}
	return dir, nil
}

// configureOpenCode writes only guest-local configuration. The marker lets us remove
// our previous copy when no file is selected, without deleting unmanaged files.
func (s *Sandbox) configureOpenCode(ctx context.Context, config []byte) error {
	if config == nil {
		return s.Exec(ctx, nil, nil, nil, "bash", "-eu", "-c", `
if test -f /etc/opencode/.opencode-sandbox-managed; then
  rm -f /etc/opencode/opencode.json /etc/opencode/.opencode-sandbox-managed
fi`)
	}
	return s.Exec(ctx, bytes.NewReader(config), nil, nil, "bash", "-eu", "-c", `
umask 077
mkdir -p /etc/opencode
cat > /etc/opencode/.opencode-sandbox-config.tmp
mv /etc/opencode/.opencode-sandbox-config.tmp /etc/opencode/opencode.json
touch /etc/opencode/.opencode-sandbox-managed`)
}
