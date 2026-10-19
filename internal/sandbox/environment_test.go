package sandbox

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnvironmentInstructions(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name        string
		environment environmentInstructions
		contains    []string
		absent      []string
	}{
		{
			name: "restricted",
			environment: environmentInstructions{
				AllowedHosts:   []string{"github.com", "registry.npmjs.org"},
				ReadOnlyMounts: []string{"/reference/docs", "/reference/config.json"},
			},
			contains: []string{"HTTPS/WSS on TCP port 443", "- github.com", "- registry.npmjs.org", "Plain hostnames match exactly", "A leading dot includes the base domain", `- "/reference/docs"`, `- "/reference/config.json"`},
			absent:   []string{"Public internet access is allowed", "Application internet access is blocked", "No additional read-only host mounts"},
		},
		{
			name:        "unrestricted",
			environment: environmentInstructions{Unrestricted: true},
			contains:    []string{"Public internet access is allowed on any port/protocol", "No additional read-only host mounts"},
			absent:      []string{"HTTPS/WSS on TCP port 443", "Application internet access is blocked"},
		},
		{
			name:     "denied",
			contains: []string{"Application internet access is blocked", "No additional read-only host mounts"},
			absent:   []string{"Public internet access is allowed", "HTTPS/WSS on TCP port 443"},
		},
		{
			name:        "quoted mount target",
			environment: environmentInstructions{ReadOnlyMounts: []string{"/reference/line\nbreak"}},
			contains:    []string{`"/reference/line\nbreak"`},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			contents, err := renderEnvironmentInstructions(test.environment)
			require.NoError(t, err)
			text := string(contents)
			for _, expected := range test.contains {
				require.Contains(t, text, expected)
			}
			for _, absent := range test.absent {
				require.NotContains(t, text, absent)
			}
			require.Contains(t, text, "protected destinations remain")
			require.Contains(t, text, "Do not attempt")
			require.Contains(t, text, "Reading these mounts is allowed")
			require.Contains(t, text, "immediately visible on the host")
		})
	}
}
