package sandbox_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"opencode-sandbox/internal/config"
	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/require"
)

func availableHostPort(t *testing.T) uint16 {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	address, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok)
	port := uint16(address.Port)
	require.NoError(t, listener.Close())
	return port
}

func hostHTTPService(t *testing.T) (uint16, string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := io.WriteString(w, "host-service"); err != nil {
			t.Errorf("write host response: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	address, ok := server.Listener.Addr().(*net.TCPAddr)
	require.True(t, ok)
	port := uint16(address.Port)
	return port, fmt.Sprintf("http://host.microsandbox.internal:%d", port)
}

func TestVMToHostPorts(t *testing.T) {
	t.Parallel()
	for _, unrestricted := range []bool{false, true} {
		for _, allowHost := range []bool{false, true} {
			t.Run(fmt.Sprintf("unrestricted=%t/allow_host=%t", unrestricted, allowHost), func(t *testing.T) {
				t.Parallel()
				port, allowedURL := hostHTTPService(t)
				_, blockedURL := hostHTTPService(t)
				settings := testSettings(t)
				settings.Network.Unrestricted = unrestricted
				if allowHost {
					settings.Network.VMToHost.Ports = []uint16{port}
				}
				s := prepareSandboxWithSettings(t, newWorktree(t), settings)
				if allowHost {
					require.Equal(t, "host-service", execInSandbox(t, s, "curl", "--noproxy", "*", "--fail", "--silent", "--show-error", "--max-time", "5", allowedURL))
				} else {
					requireNetworkBlocked(t, s, allowedURL)
				}
				requireNetworkBlocked(t, s, blockedURL)
			})
		}
	}
}

func TestVMToHostPortsAfterSnapshotRestoreAndResume(t *testing.T) {
	t.Parallel()
	f := newSnapshotFixture(t, "true")
	port, allowedURL := hostHTTPService(t)
	_, blockedURL := hostHTTPService(t)
	f.settings.Network.VMToHost.Ports = []uint16{port}
	s := prepareSnapshotChild(t, f.ctx, f.worktree, f.settings)
	require.Equal(t, "host-service", execInSandbox(t, s, "curl", "--noproxy", "*", "--fail", "--silent", "--show-error", "--max-time", "5", allowedURL))
	requireNetworkBlocked(t, s, blockedURL)
	require.NoError(t, s.Stop(f.ctx))
	resumed := prepareSandboxWithSettings(t, s.Worktree, f.settings)
	require.Equal(t, "host-service", execInSandbox(t, resumed, "curl", "--noproxy", "*", "--fail", "--silent", "--show-error", "--max-time", "5", allowedURL))
	requireNetworkBlocked(t, resumed, blockedURL)
}

func serveGuestHTTP(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	execInSandbox(t, s, "bash", "-eu", "-c", `
nohup bash -c 'while true; do printf "HTTP/1.1 200 OK\r\nContent-Length: 7\r\nConnection: close\r\n\r\nsandbox" | nc -l -N -s 0.0.0.0 -p 8080; done' >/tmp/port-server.log 2>&1 </dev/null &
for i in {1..100}; do
  if ss -ltn | grep -q ':8080 '; then exit 0; fi
  sleep 0.05
done
exit 1`)
}

func requireForwardedHTTP(t *testing.T, port uint16) {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d", port))
	require.NoError(t, err)
	defer response.Body.Close() //nolint:errcheck // Read-only response body.
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Equal(t, "sandbox", string(body))
}

func TestPortMappingHTTPAndResume(t *testing.T) {
	t.Parallel()
	port := availableHostPort(t)
	settings := testSettings(t)
	settings.Network.AllowedHosts = []string{}
	settings.Network.HostToVM = []config.PortSettings{{HostPort: port, VMPort: 8080}}
	s := prepareSandboxWithSettings(t, newWorktree(t), settings)
	serveGuestHTTP(t, s)
	requireForwardedHTTP(t, port)
	requireNetworkBlocked(t, s, "https://example.org")

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	require.NoError(t, s.Stop(ctx))
	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
	require.NoError(t, err, "stopping the VM must release the host port")
	_, err = sandbox.Prepare(ctx, s.Worktree, nil, settings)
	require.ErrorContains(t, err, "host TCP port")
	require.NoError(t, listener.Close())
	resumed := prepareSandboxWithSettings(t, s.Worktree, settings)
	serveGuestHTTP(t, resumed)
	requireForwardedHTTP(t, port)
}

func TestPortMappingAfterSnapshotRestore(t *testing.T) {
	t.Parallel()
	f := newSnapshotFixture(t, "true")
	port := availableHostPort(t)
	f.settings.Network.HostToVM = []config.PortSettings{{HostPort: port, VMPort: 8080}}
	s := prepareSnapshotChild(t, f.ctx, f.worktree, f.settings)
	serveGuestHTTP(t, s)
	requireForwardedHTTP(t, port)
}

func TestPortMappingWebSocket(t *testing.T) {
	t.Parallel()
	port := availableHostPort(t)
	settings := testSettings(t)
	settings.Network.HostToVM = []config.PortSettings{{HostPort: port, VMPort: 8080}}
	s := startSandboxWithSettings(t, settings)
	// A minimal WebSocket peer: read the upgrade headers and a masked text
	// frame, then return an unmasked text frame over the same TCP connection.
	execInSandbox(t, s, "bash", "-eu", "-c", `
cat >/tmp/websocket-peer.sh <<'PEER'
#!/bin/bash
set -eu
coproc PEER { nc -l -N -s 0.0.0.0 -p 8080; }
exec 3<&"${PEER[0]}" 4>&"${PEER[1]}"
input=3
output=4
while IFS= read -r line <&"$input"; do
  if [[ "$line" == $'\r' ]]; then break; fi
done
printf 'HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=\r\n\r\n' >&"$output"
frame=$(dd bs=1 count=8 <&"$input" 2>/dev/null | od -An -tx1 | tr -d ' \n')
[[ "$frame" == 8182010203046e69 ]]
printf '\201\002ok' >&"$output"
exec {output}>&-
wait
PEER
nohup bash /tmp/websocket-peer.sh >/tmp/websocket-peer.log 2>&1 </dev/null &
for i in {1..100}; do
  if ss -ltn | grep -q ':8080 '; then exit 0; fi
  sleep 0.05
done
exit 1`)
	conn, err := net.DialTimeout("tcp4", fmt.Sprintf("127.0.0.1:%d", port), 5*time.Second)
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck // Read-only test connection cleanup.
	require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))
	_, err = io.WriteString(conn, "GET / HTTP/1.1\r\nHost: localhost\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n")
	require.NoError(t, err)
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, &http.Request{Method: http.MethodGet})
	require.NoError(t, err)
	defer response.Body.Close() //nolint:errcheck // Read-only response body.
	require.Equal(t, http.StatusSwitchingProtocols, response.StatusCode)
	require.Equal(t, "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=", response.Header.Get("Sec-WebSocket-Accept"))
	_, err = conn.Write([]byte{0x81, 0x82, 1, 2, 3, 4, 'o' ^ 1, 'k' ^ 2})
	require.NoError(t, err)
	frame := make([]byte, 4)
	_, err = io.ReadFull(reader, frame)
	require.NoError(t, err)
	require.Equal(t, []byte{0x81, 2, 'o', 'k'}, frame)
}
