package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStateLayersAreOptInAndReplaceInheritedEntries(t *testing.T) {
	t.Parallel()
	defaults := DefaultSettings()
	require.Empty(t, defaults.State)
	global, err := mergeSettings(defaults, []byte("[[state]]\nname = 'history'\ntarget = '/var/lib/history'"), t.TempDir())
	require.NoError(t, err)
	require.Equal(t, []StateSettings{{Name: "history", Target: "/var/lib/history"}}, global.State)
	for _, tc := range []struct {
		name, input string
		want        []StateSettings
	}{
		{"inherit", "", global.State},
		{"replace", "[[state]]\nname = 'other'\ntarget = '/other'", []StateSettings{{Name: "other", Target: "/other"}}},
		{"clear", "state = []", []StateSettings{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings, err := mergeSettings(global, []byte(tc.input), t.TempDir())
			require.NoError(t, err)
			require.Equal(t, tc.want, settings.State)
		})
	}
}

func TestStateValidation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, input, want string }{
		{"unsafe name", "[[state]]\nname = '../outside'\ntarget = '/history'", "state[0].name"},
		{"unclean target", "[[state]]\nname = 'history'\ntarget = '/history/../other'", "state[0].target"},
		{"duplicate name", "[[state]]\nname = 'history'\ntarget = '/history'\n[[state]]\nname = 'history'\ntarget = '/other'", "duplicates"},
		{"nested state", "[[state]]\nname = 'history'\ntarget = '/history'\n[[state]]\nname = 'nested'\ntarget = '/history/sub'", "overlaps state"},
		{"nested file", "[[state]]\nname = 'history'\ntarget = '/history'\n[[files]]\nsource = 'seed'\ntarget = '/history/file'", "overlaps files"},
		{"explicit host source", "[[state]]\nname = 'history'\ntarget = '/history'\nsource = '/host'", "unknown launcher setting"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := mergeSettings(DefaultSettings(), []byte(tc.input), t.TempDir())
			require.ErrorContains(t, err, tc.want)
		})
	}
}
