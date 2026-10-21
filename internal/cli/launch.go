package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	launcherconfig "opencode-sandbox/internal/config"
	"opencode-sandbox/internal/logging"
	"opencode-sandbox/internal/sandbox"
	"opencode-sandbox/internal/workspace"
)

func launch(ctx context.Context, awsRole, awsProfile *string) (err error) {
	if err := identifyHerdrAgent(); err != nil {
		return err
	}
	configDir, err := hostConfigDir(os.Getenv("XDG_CONFIG_HOME"), os.Getenv("HOME"))
	if err != nil {
		return err
	}
	stateDir, err := hostStateDir(os.Getenv("XDG_STATE_HOME"), os.Getenv("HOME"))
	if err != nil {
		return err
	}
	paths := launcherconfig.Paths{ConfigDir: configDir, StateDir: stateDir}
	w, err := workspace.Resolve(ctx, ".")
	if err != nil {
		return err
	}
	ctx, finish := startLogging(ctx, stateDir, w.Root(), "launch", os.Stderr)
	defer func() { finish(err) }()
	logger := logging.FromContext(ctx)
	paths.Logger = logger
	logger.Info("CLI configuration overrides", "aws_role", awsRole, "aws_profile", awsProfile)
	settings, err := paths.LoadWorkspaceSettings(w.Root(), approveWorkspaceConfig)
	if err != nil {
		return err
	}
	awsSettings, err := resolveAWSSettings(settings.AWS, awsRole, awsProfile)
	if err != nil {
		return err
	}
	settings.AWS = awsSettings
	logger.Info("Effective configuration", "settings", settings)
	var awsOptions *sandbox.AWSOptions
	if awsSettings.Role != "" {
		awsOptions = &sandbox.AWSOptions{RoleARN: awsSettings.Role}
	}
	secrets, err := sandbox.LoadSecrets(settings)
	if err != nil {
		return err
	}
	s, err := sandbox.PrepareWorkspace(ctx, w, secrets, settings, sandbox.DiagnosticsOptions{StateDir: stateDir, Output: os.Stderr})
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
		if awsSettings.Profile != "" {
			loadOptions = append(loadOptions, awsconfig.WithSharedConfigProfile(awsSettings.Profile))
		}
		cfg, err := awsconfig.LoadDefaultConfig(loadCtx, loadOptions...)
		if err != nil {
			logger.Error("Load host AWS configuration failed", "error", err)
			return fmt.Errorf("load host AWS profile failed (details suppressed)")
		}
		// Preserve the SDK's provider/cache behavior, recording only metadata.
		// This distinguishes aws login/SSO from overriding environment keys.
		provider := cfg.Credentials
		cfg.Credentials = aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
			credentials, err := provider.Retrieve(ctx)
			if err != nil {
				logger.Error("Retrieve host AWS credentials failed", "error", err)
			} else {
				logger.Info("Host AWS credentials retrieved", "source", credentials.Source, "can_expire", credentials.CanExpire, "expires_at", credentials.Expires)
			}
			return credentials, err
		})
		awsOptions.Region = cfg.Region
		awsOptions.Client = sts.NewFromConfig(cfg)
		logger.Info("Host AWS configuration loaded", "profile", awsSettings.Profile, "region", cfg.Region)
	}
	stopAWS, err := s.StartAWS(ctx, awsOptions)
	if err != nil {
		return err
	}
	defer stopAWS()
	if err := s.ImportOpenAI(ctx); err != nil {
		return err
	}
	return s.Open(ctx)
}

// Validate AWS only after CLI overrides, so a profile-only config can use a CLI role.
func resolveAWSSettings(settings launcherconfig.AWSSettings, role, profile *string) (launcherconfig.AWSSettings, error) {
	if role != nil {
		settings.Role = *role
	}
	if profile != nil {
		settings.Profile = *profile
	}
	if settings.Role == "" && settings.Profile == "" {
		return settings, nil
	}
	if settings.Role == "" {
		return settings, fmt.Errorf("aws.profile (--aws-profile) requires aws.role or --aws-role")
	}
	if err := (sandbox.AWSOptions{RoleARN: settings.Role}).Validate(); err != nil {
		return settings, fmt.Errorf("aws.role: %w", err)
	}
	return settings, nil
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
