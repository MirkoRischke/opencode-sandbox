package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPortConfigurationLayers(t *testing.T) {
	t.Parallel()
	settings, err := mergeSettings(DefaultSettings(), []byte("[[ports]]\nhost = 3000\nguest = 8080\n[[ports]]\nhost = 3001\nguest = 8080\n"), "/global")
	require.NoError(t, err)
	require.Equal(t, []PortSettings{{Host: 3000, Guest: 8080}, {Host: 3001, Guest: 8080}}, settings.Ports)
	inherited, err := mergeSettings(settings, []byte("[vm]\ncpus = 4\n"), "/workspace")
	require.NoError(t, err)
	require.Equal(t, settings.Ports, inherited.Ports)
	replaced, err := mergeSettings(settings, []byte("[[ports]]\nhost = 4000\nguest = 80\n"), "/workspace")
	require.NoError(t, err)
	require.Equal(t, []PortSettings{{Host: 4000, Guest: 80}}, replaced.Ports)
	cleared, err := mergeSettings(settings, []byte("ports = []\n"), "/workspace")
	require.NoError(t, err)
	require.Empty(t, cleared.Ports)
}

func TestInvalidPortConfiguration(t *testing.T) {
	t.Parallel()
	for _, data := range []string{
		"[[ports]]\nguest = 3000", "[[ports]]\nhost = 3000",
		"[[ports]]\nhost = 0\nguest = 3000", "[[ports]]\nhost = -1\nguest = 3000",
		"[[ports]]\nhost = 65536\nguest = 3000", "[[ports]]\nhost = 3000\nguest = 65536",
		"[[ports]]\nhost = '3000'\nguest = 3000", "[[ports]]\nhost = 3000\nguest = 3000\nbind = '0.0.0.0'",
		"[[ports]]\nhost = 3000\nguest = 80\n[[ports]]\nhost = 3000\nguest = 81",
	} {
		t.Run(data, func(t *testing.T) {
			_, err := mergeSettings(DefaultSettings(), []byte(data), "/workspace")
			require.Error(t, err)
		})
	}
}
