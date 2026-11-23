package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	launcherconfig "opencode-sandbox/internal/config"

	"github.com/BurntSushi/toml"
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
			if _, _, err := inspectConfig(); err != nil {
				return err
			}
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "Configuration valid.")
			return err
		},
	})
	config.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Print merged launcher configuration and list its sources without approving it",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			settings, sources, err := inspectConfig()
			if err != nil {
				return err
			}
			var out bytes.Buffer
			if err := toml.NewEncoder(&out).Encode(settings); err != nil {
				return fmt.Errorf("encode merged configuration: %w", err)
			}
			if _, err := out.WriteTo(cmd.OutOrStdout()); err != nil {
				return err
			}
			var report bytes.Buffer
			fmt.Fprintln(&report)
			fmt.Fprintln(&report, "Configuration sources (merge order):")
			fmt.Fprintln(&report, "  built-in defaults")
			for _, path := range sources {
				fmt.Fprintf(&report, "  %q\n", path)
			}
			_, err = report.WriteTo(cmd.ErrOrStderr())
			return err
		},
	})
	return config
}

func inspectConfig() (launcherconfig.Settings, []string, error) {
	configDir, err := hostConfigDir(os.Getenv("XDG_CONFIG_HOME"), os.Getenv("HOME"))
	if err != nil {
		return launcherconfig.Settings{}, nil, err
	}
	settings, sources, err := (launcherconfig.Paths{ConfigDir: configDir}).InspectWorkspaceSettings(".")
	if err != nil {
		return settings, nil, err
	}
	if _, err := resolveAWSSettings(settings.AWS, nil, nil); err != nil {
		return settings, nil, err
	}
	return settings, sources, nil
}
