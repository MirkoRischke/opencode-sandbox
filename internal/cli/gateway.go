package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	gateway "opencode-sandbox/llm-gateway"

	"github.com/spf13/cobra"
)

func hostGateway() (*gateway.Gateway, error) {
	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		home := os.Getenv("HOME")
		if home == "" {
			return nil, fmt.Errorf("HOME is not defined")
		}
		dataDir = filepath.Join(home, ".local", "share")
	}
	configDir, err := hostConfigDir(os.Getenv("XDG_CONFIG_HOME"), os.Getenv("HOME"))
	if err != nil {
		return nil, err
	}
	options := gateway.Options{
		ProjectName: os.Getenv("OPENCODE_SANDBOX_GATEWAY_PROJECT"),
		ConfigFile:  filepath.Join(configDir, "opencode-sandbox", "llm-gateway", "config.yaml"),
	}
	if value := os.Getenv("OPENCODE_SANDBOX_GATEWAY_PORT"); value != "" {
		port, err := strconv.ParseUint(value, 10, 16)
		if err != nil {
			return nil, fmt.Errorf("parse OPENCODE_SANDBOX_GATEWAY_PORT: %w", err)
		}
		if port == 0 {
			return nil, fmt.Errorf("OPENCODE_SANDBOX_GATEWAY_PORT must be between 1 and 65535")
		}
		options.Port = uint16(port)
	}
	return gateway.NewWithOptions(dataDir, options), nil
}

func newGatewayCommand() *cobra.Command {
	command := &cobra.Command{
		Use: "gateway", Short: "Manage the shared host LLM gateway",
		Args: cobra.NoArgs, RunE: requireSubcommand,
	}
	command.AddCommand(&cobra.Command{
		Use: "start", Short: "Start the LLM gateway and wait until healthy",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			g, err := hostGateway()
			if err != nil {
				return err
			}
			return g.Ensure(cmd.Context(), cmd.ErrOrStderr())
		},
	}, &cobra.Command{
		Use: "stop", Short: "Stop the shared LLM gateway, preserving credentials and data",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			g, err := hostGateway()
			if err != nil {
				return err
			}
			return g.Stop(cmd.Context(), cmd.ErrOrStderr())
		},
	}, &cobra.Command{
		Use: "authenticate [chatgpt|copilot]", Short: "Authenticate a gateway provider using device login (default chatgpt)",
		Args: cobra.MaximumNArgs(1), ValidArgs: []string{"chatgpt", "copilot"},
		RunE: func(cmd *cobra.Command, args []string) error {
			provider := "chatgpt"
			if len(args) != 0 {
				provider = args[0]
			}
			g, err := hostGateway()
			if err != nil {
				return err
			}
			if err := g.Authenticate(cmd.Context(), provider, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s gateway authentication ready.\n", provider)
			return err
		},
	})
	return command
}
