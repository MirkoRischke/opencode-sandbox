package sandbox_test

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	launcherconfig "opencode-sandbox/internal/config"
	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	m "github.com/superradcompany/microsandbox/sdk/go"
)

func TestRestrictedDNSForwardsOnlyAllowedNames(t *testing.T) {
	t.Parallel()
	probe, queries := startFilteringDNSProbe(t, launcherconfig.NetworkSettings{AllowedHosts: []string{"allowed.test", ".trusted.test"}})
	for _, transport := range []string{"+notcp", "+tcp"} {
		for _, query := range []struct {
			name    string
			allowed bool
		}{
			{"allowed.test", true}, {"sub.trusted.test", true}, {"secret.attacker.test", false}, {"sub.allowed.test", false}, {"alias.other.test", false},
		} {
			t.Run(transport+"/"+query.name, func(t *testing.T) {
				before := queries()
				output := queryDNS(t, probe, transport, query.name)
				if query.allowed {
					require.Contains(t, output, "status: NOERROR")
					require.Contains(t, output, "1.1.1.1")
					require.Contains(t, queries()[len(before):], query.name, "allowed queries must reach the upstream")
				} else {
					require.Contains(t, output, "status: NXDOMAIN")
					require.Equal(t, before, queries(), "denied queries must never reach the upstream")
				}
			})
		}
	}
}

func TestEmptyDNSAllowlistNeverForwardsQueries(t *testing.T) {
	t.Parallel()
	probe, queries := startFilteringDNSProbe(t, launcherconfig.NetworkSettings{AllowedHosts: []string{}})
	for _, transport := range []string{"+notcp", "+tcp"} {
		for _, name := range []string{"allowed.test", "sub.trusted.test", "secret.attacker.test", "sub.allowed.test", "alias.other.test"} {
			output := queryDNS(t, probe, transport, name)
			require.Contains(t, output, "status: NXDOMAIN", "%s %s", transport, name)
		}
	}
	require.Empty(t, queries(), "an empty allowlist must never forward a query")
}

func TestUnrestrictedDNSForwardsAllNames(t *testing.T) {
	t.Parallel()
	probe, queries := startFilteringDNSProbe(t, launcherconfig.NetworkSettings{AllowedHosts: []string{}, Unrestricted: true})
	for _, transport := range []string{"+notcp", "+tcp"} {
		for _, name := range []string{"allowed.test", "sub.trusted.test", "secret.attacker.test", "sub.allowed.test", "alias.other.test"} {
			before := queries()
			output := queryDNS(t, probe, transport, name)
			require.Contains(t, output, "status: NOERROR", "%s %s", transport, name)
			require.Contains(t, output, "1.1.1.1")
			require.Contains(t, queries()[len(before):], name)
		}
	}
}

func TestAllowedDNSResolvesAliasesOutsideTheAllowlist(t *testing.T) {
	t.Parallel()
	probe, _ := startFilteringDNSProbe(t, launcherconfig.NetworkSettings{AllowedHosts: []string{"allowed.test"}})
	for _, transport := range []string{"+notcp", "+tcp"} {
		output := queryDNS(t, probe, transport, "allowed.test")
		require.Contains(t, output, "status: NOERROR")
		require.Contains(t, output, "alias.other.test.")
		require.Contains(t, output, "1.1.1.1")
	}
}

func TestAllowedDNSCannotBypassProtectedDestinations(t *testing.T) {
	t.Parallel()
	probe, _ := startFilteringDNSProbe(t, launcherconfig.NetworkSettings{AllowedHosts: []string{"protected.test", "host.microsandbox.internal"}})
	for _, host := range []string{"protected.test", "host.microsandbox.internal"} {
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		out, err := probe.Exec(ctx, "curl", []string{"--noproxy", "*", "--insecure", "--max-time", "3", "https://" + host})
		cancel()
		require.NoError(t, err)
		require.NotZero(t, out.ExitCode(), "allowed hostname %s must not bypass protected destinations", host)
	}
}

func startFilteringDNSProbe(t *testing.T, network launcherconfig.NetworkSettings) (*m.Sandbox, func() []string) {
	t.Helper()
	settings := launcherconfig.DefaultSettings()
	settings.Network = network
	source := startSandboxWithSettings(t, settings)
	upstream, queries := startDNSUpstream(t)
	return startDNSProbe(t, source, upstream), queries
}

func queryDNS(t *testing.T, probe *m.Sandbox, transport, name string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	out, err := probe.Exec(ctx, "dig", []string{transport, "+tries=1", "+time=2", name, "A"})
	require.NoError(t, err)
	require.Zero(t, out.ExitCode(), out.Stderr())
	return out.Stdout()
}

// Clone the launcher's actual serialized network policy, changing only the
// trusted upstream. This avoids a production-only hook for DNS test fixtures.
func startDNSProbe(t *testing.T, source *sandbox.Sandbox, upstream string) *m.Sandbox {
	t.Helper()
	var config struct {
		Network struct {
			Policy struct {
				DefaultEgress  m.PolicyAction `json:"default_egress"`
				DefaultIngress m.PolicyAction `json:"default_ingress"`
				Rules          []struct {
					Action      m.PolicyAction                `json:"action"`
					Direction   m.PolicyDirection             `json:"direction"`
					Destination map[string]string             `json:"destination"`
					Protocols   []m.PolicyProtocol            `json:"protocols"`
					Ports       []struct{ Start, End uint16 } `json:"ports"`
				} `json:"rules"`
			} `json:"policy"`
		} `json:"network"`
	}
	require.NoError(t, json.Unmarshal([]byte(storedSandboxConfig(t, source)), &config))
	var sourceConfig m.SandboxConfig
	require.NoError(t, json.Unmarshal([]byte(storedSandboxConfig(t, source)), &sourceConfig))
	network := &m.NetworkConfig{
		DefaultEgress:  config.Network.Policy.DefaultEgress,
		DefaultIngress: config.Network.Policy.DefaultIngress,
		DNS:            &m.DNSConfig{Nameservers: []string{upstream}},
	}
	for _, rule := range config.Network.Policy.Rules {
		var destination string
		for kind, value := range rule.Destination {
			switch kind {
			case "group":
				destination = strings.ReplaceAll(value, "_", "-")
			case "domain", "cidr":
				destination = value
			case "domain_suffix":
				destination = "." + value
			default:
				t.Fatalf("unexpected destination kind %q", kind)
			}
		}
		var ports []string
		for _, port := range rule.Ports {
			ports = append(ports, fmt.Sprintf("%d-%d", port.Start, port.End))
		}
		network.Rules = append(network.Rules, m.PolicyRule{
			Action: rule.Action, Direction: rule.Direction, Destination: destination,
			Protocols: rule.Protocols, Ports: ports,
		})
	}
	name := source.Name + "-dns"
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	// Preparation already imported this identity-addressed image. Reuse that
	// reference rather than assuming the original Docker tag is in the native cache.
	require.NotEmpty(t, sourceConfig.Image)
	probe, err := m.CreateSandbox(ctx, name, m.WithImage(sourceConfig.Image), m.WithPullPolicy(m.PullPolicyNever),
		m.WithMemory(512), m.WithRootDisk(m.RootDisk.Managed(1024)), m.WithDetached(),
		m.WithNetwork(network), m.WithInterceptTLS())
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		assert.NoError(t, probe.Destroy(ctx))
		assert.NoError(t, probe.Close())
	})
	return probe
}

// A deterministic UDP/TCP resolver: records every query and returns a public A
// record, with an out-of-allowlist CNAME for allowed.test and a private answer
// for protected.test. No external DNS service is involved.
func startDNSUpstream(t *testing.T) (string, func() []string) {
	t.Helper()
	tcp, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	udp, err := net.ListenPacket("udp", tcp.Addr().String())
	require.NoError(t, err)
	var mu sync.Mutex
	queries := []string{}
	respond := func(query []byte) []byte {
		name, end := dnsQuestion(query)
		if end == 0 {
			return nil
		}
		mu.Lock()
		queries = append(queries, name)
		mu.Unlock()
		return dnsResponse(query, name, end)
	}
	var workers sync.WaitGroup
	workers.Add(2)
	t.Cleanup(func() {
		assert.NoError(t, udp.Close())
		assert.NoError(t, tcp.Close())
		workers.Wait()
	})
	go func() { defer workers.Done(); serveDNSUDP(t, udp, respond) }()
	go func() { defer workers.Done(); serveDNSTCP(t, tcp, respond) }()
	return tcp.Addr().String(), func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string{}, queries...)
	}
}

func dnsQuestion(query []byte) (string, int) {
	if len(query) < 17 {
		return "", 0
	}
	var labels []string
	end := 12
	for query[end] != 0 {
		length := int(query[end])
		end++
		if length > 63 || end+length >= len(query) {
			return "", 0
		}
		labels = append(labels, string(query[end:end+length]))
		end += length
	}
	end += 5 // terminating zero, type, class
	if end > len(query) {
		return "", 0
	}
	return strings.Join(labels, "."), end
}

func dnsResponse(query []byte, name string, end int) []byte {
	response := append([]byte(nil), query[:end]...)
	binary.BigEndian.PutUint16(response[2:4], 0x8180)
	binary.BigEndian.PutUint16(response[6:8], 1)
	clear(response[8:12])
	owner := []byte{0xc0, 0x0c} // question name
	if name == "allowed.test" {
		alias := []byte("\x05alias\x05other\x04test\x00")
		binary.BigEndian.PutUint16(response[6:8], 2)
		response = append(response, owner...)
		response = append(response, 0, 5, 0, 1, 0, 0, 0, 60, 0, byte(len(alias)))
		response = append(response, alias...)
		owner = alias
	}
	response = append(response, owner...)
	response = append(response, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4)
	if name == "protected.test" {
		return append(response, 127, 0, 0, 1)
	}
	return append(response, 1, 1, 1, 1)
}

func serveDNSUDP(t *testing.T, udp net.PacketConn, respond func([]byte) []byte) {
	t.Helper()
	buffer := make([]byte, 4096)
	for {
		n, peer, err := udp.ReadFrom(buffer)
		if err != nil {
			return
		}
		_, err = udp.WriteTo(respond(buffer[:n]), peer)
		assert.NoError(t, err)
	}
}

func serveDNSTCP(t *testing.T, tcp net.Listener, respond func([]byte) []byte) {
	t.Helper()
	for {
		conn, err := tcp.Accept()
		if err != nil {
			return
		}
		serveDNSConnection(t, conn, respond)
		assert.NoError(t, conn.Close())
	}
}

func serveDNSConnection(t *testing.T, conn net.Conn, respond func([]byte) []byte) {
	t.Helper()
	for {
		if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
			t.Errorf("set DNS connection deadline: %v", err)
			return
		}
		var size [2]byte
		if _, err := io.ReadFull(conn, size[:]); err != nil {
			return
		}
		query := make([]byte, binary.BigEndian.Uint16(size[:]))
		if _, err := io.ReadFull(conn, query); err != nil {
			return
		}
		response := respond(query)
		binary.BigEndian.PutUint16(size[:], uint16(len(response)))
		_, err := conn.Write(append(size[:], response...))
		assert.NoError(t, err)
	}
}
