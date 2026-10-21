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

func TestDNSQueriesAreFilteredBeforeForwarding(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"restricted", "empty", "unrestricted"} {
		t.Run(mode, func(t *testing.T) {
			settings := launcherconfig.DefaultSettings()
			settings.Network.AllowedHosts = []string{"allowed.test", ".trusted.test", "protected.test", "host.microsandbox.internal"}
			settings.Network.AdditionalAllowedHosts = []string{}
			settings.Network.Unrestricted = mode == "unrestricted"
			if mode == "empty" {
				settings.Network.AllowedHosts = []string{}
			}
			source := startSandboxWithSettings(t, settings)
			upstream, queries := startDNSUpstream(t)
			probe := startDNSProbe(t, source, settings.VM.Image, upstream)
			for _, transport := range []string{"+notcp", "+tcp"} {
				for _, name := range []string{"allowed.test", "sub.trusted.test", "secret.attacker.test", "sub.allowed.test", "alias.other.test"} {
					allowed := mode == "unrestricted" || mode == "restricted" && (name == "allowed.test" || name == "sub.trusted.test")
					before := queries()
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					out, err := probe.Exec(ctx, "dig", []string{transport, "+tries=1", "+time=2", name, "A"})
					cancel()
					require.NoError(t, err)
					require.Zero(t, out.ExitCode(), out.Stderr())
					if allowed {
						require.Contains(t, out.Stdout(), "status: NOERROR", "%s %s", transport, name)
						require.Contains(t, out.Stdout(), "1.1.1.1")
						require.Contains(t, queries()[len(before):], name, "positive control must reach our upstream")
						if name == "allowed.test" {
							require.Contains(t, out.Stdout(), "alias.other.test.", "CNAME outside allowlist must resolve")
						}
					} else {
						require.Contains(t, out.Stdout(), "status: NXDOMAIN", "%s %s", transport, name)
						require.Equal(t, before, queries(), "denied name must never reach upstream")
					}
				}
			}
			if mode == "restricted" {
				for _, host := range []string{"protected.test", "host.microsandbox.internal"} {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					out, err := probe.Exec(ctx, "curl", []string{"--noproxy", "*", "--insecure", "--max-time", "3", "https://" + host})
					cancel()
					require.NoError(t, err)
					require.NotZero(t, out.ExitCode(), "allowed hostname must not bypass protected destinations")
				}
			}
		})
	}
}

// Clone the launcher's actual serialized network policy, changing only the
// trusted upstream. This avoids a production-only hook for DNS test fixtures.
func startDNSProbe(t *testing.T, source *sandbox.Sandbox, image, upstream string) *m.Sandbox {
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
	probe, err := m.CreateSandbox(ctx, name, m.WithImage(image), m.WithPullPolicy(m.PullPolicyNever),
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
		if len(query) < 17 {
			return nil
		}
		var labels []string
		end := 12
		for query[end] != 0 {
			length := int(query[end])
			end++
			if length > 63 || end+length >= len(query) {
				return nil
			}
			labels = append(labels, string(query[end:end+length]))
			end += length
		}
		end += 5 // terminating zero, type, class
		if end > len(query) {
			return nil
		}
		name := strings.Join(labels, ".")
		mu.Lock()
		queries = append(queries, name)
		mu.Unlock()
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
	var workers sync.WaitGroup
	workers.Add(2)
	t.Cleanup(func() {
		assert.NoError(t, udp.Close())
		assert.NoError(t, tcp.Close())
		workers.Wait()
	})
	go func() {
		defer workers.Done()
		buffer := make([]byte, 4096)
		for {
			n, peer, err := udp.ReadFrom(buffer)
			if err != nil {
				return
			}
			_, err = udp.WriteTo(respond(buffer[:n]), peer)
			assert.NoError(t, err)
		}
	}()
	go func() {
		defer workers.Done()
		for {
			conn, err := tcp.Accept()
			if err != nil {
				return
			}
			for {
				assert.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))
				var size [2]byte
				if _, err := io.ReadFull(conn, size[:]); err != nil {
					break
				}
				query := make([]byte, binary.BigEndian.Uint16(size[:]))
				if _, err := io.ReadFull(conn, query); err != nil {
					break
				}
				response := respond(query)
				binary.BigEndian.PutUint16(size[:], uint16(len(response)))
				_, err = conn.Write(append(size[:], response...))
				assert.NoError(t, err)
			}
			assert.NoError(t, conn.Close())
		}
	}()
	return tcp.Addr().String(), func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string{}, queries...)
	}
}
