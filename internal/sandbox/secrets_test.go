package sandbox

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"opencode-sandbox/internal/config"

	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

func TestLoadSecretsPreservesHostPatterns(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0700))
	path := filepath.Join(dir, "token")
	require.NoError(t, os.WriteFile(path, []byte("fake-token"), 0600))
	hosts := []string{"Example.com", "*.Example.com"}
	settings := config.DefaultSettings()
	settings.Secrets = map[string]config.SecretSettings{
		"test": {File: path, Env: "TOKEN", AllowedHosts: hosts, Headers: []string{"authorization"}},
	}
	secrets, err := LoadSecrets(settings)
	require.NoError(t, err)
	require.Len(t, secrets, 1)
	require.Equal(t, hosts, secrets[0].entry.Allow)
}

func TestGitHubTokenRejectsUnsafePermissionsAndInvalidContents(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name              string
		dirMode, fileMode os.FileMode
		contents          string
	}{
		{"readable directory", 0755, 0600, "github_pat_FAKE"},
		{"readable token", 0700, 0644, "github_pat_FAKE"},
		{"empty token", 0700, 0600, ""},
		{"multiple lines", 0700, 0600, "first\nsecond"},
		{"NUL byte", 0700, 0600, "first\x00second"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "github-token")
			require.NoError(t, os.WriteFile(path, []byte(tc.contents), 0600))
			require.NoError(t, os.Chmod(path, tc.fileMode))
			require.NoError(t, os.Chmod(dir, tc.dirMode))

			_, err := readSecretFile(path)

			require.Error(t, err)
			if tc.contents != "" {
				require.NotContains(t, err.Error(), tc.contents)
			}
		})
	}
}

func TestGitHubTokenRejectsSymlinks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0700))
	path := filepath.Join(dir, "github-token")
	target := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(target, []byte("github_pat_FAKE"), 0600))
	require.NoError(t, os.Symlink(target, path))

	_, err := readSecretFile(path)

	require.ErrorContains(t, err, "regular owner-only file")
}

func TestLoadKeyringSecrets(t *testing.T) {
	t.Parallel()
	for _, ref := range []*config.KeyringSettings{{}, {Service: "company", Account: "work"}} {
		settings := config.DefaultSettings()
		settings.Secrets = map[string]config.SecretSettings{
			"github": {Keyring: ref, Env: "GH_TOKEN", AllowedHosts: []string{"*.Example.com"}, Headers: []string{"Authorization"}},
		}
		calls := 0
		secrets, err := loadSecrets(settings, func(service, account string) (string, error) {
			calls++
			wantService, wantAccount := ref.Reference("github")
			require.Equal(t, wantService, service)
			require.Equal(t, wantAccount, account)
			return "FAKE_TOKEN", nil
		})
		require.NoError(t, err)
		require.Equal(t, 1, calls)
		require.Len(t, secrets, 1)
		require.Equal(t, []string{"*.Example.com"}, secrets[0].entry.Allow)
		// File and keyring sources produce the exact same proxy definition.
		dir := t.TempDir()
		require.NoError(t, os.Chmod(dir, 0700))
		path := filepath.Join(dir, "token")
		require.NoError(t, os.WriteFile(path, []byte("FAKE_TOKEN\n"), 0600))
		file := settings.Secrets["github"]
		file.Keyring, file.File = nil, path
		settings.Secrets["github"] = file
		fromFile, err := loadSecrets(settings, func(string, string) (string, error) {
			t.Fatal("file secrets must not access the keyring")
			return "", nil
		})
		require.NoError(t, err)
		require.Equal(t, fromFile, secrets)
	}
}

func TestLoadKeyringSecretsFailsClosed(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		value   string
		err     error
		message string
	}{
		{err: keyring.ErrNotFound, message: "entry not found"},
		{err: errors.New("keyring provider unavailable"), message: "keyring access failed"},
		{value: "", message: "one non-empty line"},
		{value: "SENSITIVE_VALUE\nother", message: "one non-empty line"},
		{value: "SENSITIVE_VALUE\x00", message: "one non-empty line"},
		{value: strings.Repeat("x", 16385), message: "too large"},
	} {
		settings := config.DefaultSettings()
		settings.Secrets = map[string]config.SecretSettings{
			"github": {Keyring: &config.KeyringSettings{}, Env: "GH_TOKEN", AllowedHosts: []string{"github.com"}, Headers: []string{"authorization"}},
		}
		secrets, err := loadSecrets(settings, func(string, string) (string, error) { return tc.value, tc.err })
		require.ErrorContains(t, err, tc.message)
		if tc.err != nil {
			require.ErrorIs(t, err, tc.err)
			require.ErrorContains(t, err, tc.err.Error())
		}
		require.NotContains(t, err.Error(), "SENSITIVE_VALUE")
		require.Nil(t, secrets)
	}
}
