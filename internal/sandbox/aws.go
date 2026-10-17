package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

type AWSOptions struct {
	SourceProfile string
	RoleARN       string
}

func (a AWSOptions) Validate() error {
	role, err := arn.Parse(a.RoleARN)
	if err != nil || role.Service != "iam" || !strings.HasPrefix(role.Resource, "role/") || len(strings.TrimPrefix(role.Resource, "role/")) == 0 || role.AccountID == "" {
		return fmt.Errorf("--aws-role must be an IAM role ARN")
	}
	return nil
}

// StartAWS obtains credentials on the host and refreshes a guest-only credentials
// file until stopped. Stop refresh before stopping the VM. No AWS settings clears
// only files managed by the launcher.
func (s *Sandbox) StartAWS(ctx context.Context, settings *AWSOptions) (func(), error) {
	if settings == nil {
		if s.Exec(ctx, nil, nil, nil, "bash", "-eu", "-c", `
test ! -L /root/.aws
if [ -f /root/.aws/.opencode-sandbox-managed ]; then
  rm -f /root/.aws/credentials /root/.aws/config /root/.aws/.opencode-sandbox-managed
fi`) != nil {
			return nil, fmt.Errorf("clear launcher-managed AWS credentials failed (details suppressed)")
		}
		return func() {}, nil
	}
	if err := settings.Validate(); err != nil {
		return nil, err
	}
	initialCtx, cancelInitial := context.WithTimeout(ctx, 45*time.Second)
	defer cancelInitial()
	loadOptions := []func(*config.LoadOptions) error{config.WithDefaultRegion("eu-west-1")}
	if settings.SourceProfile != "" {
		loadOptions = append(loadOptions, config.WithSharedConfigProfile(settings.SourceProfile))
	}
	cfg, err := config.LoadDefaultConfig(initialCtx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("load host AWS profile failed (details suppressed)")
	}
	if strings.ContainsAny(cfg.Region, "\r\n\x00") {
		return nil, fmt.Errorf("host AWS region is invalid")
	}
	client := sts.NewFromConfig(cfg)
	refresh := func(ctx context.Context) (time.Time, error) {
		refreshCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		result, err := client.AssumeRole(refreshCtx, &sts.AssumeRoleInput{
			RoleArn: aws.String(settings.RoleARN), RoleSessionName: aws.String(s.Name),
			DurationSeconds: aws.Int32(3600),
		})
		if err != nil {
			return time.Time{}, fmt.Errorf("assume AWS role failed; check host authentication and role permissions (details suppressed)")
		}
		if result.Credentials == nil || result.Credentials.Expiration == nil || !result.Credentials.Expiration.After(time.Now()) {
			return time.Time{}, fmt.Errorf("AWS returned missing or expired role credentials")
		}
		credentials := result.Credentials
		values := []string{aws.ToString(credentials.AccessKeyId), aws.ToString(credentials.SecretAccessKey), aws.ToString(credentials.SessionToken)}
		for _, value := range values {
			if value == "" || strings.ContainsAny(value, "\r\n\x00") {
				return time.Time{}, fmt.Errorf("AWS returned invalid role credentials")
			}
		}
		payload := fmt.Sprintf("[default]\naws_access_key_id = %s\naws_secret_access_key = %s\naws_session_token = %s\n", values[0], values[1], values[2])
		if s.writeAWSFiles(refreshCtx, payload, cfg.Region) != nil {
			return time.Time{}, fmt.Errorf("write guest AWS credentials failed; check for unmanaged ~/.aws files (details suppressed)")
		}
		return *credentials.Expiration, nil
	}
	expires, err := refresh(initialCtx)
	if err != nil {
		return nil, err
	}
	refreshCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			delay := max(time.Until(expires.Add(-5*time.Minute)), time.Second)
			timer := time.NewTimer(delay)
			select {
			case <-refreshCtx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			for {
				updated, err := refresh(refreshCtx)
				if refreshCtx.Err() != nil {
					return
				}
				if err == nil {
					expires = updated
					break
				}
				fmt.Fprintln(os.Stderr, "AWS credential refresh failed; retrying in 30s. Existing credentials expire at", expires.UTC().Format(time.RFC3339))
				timer := time.NewTimer(30 * time.Second)
				select {
				case <-refreshCtx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
		}
	}()
	return func() { cancel(); <-done }, nil
}

func (s *Sandbox) writeAWSFiles(ctx context.Context, credentials, region string) error {
	var paths bytes.Buffer
	if err := s.Exec(ctx, nil, &paths, nil, "bash", "-eu", "-c", `
umask 077
test ! -L /root/.aws
mkdir -p /root/.aws
chmod 700 /root/.aws
cd /root/.aws
if [ ! -f .opencode-sandbox-managed ] && { [ -e credentials ] || [ -L credentials ] || [ -e config ] || [ -L config ]; }; then
  exit 1
fi
touch .opencode-sandbox-managed
mktemp /root/.aws/.credentials.XXXXXX
mktemp /root/.aws/.config.XXXXXX
`); err != nil {
		return err
	}
	temporary := strings.Fields(paths.String())
	if len(temporary) != 2 {
		return fmt.Errorf("create guest AWS temporary files failed")
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, path := range temporary {
			_ = s.vm.FS().Remove(cleanupCtx, path)
		}
	}()
	if err := s.vm.FS().WriteString(ctx, temporary[0], credentials); err != nil {
		return err
	}
	if err := s.vm.FS().WriteString(ctx, temporary[1], "[default]\nregion = "+region+"\n"); err != nil {
		return err
	}
	if err := s.vm.FS().Rename(ctx, temporary[1], "/root/.aws/config"); err != nil {
		return err
	}
	return s.vm.FS().Rename(ctx, temporary[0], "/root/.aws/credentials")
}
