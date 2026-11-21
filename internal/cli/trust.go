package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	launcherconfig "opencode-sandbox/internal/config"

	"golang.org/x/term"
)

func approveWorkspaceConfig(path string, _ []byte, _ launcherconfig.Settings) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("untrusted workspace config %q; launch interactively to approve it", path)
	}
	return confirmWorkspaceConfig(os.Stdin, os.Stderr, path)
}

func confirmWorkspaceConfig(input io.Reader, output io.Writer, path string) error {
	if _, err := fmt.Fprintf(output, "Workspace config is new or changed: %q\nTrust this config? [y/N] ", path); err != nil {
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
