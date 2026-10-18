package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	launcherconfig "opencode-sandbox/internal/config"
	"opencode-sandbox/internal/sandbox"
)

func launch(ctx context.Context, configPath, cliConfigPath, awsRole, awsProfile string) (err error) {
	if err := identifyHerdrAgent(); err != nil {
		return err
	}
	var awsOptions *sandbox.AWSOptions
	if awsRole != "" || awsProfile != "" {
		awsOptions = &sandbox.AWSOptions{RoleARN: awsRole}
		if err := awsOptions.Validate(); err != nil {
			return err
		}
	}
	configDir, err := hostConfigDir(os.Getenv("XDG_CONFIG_HOME"), os.Getenv("HOME"))
	if err != nil {
		return err
	}
	config, err := sandbox.LoadConfig(configPath, configDir)
	if err != nil {
		return err
	}
	cliConfig, err := sandbox.LoadCLIConfig(cliConfigPath, configDir)
	if err != nil {
		return err
	}
	stateDir, err := hostStateDir(os.Getenv("XDG_STATE_HOME"), os.Getenv("HOME"))
	if err != nil {
		return err
	}
	paths := launcherconfig.Paths{ConfigDir: configDir, StateDir: stateDir}
	settings, err := paths.LoadRepoSettings(".", approveRepoConfig)
	if err != nil {
		return err
	}
	secrets, err := sandbox.LoadSecrets(settings)
	if err != nil {
		return err
	}
	s, err := sandbox.Prepare(ctx, ".", config, secrets, settings)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, s.Shutdown()) }()
	if awsOptions != nil {
		// Resolve the host environment only here; sandbox lifecycle code receives
		// an explicitly configured client and region.
		loadCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		loadOptions := []func(*awsconfig.LoadOptions) error{awsconfig.WithDefaultRegion("eu-west-1")}
		if awsProfile != "" {
			loadOptions = append(loadOptions, awsconfig.WithSharedConfigProfile(awsProfile))
		}
		cfg, err := awsconfig.LoadDefaultConfig(loadCtx, loadOptions...)
		if err != nil {
			return fmt.Errorf("load host AWS profile failed (details suppressed)")
		}
		awsOptions.Region = cfg.Region
		awsOptions.Client = sts.NewFromConfig(cfg)
	}
	stopAWS, err := s.StartAWS(ctx, awsOptions)
	if err != nil {
		return err
	}
	defer stopAWS()
	if err := s.SyncTUIConfig(ctx, cliConfig); err != nil {
		return err
	}
	if err := s.ImportOpenAI(ctx); err != nil {
		return err
	}
	return s.Open(ctx)
}

func identifyHerdrAgent() error {
	if os.Getenv("HERDR_ENV") != "1" || os.Getenv("HERDR_AGENT") != "" {
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("identify Herdr agent: %w", err)
	}
	if err := os.Setenv("HERDR_AGENT", "opencode"); err != nil {
		return fmt.Errorf("identify Herdr agent: %w", err)
	}
	// Herdr reads /proc/<pid>/environ on Linux, which exposes the initial
	// environment rather than later Setenv changes. Exec preserves the PID,
	// terminal, and arguments while making the hint visible to Herdr.
	return syscall.Exec(executable, os.Args, os.Environ())
}
