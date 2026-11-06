package config

import (
	_ "embed"
	"strings"
)

//go:embed default-image.txt
var defaultImage string

// DefaultImage returns the Docker image used for fresh VMs by default.
func DefaultImage() string {
	return strings.TrimSpace(defaultImage)
}
