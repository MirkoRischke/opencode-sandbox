package cli

import (
	"fmt"
	"path/filepath"

	launcherconfig "opencode-sandbox/internal/config"

	"github.com/spf13/cobra"
)

func hostConfigDir(xdg, home string) (string, error) {
	if xdg != "" {
		return xdg, nil
	}
	if home == "" {
		return "", fmt.Errorf("HOME is not defined")
	}
	return filepath.Join(home, ".config"), nil
}

func hostStateDir(xdg, home string) (string, error) {
	if xdg != "" {
		return xdg, nil
	}
	if home == "" {
		return "", fmt.Errorf("HOME is not defined")
	}
	return filepath.Join(home, ".local", "state"), nil
}

func newConfigCommand() *cobra.Command {
	config := &cobra.Command{
		Use:   "config",
		Short: "Inspect launcher configuration",
		Args:  cobra.NoArgs,
		RunE:  requireSubcommand,
	}
	config.AddCommand(&cobra.Command{
		Use:   "default",
		Short: "Print documented launcher defaults without loading configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return launcherconfig.WriteDefaultConfig(cmd.OutOrStdout())
		},
	})
	return config
}
