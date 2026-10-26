package sandbox_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"opencode-sandbox/internal/sandbox"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type awsRequest struct {
	Role, Session, Duration, Authorization string
}

type awsHost struct {
	options  *sandbox.AWSOptions
	profile  string
	env      map[string]string
	requests <-chan awsRequest
}

// A real host AWS SDK talks only to this local fake STS. Never load developer
// profiles, SSO caches, or real credentials in these tests.
func fakeAWSHost(t *testing.T, region string, response func(int) (time.Time, bool)) *awsHost {
	t.Helper()
	dir := t.TempDir()
	credentials := filepath.Join(dir, "credentials")
	config := filepath.Join(dir, "config")
	require.NoError(t, os.WriteFile(credentials, []byte("[sandbox-test]\naws_access_key_id = FAKE_HOST_SOURCE_KEY\naws_secret_access_key = FAKE_HOST_SOURCE_SECRET\n"), 0600))
	contents := "[profile sandbox-test]\n"
	if region != "" {
		contents += "region = " + region + "\n"
	}
	require.NoError(t, os.WriteFile(config, []byte(contents), 0600))
	env := map[string]string{
		"AWS_REGION": "", "AWS_DEFAULT_REGION": "", "AWS_ACCESS_KEY_ID": "",
		"AWS_SECRET_ACCESS_KEY": "", "AWS_SESSION_TOKEN": "", "AWS_PROFILE": "",
		"AWS_CONFIG_FILE": config, "AWS_SHARED_CREDENTIALS_FILE": credentials,
		"AWS_EC2_METADATA_DISABLED": "true",
		"XDG_CONFIG_HOME":           dir, "XDG_STATE_HOME": dir,
		"HERDR_ENV": "",
	}
	requests := make(chan awsRequest, 32)
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ParseForm() != nil || r.Form.Get("Action") != "AssumeRole" {
			http.Error(w, "invalid STS request", http.StatusBadRequest)
			return
		}
		requests <- awsRequest{r.Form.Get("RoleArn"), r.Form.Get("RoleSessionName"), r.Form.Get("DurationSeconds"), r.Header.Get("Authorization")}
		n := int(count.Add(1))
		expires, success := response(n)
		w.Header().Set("Content-Type", "text/xml")
		if !success {
			w.WriteHeader(http.StatusForbidden)
			_, err := fmt.Fprint(w, `<ErrorResponse><Error><Code>AccessDenied</Code><Message>FAKE_HOST_SOURCE_SECRET must not leak</Message></Error></ErrorResponse>`)
			assert.NoError(t, err)
			return
		}
		_, err := fmt.Fprintf(w, `<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><AssumeRoleResult><Credentials><AccessKeyId>FAKE_ROLE_KEY_%d</AccessKeyId><SecretAccessKey>FAKE_ROLE_SECRET_%d</SecretAccessKey><SessionToken>FAKE_ROLE_SESSION_%d</SessionToken><Expiration>%s</Expiration></Credentials></AssumeRoleResult></AssumeRoleResponse>`, n, n, n, expires.UTC().Format(time.RFC3339))
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	env["AWS_ENDPOINT_URL_STS"] = server.URL
	if region == "" {
		region = "eu-west-1"
	}
	client := sts.New(sts.Options{
		Region:       region,
		BaseEndpoint: aws.String(server.URL),
		Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
			return aws.Credentials{AccessKeyID: "FAKE_HOST_SOURCE_KEY", SecretAccessKey: "FAKE_HOST_SOURCE_SECRET"}, nil
		}),
	})
	return &awsHost{
		options: &sandbox.AWSOptions{RoleARN: "arn:aws:iam::123456789012:role/SandboxReadOnly", Region: region, Client: client},
		profile: "sandbox-test",
		env:     env, requests: requests,
	}
}

// Environment overrides belong exclusively to the launcher subprocess, never
// the test process. A controlling terminal is required by the SDK attachment.
func (host *awsHost) run(t *testing.T, s *sandbox.Sandbox, probe string) error {
	t.Helper()
	var args []string
	if host.options != nil {
		args = append(args, "--aws-role", host.options.RoleARN)
		if host.profile != "" {
			args = append(args, "--aws-profile", host.profile)
		}
	}
	return runLauncherProbe(t, s, host.env, probe, args...)
}

func validAWSResponse(_ int) (time.Time, bool) {
	return time.Now().Add(time.Hour), true
}
