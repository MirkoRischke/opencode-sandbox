package config

import "fmt"

type PortSettings struct {
	HostPort uint16 `toml:"host_port" description:"Host TCP port (1-65535), bound only to 127.0.0.1. Must be available when the VM starts."`
	VMPort   uint16 `toml:"vm_port" description:"VM TCP port (1-65535). The application must listen on 0.0.0.0 inside the VM."`
}

func validatePorts(ports []PortSettings) error {
	seen := make(map[uint16]bool, len(ports))
	for i, port := range ports {
		if port.HostPort == 0 || port.VMPort == 0 {
			return fmt.Errorf("network.host_to_vm[%d]: host_port and vm_port must be between 1 and 65535", i)
		}
		if seen[port.HostPort] {
			return fmt.Errorf("network.host_to_vm[%d]: duplicate host TCP port %d", i, port.HostPort)
		}
		seen[port.HostPort] = true
	}
	return nil
}
