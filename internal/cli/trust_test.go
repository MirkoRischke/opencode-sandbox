package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	launcherconfig "opencode-sandbox/internal/config"
)

func TestConfirmWorkspaceConfigAcceptsExplicitApproval(t *testing.T) {
	t.Parallel()
	for _, answer := range []string{"y\n", "yes\n", "YES\n"} {
		t.Run(answer, func(t *testing.T) {
			t.Parallel()
			err := confirmWorkspaceConfig(strings.NewReader(answer), &bytes.Buffer{}, "/repo/.opencode-sandbox.toml", nil, launcherconfig.DefaultSettings())
			require.NoError(t, err)
		})
	}
}

func TestConfirmWorkspaceConfigRejectsMissingApproval(t *testing.T) {
	t.Parallel()
	for _, answer := range []string{"n\n", "\n", "", "yes"} {
		t.Run(answer, func(t *testing.T) {
			t.Parallel()
			err := confirmWorkspaceConfig(strings.NewReader(answer), &bytes.Buffer{}, "/repo/.opencode-sandbox.toml", nil, launcherconfig.DefaultSettings())
			require.Error(t, err)
		})
	}
}

func TestWorkspaceApprovalShowsCredentialAccessWithoutTerminalEscapes(t *testing.T) {
	t.Parallel()
	settings := launcherconfig.DefaultSettings()
	settings.Secrets = map[string]launcherconfig.SecretSettings{"github": {
		File: "creds/token", Env: "GH_TOKEN", AllowedHosts: []string{"api.github.com"}, Headers: []string{"authorization"},
	}}
	var output bytes.Buffer
	err := confirmWorkspaceConfig(strings.NewReader("yes\n"), &output, "/repo/.opencode-sandbox.toml", []byte("# override\n\x1b[2J\n"), settings)
	require.NoError(t, err)
	require.Contains(t, output.String(), "# override")
	require.Contains(t, output.String(), "api.github.com")
	require.Contains(t, output.String(), "creds/token")
	require.Contains(t, output.String(), "host credential access")
	require.Contains(t, output.String(), "\\x1b")
	require.NotContains(t, output.String(), "\x1b")
}

//nolint:paralleltest // Replaces process-wide os.Stdin to exercise the real approval entry point; concurrent stdin users would observe the test pipe.
func TestApproveWorkspaceConfigRejectsPipedInput(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := reader.Close(); err != nil {
			t.Error(err)
		}
	}()
	defer func() {
		if err := writer.Close(); err != nil {
			t.Error(err)
		}
	}()
	original := os.Stdin
	os.Stdin = reader
	defer func() { os.Stdin = original }()
	if _, err := writer.WriteString("yes\n"); err != nil {
		t.Fatal(err)
	}
	err = approveWorkspaceConfig("/repo/.opencode-sandbox.toml", nil, launcherconfig.DefaultSettings())
	if err == nil || !strings.Contains(err.Error(), "launch interactively") {
		t.Fatalf("expected non-interactive refusal, got %v", err)
	}
}
