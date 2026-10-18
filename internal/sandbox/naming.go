package sandbox

import (
	"fmt"
	"path/filepath"
	"strings"

	"opencode-sandbox/internal/workspace"
)

func sandboxName(w workspace.Workspace) string {
	return fmt.Sprintf("oc-%s-%s", namePart(filepath.Base(w.Root())), w.ID()[:8])
}

func namePart(name string) string {
	parts := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
	name = strings.Join(parts, "-")
	if name == "" {
		return "unnamed"
	}
	// Bound both labels so the full name stays below the runtime's 128-byte limit.
	if len(name) > 48 {
		name = strings.TrimRight(name[:48], "-")
	}
	return name
}
