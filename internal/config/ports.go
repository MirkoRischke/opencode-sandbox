package config

import "fmt"

type PortSettings struct {
	Host  uint16 `toml:"host" description:"Host TCP port (1-65535), bound only to 127.0.0.1. Must be available when the VM starts."`
	Guest uint16 `toml:"guest" description:"Guest TCP port (1-65535). The application must listen on 0.0.0.0 inside the VM."`
}

func validatePorts(ports []PortSettings) error {
	seen := make(map[uint16]bool, len(ports))
	for i, port := range ports {
		if port.Host == 0 || port.Guest == 0 {
			return fmt.Errorf("ports[%d]: host and guest must be between 1 and 65535", i)
		}
		if seen[port.Host] {
			return fmt.Errorf("ports[%d]: duplicate host TCP port %d", i, port.Host)
		}
		seen[port.Host] = true
	}
	return nil
}
