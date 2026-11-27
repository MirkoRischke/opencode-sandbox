package sandbox

import (
	"net"
	"testing"

	"opencode-sandbox/internal/config"

	"github.com/stretchr/testify/require"
	m "github.com/superradcompany/microsandbox/sdk/go"
)

func TestPortBindingsAreLoopbackTCP(t *testing.T) {
	t.Parallel()
	require.Empty(t, portBindings(nil))
	require.Equal(t, []m.PortBinding{
		{Bind: "127.0.0.1", HostPort: 3000, GuestPort: 8080, Protocol: m.PortProtocolTCP},
	}, portBindings([]config.PortSettings{{HostPort: 3000, VMPort: 8080}}))
}

func TestOccupiedHostPortFailsClearly(t *testing.T) {
	t.Parallel()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, listener.Close()) })
	address, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok)
	port := uint16(address.Port)
	bindings := portBindings([]config.PortSettings{{HostPort: port, VMPort: 8080}})
	err = checkPortBindings(bindings)
	require.ErrorContains(t, err, "host TCP port "+address.String()+" unavailable")
	var opErr *net.OpError
	require.ErrorAs(t, err, &opErr)
}

func TestPortBindingCheckClosesListener(t *testing.T) {
	t.Parallel()
	listener := &portCheckListener{}
	bindings := portBindings([]config.PortSettings{{HostPort: 3000, VMPort: 8080}})
	err := checkPortBindingsWith(bindings, func(network, address string) (net.Listener, error) {
		require.Equal(t, "tcp4", network)
		require.Equal(t, "127.0.0.1:3000", address)
		return listener, nil
	})
	require.NoError(t, err)
	require.True(t, listener.closed)
}

type portCheckListener struct {
	net.Listener
	closed bool
}

func (l *portCheckListener) Close() error {
	l.closed = true
	return nil
}

func TestStoredPortBindings(t *testing.T) {
	t.Parallel()
	bindings, err := storedPortBindings(`{"network":{"ports":[{"host_port":3000,"guest_port":8080,"protocol":"tcp","host_bind":"127.0.0.1"}]}}`)
	require.NoError(t, err)
	require.Equal(t, []m.PortBinding{{Bind: "127.0.0.1", HostPort: 3000, Protocol: m.PortProtocolTCP}}, bindings)
	bindings, err = storedPortBindings(`{"network":{}}`)
	require.NoError(t, err)
	require.Empty(t, bindings)
	_, err = storedPortBindings(`{`)
	require.ErrorContains(t, err, "read sandbox port mappings")
}
