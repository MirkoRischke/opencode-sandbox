package cli

import (
	"fmt"
	"os"
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
	config.AddCommand(&cobra.Command{
		Use:   "validate",
		Short: "Validate global and current-directory configuration without launching or approving it",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			configDir, err := hostConfigDir(os.Getenv("XDG_CONFIG_HOME"), os.Getenv("HOME"))
			if err != nil {
				return err
			}
			settings, err := (launcherconfig.Paths{ConfigDir: configDir}).ValidateWorkspaceSettings(".")
			if err != nil {
				return err
			}
			if _, err := resolveAWSSettings(settings.AWS, nil, nil); err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "Configuration valid.")
			return err
		},
	})
	return config
}
