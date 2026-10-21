package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmptySettingsLayerPreservesDefaults(t *testing.T) {
	t.Parallel()
	settings, err := mergeSettings(DefaultSettings(), nil, "/project")
	require.NoError(t, err)
	require.Equal(t, DefaultSettings(), settings)
}

func TestSettingsLayersPreserveSecretNamesAndOrigins(t *testing.T) {
	t.Parallel()
	global := t.TempDir()
	repo := t.TempDir()
	settings, err := mergeSettings(DefaultSettings(), []byte(`
[secrets."api.example.com"]
file = 'global-token'
env = 'API_TOKEN'
allowed_hosts = ['api.example.com']
headers = ['authorization']
`), global)
	require.NoError(t, err)
	_, err = mergeSettings(settings, []byte(`
[secrets."api.example.com"]
headers = ['x-token']
`), repo)
	require.ErrorContains(t, err, "exactly one source")
	merged, err := mergeSettings(settings, []byte(`
[secrets."api.example.com"]
keyring = {}
env = 'REPO_TOKEN'
allowed_hosts = ['repo.example.com']
headers = ['x-token']
`), repo)
	require.NoError(t, err)
	require.Len(t, merged.Secrets, 1)
	require.Empty(t, merged.Secrets["api.example.com"].File)
	require.NotNil(t, merged.Secrets["api.example.com"].Keyring)
	require.Equal(t, []string{"x-token"}, merged.Secrets["api.example.com"].Headers)
	require.Equal(t, []string{"authorization"}, settings.Secrets["api.example.com"].Headers, "merging must not mutate inherited settings")

	merged, err = mergeSettings(merged, []byte(`
[secrets."api.example.com"]
file = 'repo-token'
env = 'FILE_TOKEN'
allowed_hosts = ['file.example.com']
headers = ['authorization']
`), repo)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(repo, "repo-token"), merged.Secrets["api.example.com"].File)
	require.Nil(t, merged.Secrets["api.example.com"].Keyring)
}

func TestSecretEntriesReplaceCompletely(t *testing.T) {
	t.Parallel()
	global, err := mergeSettings(DefaultSettings(), []byte(`
[secrets.github]
keyring = { service = 'company', account = 'work' }
env = 'GH_TOKEN'
allowed_hosts = ['github.com']
headers = ['authorization']
[secrets.other]
file = 'other-token'
env = 'OTHER_TOKEN'
allowed_hosts = ['other.example.com']
headers = ['authorization']
`), "/global")
	require.NoError(t, err)
	for _, source := range []string{"", "keyring = {}\n"} {
		_, err := mergeSettings(global, []byte("[secrets.github]\n"+source), "/repo")
		require.Error(t, err, "incomplete entries must not inherit fields")
	}
	merged, err := mergeSettings(global, []byte(`
[secrets.github]
keyring = {}
env = 'REPO_TOKEN'
allowed_hosts = ['api.github.com']
headers = ['x-token']
`), "/repo")
	require.NoError(t, err)
	require.Equal(t, global.Secrets["other"], merged.Secrets["other"])
	service, account := merged.Secrets["github"].Keyring.Reference("github")
	require.Equal(t, "opencode-sandbox", service)
	require.Equal(t, "github", account)
	require.Equal(t, "company", global.Secrets["github"].Keyring.Service)
}

func TestKeyringSourceConfig(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		"keyring = {}", "keyring = { service = 'company', account = 'work' }",
		"keyring = { service = 'company' }", "keyring = { account = 'work' }",
	} {
		settings, err := mergeSettings(DefaultSettings(), []byte("[secrets.github]\n"+source+"\nenv = 'GH_TOKEN'\nallowed_hosts = ['github.com']\nheaders = ['authorization']\n"), "/global")
		require.NoError(t, err)
		require.NotNil(t, settings.Secrets["github"].Keyring)
	}
	for _, source := range []string{
		"keyring = true", "keyring = false", "keyring = {}\nfile = 'token'",
		"keyring = { unknown = 'field' }", "keyring = { service = '   ' }",
		`keyring = { account = "bad\u0000name" }`,
	} {
		_, err := mergeSettings(DefaultSettings(), []byte("[secrets.github]\n"+source+"\nenv = 'GH_TOKEN'\nallowed_hosts = ['github.com']\nheaders = ['authorization']\n"), "/global")
		require.Error(t, err, source)
	}
}
