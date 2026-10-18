package sandbox_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	launcherconfig "opencode-sandbox/internal/config"
	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/require"
)

func TestAWSIsNotConfiguredGlobally(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "opencode-sandbox"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "opencode-sandbox", "config.toml"), []byte("[aws]\nsource_profile = 'sandbox-test'\nrole_arn = 'arn:aws:iam::123456789012:role/SandboxReadOnly'\n"), 0600))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, buildLauncher(t))
	command.Dir = newWorktree(t)
	command.Env = append(os.Environ(), "XDG_CONFIG_HOME="+dir, "XDG_STATE_HOME="+dir, "HERDR_ENV=")
	output, err := command.CombinedOutput()
	require.Error(t, err)
	require.Contains(t, string(output), "unknown launcher setting")
}

func TestAWSRoleCanUseDefaultHostCredentialsWithoutProfileFlag(t *testing.T) {
	t.Parallel()
	host := fakeAWSHost(t, "", validAWSResponse)
	host.profile = ""
	require.NoError(t, os.WriteFile(host.env["AWS_CONFIG_FILE"], []byte("[default]\n"), 0600))
	require.NoError(t, os.WriteFile(host.env["AWS_SHARED_CREDENTIALS_FILE"], []byte("[default]\naws_access_key_id = FAKE_DEFAULT_SOURCE_KEY\naws_secret_access_key = FAKE_DEFAULT_SOURCE_SECRET\n"), 0600))
	s := prepareLauncherProbe(t)
	require.NoError(t, host.run(t, s, "true"))
	require.Contains(t, (<-host.requests).Authorization, "Credential=FAKE_DEFAULT_SOURCE_KEY/")
}

func TestAWSRoleUsesHostProfileEnvironmentWhenNoProfileFlagIsGiven(t *testing.T) {
	t.Parallel()
	host := fakeAWSHost(t, "", validAWSResponse)
	host.profile = ""
	host.env["AWS_PROFILE"] = "sandbox-test"
	s := prepareLauncherProbe(t)
	require.NoError(t, host.run(t, s, "true"))
	require.Contains(t, (<-host.requests).Authorization, "Credential=FAKE_HOST_SOURCE_KEY/")
}

func TestAWSWritesOnlyRoleCredentialsAndUsesHostRegion(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, profileRegion, envRegion, defaultEnvRegion, want string }{
		{"profile", "us-east-2", "", "", "us-east-2"},
		{"environment", "us-east-2", "eu-central-1", "", "eu-central-1"},
		{"default environment", "us-east-2", "", "ap-southeast-2", "ap-southeast-2"},
		{"fallback", "", "", "", "eu-west-1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			host := fakeAWSHost(t, tc.profileRegion, validAWSResponse)
			host.env["AWS_REGION"] = tc.envRegion
			host.env["AWS_DEFAULT_REGION"] = tc.defaultEnvRegion
			s := prepareLauncherProbe(t)
			require.NoError(t, host.run(t, s, fmt.Sprintf(`
grep -q '^aws_access_key_id = FAKE_ROLE_KEY_1$' /root/.aws/credentials
grep -q '^aws_secret_access_key = FAKE_ROLE_SECRET_1$' /root/.aws/credentials
grep -q '^aws_session_token = FAKE_ROLE_SESSION_1$' /root/.aws/credentials
if grep -q FAKE_HOST_SOURCE /root/.aws/credentials; then exit 1; fi
test "$(stat -c '%%a' /root/.aws /root/.aws/credentials /root/.aws/config)" = $'700\n600\n600'
test "$(aws configure get aws_access_key_id)" = FAKE_ROLE_KEY_1
test "$(aws configure get region)" = '%s'
test -z "${AWS_SHARED_CREDENTIALS_FILE+x}"
test ! -e '%s'
`, tc.want, host.env["AWS_SHARED_CREDENTIALS_FILE"])))
			request := <-host.requests
			require.Equal(t, "3600", request.Duration)
			require.Equal(t, host.options.RoleARN, request.Role)
			require.Equal(t, s.Name, request.Session)
			require.Contains(t, request.Authorization, "Credential=FAKE_HOST_SOURCE_KEY/")
			require.Contains(t, request.Authorization, "/"+tc.want+"/sts/")
			require.NotContains(t, storedSandboxConfig(t, s), "FAKE_ROLE_SECRET")
		})
	}
}

func TestAWSRefreshesCredentialsAndStopsRefreshingOnExit(t *testing.T) {
	t.Parallel()
	host := fakeAWSHost(t, "", func(n int) (time.Time, bool) {
		if n == 1 {
			return time.Now().Add(5*time.Minute + 2*time.Second), true
		}
		return validAWSResponse(n)
	})
	s := startSandboxWithSettings(t, launcherconfig.DefaultSettings())
	stop, err := s.StartAWS(context.Background(), host.options)
	require.NoError(t, err)
	defer stop()
	<-host.requests
	select {
	case <-host.requests:
	case <-time.After(10 * time.Second):
		t.Fatal("expected automatic refresh before expiration")
	}
	require.Eventually(t, func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		var contents bytes.Buffer
		err := s.Exec(ctx, nil, &contents, nil, "cat", "/root/.aws/credentials")
		return err == nil && strings.Contains(contents.String(), "FAKE_ROLE_KEY_2")
	}, 5*time.Second, 50*time.Millisecond)
	stop()
	select {
	case <-host.requests:
		t.Fatal("refresh continued after stop")
	case <-time.After(1100 * time.Millisecond):
	}
}

func TestAWSReplacesCredentialsOnReuseAndClearsThemWhenDisabled(t *testing.T) {
	t.Parallel()
	host := fakeAWSHost(t, "", validAWSResponse)
	s := startSandboxWithSettings(t, launcherconfig.DefaultSettings())
	stop, err := s.StartAWS(context.Background(), host.options)
	require.NoError(t, err)
	stop()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	require.NoError(t, s.Stop(ctx))
	resumed, err := sandbox.Prepare(ctx, s.Worktree, nil, nil, launcherconfig.DefaultSettings())
	require.NoError(t, err)
	defer func() { require.NoError(t, resumed.Close()) }()
	stop, err = resumed.StartAWS(ctx, host.options)
	require.NoError(t, err)
	stop()
	require.Contains(t, execInSandbox(t, resumed, "cat", "/root/.aws/credentials"), "FAKE_ROLE_KEY_2")
	stop, err = resumed.StartAWS(ctx, nil)
	require.NoError(t, err)
	stop()
	execInSandbox(t, resumed, "test", "!", "-e", "/root/.aws/credentials")
	execInSandbox(t, resumed, "test", "!", "-e", "/root/.aws/config")
}

func TestAWSFailsClosedAndSuppressesUpstreamDetails(t *testing.T) {
	t.Parallel()
	host := fakeAWSHost(t, "", func(int) (time.Time, bool) { return time.Time{}, false })
	s := startSandboxWithSettings(t, launcherconfig.DefaultSettings())
	_, err := s.StartAWS(context.Background(), host.options)
	require.ErrorContains(t, err, "assume AWS role failed")
	require.NotContains(t, err.Error(), "FAKE_HOST_SOURCE_SECRET")
	execInSandbox(t, s, "test", "!", "-e", "/root/.aws/credentials")
}

func TestAWSRejectsExpiredRoleCredentials(t *testing.T) {
	t.Parallel()
	host := fakeAWSHost(t, "", func(int) (time.Time, bool) {
		return time.Now().Add(-time.Minute), true
	})
	s := startSandboxWithSettings(t, launcherconfig.DefaultSettings())
	_, err := s.StartAWS(context.Background(), host.options)
	require.ErrorContains(t, err, "expired role credentials")
	execInSandbox(t, s, "test", "!", "-e", "/root/.aws/credentials")
}

func TestAWSRefreshFailureKeepsOnlyPreviousRoleCredentials(t *testing.T) {
	t.Parallel()
	host := fakeAWSHost(t, "", func(n int) (time.Time, bool) {
		return time.Now().Add(5*time.Minute + 2*time.Second), n == 1
	})
	s := startSandboxWithSettings(t, launcherconfig.DefaultSettings())
	stop, err := s.StartAWS(context.Background(), host.options)
	require.NoError(t, err)
	defer stop()
	<-host.requests
	select {
	case <-host.requests:
	case <-time.After(10 * time.Second):
		t.Fatal("expected attempted refresh")
	}
	stop()
	credentials := execInSandbox(t, s, "cat", "/root/.aws/credentials")
	require.Contains(t, credentials, "FAKE_ROLE_KEY_1")
	require.NotContains(t, credentials, "FAKE_HOST_SOURCE")
}

func TestAWSDoesNotOverwriteOrRemoveUnmanagedGuestFiles(t *testing.T) {
	t.Parallel()
	host := fakeAWSHost(t, "", validAWSResponse)
	s := startSandboxWithSettings(t, launcherconfig.DefaultSettings())
	execInSandbox(t, s, "bash", "-eu", "-c", "mkdir -p /root/.aws; printf 'user-owned' > /root/.aws/credentials")
	_, err := s.StartAWS(context.Background(), host.options)
	require.ErrorContains(t, err, "unmanaged")
	stop, err := s.StartAWS(context.Background(), nil)
	require.NoError(t, err)
	stop()
	require.Equal(t, "user-owned", execInSandbox(t, s, "cat", "/root/.aws/credentials"))
}
