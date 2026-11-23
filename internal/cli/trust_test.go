package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	launcherconfig "opencode-sandbox/internal/config"
)

func TestConfirmWorkspaceConfigAcceptsExplicitApproval(t *testing.T) {
	t.Parallel()
	for _, answer := range []string{"y\n", "yes\n", "YES\n"} {
		t.Run(answer, func(t *testing.T) {
			t.Parallel()
			err := confirmWorkspaceConfig(strings.NewReader(answer), &bytes.Buffer{}, "/repo/.opencode-sandbox.toml")
			require.NoError(t, err)
		})
	}
}

func TestConfirmWorkspaceConfigRejectsMissingApproval(t *testing.T) {
	t.Parallel()
	for _, answer := range []string{"n\n", "\n", "", "yes"} {
		t.Run(answer, func(t *testing.T) {
			t.Parallel()
			err := confirmWorkspaceConfig(strings.NewReader(answer), &bytes.Buffer{}, "/repo/.opencode-sandbox.toml")
			require.Error(t, err)
		})
	}
}

func TestWorkspaceApprovalShowsOnlyQuotedPathAndPrompt(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	err := confirmWorkspaceConfig(strings.NewReader("yes\n"), &output, "/repo\x1b[2J/.opencode-sandbox.toml")
	require.NoError(t, err)
	require.Equal(t, "New or changed config: \"/repo\\x1b[2J/.opencode-sandbox.toml\"\n"+
		"Trust this config? [y/N] ", output.String())
}

//nolint:paralleltest // Replaces process-wide os.Stdin to exercise the real approval entry point; concurrent stdin users would observe the test pipe.
func TestApproveWorkspaceConfigRejectsPipedInput(t *testing.T) {
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, reader.Close()) })
	t.Cleanup(func() { assert.NoError(t, writer.Close()) })
	original := os.Stdin
	os.Stdin = reader
	defer func() { os.Stdin = original }()
	_, err = writer.WriteString("yes\n")
	require.NoError(t, err)

	err = configApprover()("/repo/.opencode-sandbox.toml", nil, launcherconfig.DefaultSettings())

	require.ErrorContains(t, err, "launch interactively")
}
