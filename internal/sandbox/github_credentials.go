package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadGitHubToken reads only the dedicated sandbox credential. Missing means
// authentication is disabled; never fall back to the host gh login.
func LoadGitHubToken() ([]byte, error) {
	dir, err := userConfigDir()
	if err != nil {
		return nil, err
	}
	dir = filepath.Join(dir, "opencode-sandbox", "creds")
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect GitHub credentials directory: %w", err)
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("GitHub credentials directory must be a real owner-only directory (chmod 700): %s", dir)
	}
	path := filepath.Join(dir, "github-token")
	info, err = os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect GitHub token: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("GitHub token must be a regular owner-only file (chmod 600): %s", path)
	}
	if info.Size() > 16384 {
		return nil, fmt.Errorf("GitHub token file is too large")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read GitHub token: %w", err)
	}
	token := strings.TrimSpace(string(data))
	if token == "" || strings.ContainsAny(token, " \t\r\n\x00") {
		return nil, fmt.Errorf("GitHub token must contain one non-empty token")
	}
	return []byte(token), nil
}
