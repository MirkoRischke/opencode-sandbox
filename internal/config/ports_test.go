package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPortConfigurationLayers(t *testing.T) {
	t.Parallel()
	settings, err := mergeSettings(DefaultSettings(), []byte("[[network.host_to_vm]]\nhost_port = 3000\nvm_port = 8080\n[[network.host_to_vm]]\nhost_port = 3001\nvm_port = 8080\n"), "/global")
	require.NoError(t, err)
	require.Equal(t, []PortSettings{{HostPort: 3000, VMPort: 8080}, {HostPort: 3001, VMPort: 8080}}, settings.Network.HostToVM)
	inherited, err := mergeSettings(settings, []byte("[vm]\ncpus = 4\n"), "/workspace")
	require.NoError(t, err)
	require.Equal(t, settings.Network.HostToVM, inherited.Network.HostToVM)
	replaced, err := mergeSettings(settings, []byte("[[network.host_to_vm]]\nhost_port = 4000\nvm_port = 80\n"), "/workspace")
	require.NoError(t, err)
	require.Equal(t, []PortSettings{{HostPort: 4000, VMPort: 80}}, replaced.Network.HostToVM)
	cleared, err := mergeSettings(settings, []byte("[network]\nhost_to_vm = []\n"), "/workspace")
	require.NoError(t, err)
	require.Empty(t, cleared.Network.HostToVM)
}

func TestInvalidPortConfiguration(t *testing.T) {
	t.Parallel()
	for _, data := range []string{
		"[[network.host_to_vm]]\nvm_port = 3000", "[[network.host_to_vm]]\nhost_port = 3000",
		"[[network.host_to_vm]]\nhost_port = 0\nvm_port = 3000", "[[network.host_to_vm]]\nhost_port = -1\nvm_port = 3000",
		"[[network.host_to_vm]]\nhost_port = 65536\nvm_port = 3000", "[[network.host_to_vm]]\nhost_port = 3000\nvm_port = 65536",
		"[[network.host_to_vm]]\nhost_port = '3000'\nvm_port = 3000", "[[network.host_to_vm]]\nhost_port = 3000\nvm_port = 3000\nbind = '0.0.0.0'",
		"[[network.host_to_vm]]\nhost_port = 3000\nvm_port = 80\n[[network.host_to_vm]]\nhost_port = 3000\nvm_port = 81",
		"[[ports]]\nhost = 3000\nguest = 8080", "ports = []",
	} {
		t.Run(data, func(t *testing.T) {
			_, err := mergeSettings(DefaultSettings(), []byte(data), "/workspace")
			require.Error(t, err)
		})
	}
}

func TestVMToHostConfigurationLayers(t *testing.T) {
	t.Parallel()
	require.Equal(t, []uint16{4000}, DefaultSettings().Network.VMToHost.Ports)
	settings, err := mergeSettings(DefaultSettings(), []byte("[network.vm_to_host]\nports = [5432, 6379]"), "/global")
	require.NoError(t, err)
	require.Equal(t, []uint16{5432, 6379}, settings.Network.VMToHost.Ports)
	for _, layer := range []string{"[network]\nunrestricted = true", "[network.vm_to_host]"} {
		inherited, err := mergeSettings(settings, []byte(layer), "/workspace")
		require.NoError(t, err)
		require.Equal(t, settings.Network.VMToHost, inherited.Network.VMToHost)
	}
	replaced, err := mergeSettings(settings, []byte("[network.vm_to_host]\nports = [8080]"), "/workspace")
	require.NoError(t, err)
	require.Equal(t, []uint16{8080}, replaced.Network.VMToHost.Ports)
	cleared, err := mergeSettings(settings, []byte("[network.vm_to_host]\nports = []"), "/workspace")
	require.NoError(t, err)
	require.Empty(t, cleared.Network.VMToHost.Ports)
}

func TestInvalidVMToHostPorts(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"[0]", "[-1]", "[65536]", "['5432']", "5432", "[5432, 5432]", "[1.5]"} {
		t.Run(value, func(t *testing.T) {
			_, err := mergeSettings(DefaultSettings(), []byte("[network.vm_to_host]\nports = "+value), "/workspace")
			require.Error(t, err)
		})
	}
}
