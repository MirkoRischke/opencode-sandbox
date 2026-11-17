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
source.file = 'global-token'
guest.env = 'API_TOKEN'
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
source.keyring = {}
guest.env = 'REPO_TOKEN'
allowed_hosts = ['repo.example.com']
headers = ['x-token']
`), repo)
	require.NoError(t, err)
	require.Len(t, merged.Secrets, 1)
	require.Empty(t, merged.Secrets["api.example.com"].Source.File)
	require.NotNil(t, merged.Secrets["api.example.com"].Source.Keyring)
	require.Equal(t, []string{"x-token"}, merged.Secrets["api.example.com"].Headers)
	require.Equal(t, []string{"authorization"}, settings.Secrets["api.example.com"].Headers, "merging must not mutate inherited settings")

	merged, err = mergeSettings(merged, []byte(`
[secrets."api.example.com"]
source.file = 'repo-token'
guest.env = 'FILE_TOKEN'
allowed_hosts = ['file.example.com']
headers = ['authorization']
`), repo)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(repo, "repo-token"), merged.Secrets["api.example.com"].Source.File)
	require.Nil(t, merged.Secrets["api.example.com"].Source.Keyring)
}

func TestSecretEntriesReplaceCompletely(t *testing.T) {
	t.Parallel()
	global, err := mergeSettings(DefaultSettings(), []byte(`
[secrets.github]
source.keyring = { service = 'company', account = 'work' }
guest.env = 'GH_TOKEN'
allowed_hosts = ['github.com']
headers = ['authorization']
[secrets.other]
source.file = 'other-token'
guest.env = 'OTHER_TOKEN'
allowed_hosts = ['other.example.com']
headers = ['authorization']
`), "/global")
	require.NoError(t, err)
	for _, source := range []string{"", "source.keyring = {}\n"} {
		_, err := mergeSettings(global, []byte("[secrets.github]\n"+source), "/repo")
		require.Error(t, err, "incomplete entries must not inherit fields")
	}
	merged, err := mergeSettings(global, []byte(`
[secrets.github]
source.keyring = {}
guest.env = 'REPO_TOKEN'
allowed_hosts = ['api.github.com']
headers = ['x-token']
`), "/repo")
	require.NoError(t, err)
	require.Equal(t, global.Secrets["other"], merged.Secrets["other"])
	service, account := merged.Secrets["github"].Source.Keyring.Reference("github")
	require.Equal(t, "opencode-sandbox", service)
	require.Equal(t, "github", account)
	require.Equal(t, "company", global.Secrets["github"].Source.Keyring.Service)
}

func TestKeyringSourceConfig(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		"source.keyring = {}", "source.keyring = { service = 'company', account = 'work' }",
		"source.keyring = { service = 'company' }", "source.keyring = { account = 'work' }",
	} {
		settings, err := mergeSettings(DefaultSettings(), []byte("[secrets.github]\n"+source+"\nguest.env = 'GH_TOKEN'\nallowed_hosts = ['github.com']\nheaders = ['authorization']\n"), "/global")
		require.NoError(t, err)
		require.NotNil(t, settings.Secrets["github"].Source.Keyring)
	}
	for _, source := range []string{
		"source.keyring = true", "source.keyring = false", "source.keyring = {}\nsource.file = 'token'",
		"source.keyring = { unknown = 'field' }", "source.keyring = { service = '   ' }",
		`source.keyring = { account = "bad\u0000name" }`,
	} {
		_, err := mergeSettings(DefaultSettings(), []byte("[secrets.github]\n"+source+"\nguest.env = 'GH_TOKEN'\nallowed_hosts = ['github.com']\nheaders = ['authorization']\n"), "/global")
		require.Error(t, err, source)
	}
}

func TestSecretSourceAndGuestTables(t *testing.T) {
	t.Parallel()
	settings, err := mergeSettings(DefaultSettings(), []byte(`
[secrets.github]
allowed_hosts = ['api.github.com']
headers = ['authorization']
[secrets.github.source]
keyring = { service = 'company', account = 'work' }
[secrets.github.guest]
env = 'GH_TOKEN'
`), "/global")
	require.NoError(t, err)
	require.Equal(t, SecretSettings{
		Source:       SecretSourceSettings{Keyring: &KeyringSettings{Service: "company", Account: "work"}},
		Guest:        SecretGuestSettings{Env: "GH_TOKEN"},
		AllowedHosts: []string{"api.github.com"},
		Headers:      []string{"authorization"},
	}, settings.Secrets["github"])
}

func TestSecretRejectsFlatAndUnknownFields(t *testing.T) {
	t.Parallel()
	for _, field := range []string{
		"file = 'token'", "keyring = {}", "env = 'TOKEN'",
		"source.unknown = 'token'", "guest.unknown = 'TOKEN'",
	} {
		t.Run(field, func(t *testing.T) {
			_, err := mergeSettings(DefaultSettings(), []byte("[secrets.github]\n"+field+"\n"), "/global")
			require.ErrorContains(t, err, "unknown launcher setting")
		})
	}
}
