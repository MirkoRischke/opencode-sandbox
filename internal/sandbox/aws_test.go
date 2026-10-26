package sandbox

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"opencode-sandbox/internal/logging"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/stretchr/testify/require"
)

func TestAWSRefreshPreservesProviderFailureWithoutPrintingCredentials(t *testing.T) {
	t.Parallel()
	failure := errors.New("host credential provider unavailable")
	client := sts.NewFromConfig(aws.Config{
		Region: "eu-west-1",
		Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
			return aws.Credentials{AccessKeyID: "SENSITIVE_ACCESS", SecretAccessKey: "SENSITIVE_SECRET"}, failure
		}),
	})
	var captured bytes.Buffer
	s := &Sandbox{Name: "test", logger: logging.Console(&captured, slog.LevelInfo)}

	_, err := s.refreshAWS(t.Context(), &AWSOptions{
		RoleARN: "arn:aws:iam::123456789012:role/Developer", Region: "eu-west-1", Client: client,
	})

	require.ErrorIs(t, err, failure)
	require.ErrorContains(t, err, "assume AWS role failed")
	require.ErrorContains(t, err, failure.Error())
	require.NotContains(t, captured.String(), failure.Error(), "caller handles the returned error")
	for _, value := range []string{"SENSITIVE_ACCESS", "SENSITIVE_SECRET"} {
		require.NotContains(t, err.Error()+captured.String(), value)
	}
}

func TestAWSRoleValidationRetainsParseFailure(t *testing.T) {
	t.Parallel()
	err := (AWSOptions{RoleARN: "not-an-arn"}).Validate()
	require.ErrorContains(t, err, "--aws-role must be an IAM role ARN")
	require.Error(t, errors.Unwrap(err))
}
