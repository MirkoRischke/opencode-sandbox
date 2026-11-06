package sandbox

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"

	"opencode-sandbox/internal/config"

	m "github.com/superradcompany/microsandbox/sdk/go"
)

// The runtime stores published ports under network.ports, not the SDK's
// creation-only PortBindings field.
func storedPortBindings(data string) ([]m.PortBinding, error) {
	var stored struct {
		Network struct {
			Ports []struct {
				Host uint16 `json:"host_port"`
			} `json:"ports"`
		} `json:"network"`
	}
	if err := json.Unmarshal([]byte(data), &stored); err != nil {
		return nil, fmt.Errorf("read sandbox port mappings: %w", err)
	}
	bindings := make([]m.PortBinding, 0, len(stored.Network.Ports))
	for _, port := range stored.Network.Ports {
		bindings = append(bindings, m.PortBinding{Bind: "127.0.0.1", HostPort: port.Host, Protocol: m.PortProtocolTCP})
	}
	return bindings, nil
}

func portBindings(ports []config.PortSettings) []m.PortBinding {
	bindings := make([]m.PortBinding, 0, len(ports))
	for _, port := range ports {
		bindings = append(bindings, m.PortBinding{
			Bind: "127.0.0.1", HostPort: port.HostPort, GuestPort: port.VMPort, Protocol: m.PortProtocolTCP,
		})
	}
	return bindings
}

// The runtime logs listener bind failures without failing VM startup. Check
// before starting it so ordinary port conflicts are returned to the caller.
func checkPortBindings(bindings []m.PortBinding) error {
	for _, binding := range bindings {
		address := net.JoinHostPort(binding.Bind, strconv.Itoa(int(binding.HostPort)))
		listener, err := net.Listen("tcp4", address)
		if err != nil {
			return fmt.Errorf("host TCP port %s unavailable: %w", address, err)
		}
		if err := listener.Close(); err != nil {
			return fmt.Errorf("release host TCP port %s check: %w", address, err)
		}
	}
	return nil
}
