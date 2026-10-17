package sandbox_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/require"
)

func TestAWSIsNotConfiguredGlobally(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeSettings(t, "[aws]\nsource_profile = 'sandbox-test'\nrole_arn = 'arn:aws:iam::123456789012:role/SandboxReadOnly'\n")
	_, err := sandbox.LoadSettings()
	require.ErrorContains(t, err, "unknown launcher setting")
}

func TestAWSRoleCanUseDefaultHostCredentialsWithoutProfileFlag(t *testing.T) {
	settings, requests := fakeAWSHost(t, "", validAWSResponse)
	settings.SourceProfile = ""
	// The normal host chain selects the default profile when no profile is set.
	require.NoError(t, os.WriteFile(os.Getenv("AWS_CONFIG_FILE"), []byte("[default]\n"), 0600))
	require.NoError(t, os.WriteFile(os.Getenv("AWS_SHARED_CREDENTIALS_FILE"), []byte("[default]\naws_access_key_id = FAKE_DEFAULT_SOURCE_KEY\naws_secret_access_key = FAKE_DEFAULT_SOURCE_SECRET\n"), 0600))
	s := startSandboxWithSettings(t, sandbox.DefaultSettings())
	stop, err := s.StartAWS(context.Background(), settings)
	require.NoError(t, err)
	defer stop()
	require.Contains(t, (<-requests).Authorization, "Credential=FAKE_DEFAULT_SOURCE_KEY/")
}

func TestAWSRoleUsesHostProfileEnvironmentWhenNoProfileFlagIsGiven(t *testing.T) {
	settings, requests := fakeAWSHost(t, "", validAWSResponse)
	settings.SourceProfile = ""
	t.Setenv("AWS_PROFILE", "sandbox-test")
	s := startSandboxWithSettings(t, sandbox.DefaultSettings())
	stop, err := s.StartAWS(context.Background(), settings)
	require.NoError(t, err)
	defer stop()
	require.Contains(t, (<-requests).Authorization, "Credential=FAKE_HOST_SOURCE_KEY/")
}

func TestAWSWritesOnlyRoleCredentialsAndUsesHostRegion(t *testing.T) {
	for _, tc := range []struct{ name, profileRegion, envRegion, defaultEnvRegion, want string }{
		{"profile", "us-east-2", "", "", "us-east-2"},
		{"environment", "us-east-2", "eu-central-1", "", "eu-central-1"},
		{"default environment", "us-east-2", "", "ap-southeast-2", "ap-southeast-2"},
		{"fallback", "", "", "", "eu-west-1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings, requests := fakeAWSHost(t, tc.profileRegion, validAWSResponse)
			t.Setenv("AWS_REGION", tc.envRegion)
			t.Setenv("AWS_DEFAULT_REGION", tc.defaultEnvRegion)
			s := startSandboxWithSettings(t, sandbox.DefaultSettings())
			stop, err := s.StartAWS(context.Background(), settings)
			require.NoError(t, err)
			defer stop()
			request := <-requests
			require.Equal(t, "3600", request.Duration)
			require.Equal(t, settings.RoleARN, request.Role)
			require.Equal(t, s.Name, request.Session)
			require.Contains(t, request.Authorization, "Credential=FAKE_HOST_SOURCE_KEY/")
			require.Contains(t, request.Authorization, "/"+tc.want+"/sts/")
			credentials := execInSandbox(t, s, "cat", "/root/.aws/credentials")
			require.Contains(t, credentials, "aws_access_key_id = FAKE_ROLE_KEY_1")
			require.Contains(t, credentials, "aws_secret_access_key = FAKE_ROLE_SECRET_1")
			require.Contains(t, credentials, "aws_session_token = FAKE_ROLE_SESSION_1")
			require.NotContains(t, credentials, "FAKE_HOST_SOURCE")
			require.Equal(t, "700\n600\n600\n", execInSandbox(t, s, "stat", "-c", "%a", "/root/.aws", "/root/.aws/credentials", "/root/.aws/config"))
			require.Equal(t, "FAKE_ROLE_KEY_1\n", execInSandbox(t, s, "aws", "configure", "get", "aws_access_key_id"))
			require.Equal(t, tc.want+"\n", execInSandbox(t, s, "aws", "configure", "get", "region"))
			execInSandbox(t, s, "bash", "-eu", "-c", `test -z "${AWS_SHARED_CREDENTIALS_FILE+x}"`)
			execInSandbox(t, s, "test", "!", "-e", os.Getenv("AWS_SHARED_CREDENTIALS_FILE"))
			require.NotContains(t, storedSandboxConfig(t, s), "FAKE_ROLE_SECRET")
		})
	}
}

func TestAWSRefreshesCredentialsAndStopsRefreshingOnExit(t *testing.T) {
	settings, requests := fakeAWSHost(t, "", func(n int) (time.Time, bool) {
		if n == 1 {
			return time.Now().Add(5*time.Minute + 2*time.Second), true
		}
		return validAWSResponse(n)
	})
	s := startSandboxWithSettings(t, sandbox.DefaultSettings())
	stop, err := s.StartAWS(context.Background(), settings)
	require.NoError(t, err)
	defer stop()
	<-requests
	select {
	case <-requests:
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
	case <-requests:
		t.Fatal("refresh continued after stop")
	case <-time.After(1100 * time.Millisecond):
	}
}

func TestAWSReplacesCredentialsOnReuseAndClearsThemWhenDisabled(t *testing.T) {
	settings, _ := fakeAWSHost(t, "", validAWSResponse)
	s := startSandboxWithSettings(t, sandbox.DefaultSettings())
	stop, err := s.StartAWS(context.Background(), settings)
	require.NoError(t, err)
	stop()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	require.NoError(t, s.Stop(ctx))
	resumed, err := sandbox.Prepare(ctx, s.Worktree, nil, nil, sandbox.DefaultSettings())
	require.NoError(t, err)
	defer resumed.Close()
	stop, err = resumed.StartAWS(ctx, settings)
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
	settings, _ := fakeAWSHost(t, "", func(int) (time.Time, bool) { return time.Time{}, false })
	s := startSandboxWithSettings(t, sandbox.DefaultSettings())
	_, err := s.StartAWS(context.Background(), settings)
	require.ErrorContains(t, err, "assume AWS role failed")
	require.NotContains(t, err.Error(), "FAKE_HOST_SOURCE_SECRET")
	execInSandbox(t, s, "test", "!", "-e", "/root/.aws/credentials")
}

func TestAWSRejectsExpiredRoleCredentials(t *testing.T) {
	settings, _ := fakeAWSHost(t, "", func(int) (time.Time, bool) {
		return time.Now().Add(-time.Minute), true
	})
	s := startSandboxWithSettings(t, sandbox.DefaultSettings())
	_, err := s.StartAWS(context.Background(), settings)
	require.ErrorContains(t, err, "expired role credentials")
	execInSandbox(t, s, "test", "!", "-e", "/root/.aws/credentials")
}

func TestAWSRefreshFailureKeepsOnlyPreviousRoleCredentials(t *testing.T) {
	settings, requests := fakeAWSHost(t, "", func(n int) (time.Time, bool) {
		return time.Now().Add(5*time.Minute + 2*time.Second), n == 1
	})
	s := startSandboxWithSettings(t, sandbox.DefaultSettings())
	stop, err := s.StartAWS(context.Background(), settings)
	require.NoError(t, err)
	defer stop()
	<-requests
	select {
	case <-requests:
	case <-time.After(10*time.Second):
		t.Fatal("expected attempted refresh")
	}
	// Joining the refresh loop also ensures any in-flight failed refresh is done.
	stop()
	credentials := execInSandbox(t, s, "cat", "/root/.aws/credentials")
	require.Contains(t, credentials, "FAKE_ROLE_KEY_1")
	require.NotContains(t, credentials, "FAKE_HOST_SOURCE")
}

func TestAWSDoesNotOverwriteOrRemoveUnmanagedGuestFiles(t *testing.T) {
	settings, _ := fakeAWSHost(t, "", validAWSResponse)
	s := startSandboxWithSettings(t, sandbox.DefaultSettings())
	execInSandbox(t, s, "bash", "-eu", "-c", "mkdir -p /root/.aws; printf 'user-owned' > /root/.aws/credentials")
	_, err := s.StartAWS(context.Background(), settings)
	require.ErrorContains(t, err, "unmanaged")
	stop, err := s.StartAWS(context.Background(), nil)
	require.NoError(t, err)
	stop()
	require.Equal(t, "user-owned", execInSandbox(t, s, "cat", filepath.Join("/root/.aws", "credentials")))
}
