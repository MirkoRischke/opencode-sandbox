package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

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
	merged, err := mergeSettings(settings, []byte(`
[secrets."api.example.com"]
headers = ['x-token']
`), repo)
	require.NoError(t, err)
	require.Len(t, merged.Secrets, 1)
	require.Equal(t, filepath.Join(global, "global-token"), merged.Secrets["api.example.com"].File)
	require.Equal(t, []string{"x-token"}, merged.Secrets["api.example.com"].Headers)
	require.Equal(t, []string{"authorization"}, settings.Secrets["api.example.com"].Headers, "merging must not mutate inherited settings")

	merged, err = mergeSettings(merged, []byte(`
[secrets."api.example.com"]
file = 'repo-token'
`), repo)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(repo, "repo-token"), merged.Secrets["api.example.com"].File)
}
