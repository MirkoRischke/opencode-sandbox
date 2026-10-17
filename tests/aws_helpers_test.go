package sandbox_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/require"
)

type awsRequest struct {
	Role, Session, Duration, Authorization string
}

// A real host AWS SDK talks only to this local fake STS. Never load developer
// profiles, SSO caches, or real credentials in these tests.
func fakeAWSHost(t *testing.T, region string, response func(int) (time.Time, bool)) (*sandbox.AWSOptions, <-chan awsRequest) {
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
	for _, key := range []string{"AWS_REGION", "AWS_DEFAULT_REGION", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_PROFILE"} {
		t.Setenv(key, "")
	}
	t.Setenv("AWS_CONFIG_FILE", config)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credentials)
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
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
			fmt.Fprint(w, `<ErrorResponse><Error><Code>AccessDenied</Code><Message>FAKE_HOST_SOURCE_SECRET must not leak</Message></Error></ErrorResponse>`)
			return
		}
		fmt.Fprintf(w, `<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><AssumeRoleResult><Credentials><AccessKeyId>FAKE_ROLE_KEY_%d</AccessKeyId><SecretAccessKey>FAKE_ROLE_SECRET_%d</SecretAccessKey><SessionToken>FAKE_ROLE_SESSION_%d</SessionToken><Expiration>%s</Expiration></Credentials></AssumeRoleResult></AssumeRoleResponse>`, n, n, n, expires.UTC().Format(time.RFC3339))
	}))
	t.Cleanup(server.Close)
	t.Setenv("AWS_ENDPOINT_URL_STS", server.URL)
	return &sandbox.AWSOptions{SourceProfile: "sandbox-test", RoleARN: "arn:aws:iam::123456789012:role/SandboxReadOnly"}, requests
}

func validAWSResponse(_ int) (time.Time, bool) {
	return time.Now().Add(time.Hour), true
}
