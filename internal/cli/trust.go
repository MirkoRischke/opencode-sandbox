package cli

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	launcherconfig "opencode-sandbox/internal/config"

	"github.com/BurntSushi/toml"
	"golang.org/x/term"
)

func approveWorkspaceConfig(path string, data []byte, settings launcherconfig.Settings) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("untrusted workspace config %q; launch interactively to approve it", path)
	}
	return confirmWorkspaceConfig(os.Stdin, os.Stderr, path, data, settings)
}

func confirmWorkspaceConfig(input io.Reader, output io.Writer, path string, data []byte, settings launcherconfig.Settings) error {
	var effective bytes.Buffer
	if err := toml.NewEncoder(&effective).Encode(settings); err != nil {
		return err
	}
	var prompt bytes.Buffer
	fmt.Fprintf(&prompt, "Workspace configuration: %q\nOverrides (quoted for safe display):\n", path)
	quoteConfig(&prompt, data)
	fmt.Fprintln(&prompt, "Effective settings (credential values are never loaded for this review):")
	quoteConfig(&prompt, effective.Bytes())
	fmt.Fprint(&prompt, "Trust this workspace configuration, including host file mounts and host credential access? [y/N] ")
	if _, err := output.Write(prompt.Bytes()); err != nil {
		return err
	}
	answer, err := bufio.NewReader(input).ReadString('\n')
	if err != nil {
		return fmt.Errorf("workspace configuration not approved: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return nil
	default:
		return fmt.Errorf("workspace configuration not approved")
	}
}

// Repository content must not be able to inject terminal control sequences into
// the approval prompt or hide credential destinations from the reviewer.
func quoteConfig(output *bytes.Buffer, data []byte) {
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		fmt.Fprintf(output, "  %q\n", line)
	}
}
