package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// AWSOptions contains resolved host configuration; callers own credential and
// region discovery. StartAWS never loads profiles or the process environment.
type AWSOptions struct {
	RoleARN string
	Region  string
	Client  *sts.Client
}

func (a AWSOptions) Validate() error {
	role, err := arn.Parse(a.RoleARN)
	if err != nil {
		return fmt.Errorf("--aws-role must be an IAM role ARN: %w", err)
	}
	if role.Service != "iam" || !strings.HasPrefix(role.Resource, "role/") || len(strings.TrimPrefix(role.Resource, "role/")) == 0 || role.AccountID == "" {
		return fmt.Errorf("--aws-role must be an IAM role ARN")
	}
	return nil
}

// StartAWS obtains credentials on the host and refreshes a guest-only credentials
// file until stopped. Stop refresh before stopping the VM. No AWS settings clears
// only files managed by the launcher.
func (s *Sandbox) StartAWS(ctx context.Context, settings *AWSOptions) (func(), error) {
	if settings == nil {
		s.log().Info("AWS disabled; clearing managed credentials", "component", "aws")
		if err := s.Exec(ctx, nil, nil, nil, "bash", "-eu", "-c", `
test ! -L /root/.aws
if [ -f /root/.aws/.opencode-sandbox-managed ]; then
  rm -f /root/.aws/credentials /root/.aws/config /root/.aws/.opencode-sandbox-managed
fi`); err != nil {
			return nil, fmt.Errorf("clear launcher-managed AWS credentials: %w", err)
		}
		return func() {}, nil
	}
	if err := settings.Validate(); err != nil {
		return nil, err
	}
	initialCtx, cancelInitial := context.WithTimeout(ctx, 45*time.Second)
	defer cancelInitial()
	if settings.Client == nil {
		return nil, fmt.Errorf("AWS client is not configured")
	}
	if settings.Region == "" || strings.ContainsAny(settings.Region, "\r\n\x00") {
		return nil, fmt.Errorf("host AWS region is invalid")
	}

	expires, err := s.refreshAWS(initialCtx, settings)
	if err != nil {
		return nil, err
	}
	refreshCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runAWSRefresh(refreshCtx, settings, expires)
	}()

	return func() {
		cancel()
		<-done
		s.log().Info("AWS credential refresh stopped", "component", "aws")
	}, nil
}

func (s *Sandbox) refreshAWS(ctx context.Context, settings *AWSOptions) (time.Time, error) {
	started := time.Now()
	logger := s.log().With("component", "aws", "role", settings.RoleARN, "region", settings.Region)
	logger.Info("Assuming AWS role")
	refreshCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	result, err := settings.Client.AssumeRole(refreshCtx, &sts.AssumeRoleInput{
		RoleArn: aws.String(settings.RoleARN), RoleSessionName: aws.String(s.Name),
		DurationSeconds: aws.Int32(3600),
	})
	if err != nil {
		return time.Time{}, fmt.Errorf("assume AWS role failed; check host authentication and role permissions: %w", err)
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
	if err := s.writeAWSFiles(refreshCtx, payload, settings.Region); err != nil {
		return time.Time{}, fmt.Errorf("write guest AWS credentials failed; check for unmanaged ~/.aws files: %w", err)
	}
	logger.Info("AWS credentials refreshed", "expires_at", *credentials.Expiration, "duration_ms", time.Since(started).Milliseconds())
	return *credentials.Expiration, nil
}

func (s *Sandbox) runAWSRefresh(ctx context.Context, settings *AWSOptions, expires time.Time) {
	for {
		delay := max(time.Until(expires.Add(-5*time.Minute)), time.Second)
		s.log().Info("AWS credential refresh scheduled", "component", "aws", "refresh_at", time.Now().Add(delay), "expires_at", expires)
		if !waitAWSRefresh(ctx, delay) {
			return
		}
		updated, ok := s.retryAWSRefresh(ctx, settings, expires)
		if !ok {
			return
		}
		expires = updated
	}
}

func (s *Sandbox) retryAWSRefresh(ctx context.Context, settings *AWSOptions, expires time.Time) (time.Time, bool) {
	for {
		updated, err := s.refreshAWS(ctx, settings)
		if ctx.Err() != nil {
			return time.Time{}, false
		}
		if err == nil {
			return updated, true
		}
		s.log().Warn("AWS credential refresh will retry", "component", "aws", "error", err, "retry_in_seconds", 30, "expires_at", expires)
		if !waitAWSRefresh(ctx, 30*time.Second) {
			return time.Time{}, false
		}
	}
}

func waitAWSRefresh(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
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
			_ = s.vm.FS().Remove(cleanupCtx, path) //nolint:errcheck // Best-effort cleanup; renamed files no longer exist at these paths.
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
