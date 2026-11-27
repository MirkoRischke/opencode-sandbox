package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGitHubConfigOptInAndInheritance(t *testing.T) {
	t.Parallel()
	settings := DefaultSettings()
	require.Nil(t, settings.GitHub)
	for _, invalid := range []string{"[github]\n", "[github]\nowner = 'acme'\n", "[github]\nowner = '../acme'\napp_id = 123\n"} {
		_, err := mergeSettings(settings, []byte(invalid), "/parent")
		require.ErrorContains(t, err, "github.")
	}
	parent, err := mergeSettings(settings, []byte("[github]\nowner = 'acme'\napp_id = 123\n"), "/parent")
	require.NoError(t, err)
	child, err := mergeSettings(parent, []byte("[network]\nallowed_hosts = ['github.com']\n"), "/parent/child")
	require.NoError(t, err)
	require.Equal(t, &GitHubSettings{Owner: "acme", AppID: 123}, child.GitHub)
	_, err = mergeSettings(parent, []byte("[github]\n"), "/parent/child")
	require.ErrorContains(t, err, "github.owner")
}
