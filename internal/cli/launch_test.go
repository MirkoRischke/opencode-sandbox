package cli

import (
	"testing"

	"github.com/stretchr/testify/require"

	"opencode-sandbox/internal/config"
)

func TestResolveAWSSettings(t *testing.T) {
	t.Parallel()
	const role = "arn:aws:iam::123456789012:role/Developer"
	const otherRole = "arn:aws:iam::123456789012:role/Other"
	for _, tt := range []struct {
		name     string
		settings config.AWSSettings
		role     *string
		profile  *string
		want     config.AWSSettings
		wantErr  string
	}{
		{name: "disabled"},
		{name: "config only", settings: config.AWSSettings{Role: role, Profile: "configured"}, want: config.AWSSettings{Role: role, Profile: "configured"}},
		{name: "role only", settings: config.AWSSettings{Role: role}, want: config.AWSSettings{Role: role}},
		{name: "CLI role inherits profile", settings: config.AWSSettings{Role: role, Profile: "configured"}, role: stringPointer(otherRole), want: config.AWSSettings{Role: otherRole, Profile: "configured"}},
		{name: "CLI profile inherits role", settings: config.AWSSettings{Role: role, Profile: "configured"}, profile: stringPointer("override"), want: config.AWSSettings{Role: role, Profile: "override"}},
		{name: "CLI role completes profile config", settings: config.AWSSettings{Profile: "configured"}, role: stringPointer(role), want: config.AWSSettings{Role: role, Profile: "configured"}},
		{name: "clear profile", settings: config.AWSSettings{Role: role, Profile: "configured"}, profile: stringPointer(""), want: config.AWSSettings{Role: role}},
		{name: "disable configured AWS", settings: config.AWSSettings{Role: role, Profile: "configured"}, role: stringPointer(""), profile: stringPointer("")},
		{name: "profile without role", settings: config.AWSSettings{Profile: "configured"}, wantErr: "requires aws.role"},
		{name: "CLI profile without role", profile: stringPointer("override"), wantErr: "requires aws.role"},
		{name: "clear role leaves profile", settings: config.AWSSettings{Role: role, Profile: "configured"}, role: stringPointer(""), wantErr: "requires aws.role"},
		{name: "invalid config role", settings: config.AWSSettings{Role: "not-an-arn"}, wantErr: "aws.role"},
		{name: "invalid CLI role", role: stringPointer("not-an-arn"), wantErr: "aws.role"},
		{name: "CLI repairs config role", settings: config.AWSSettings{Role: "not-an-arn"}, role: stringPointer(role), want: config.AWSSettings{Role: role}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveAWSSettings(tt.settings, tt.role, tt.profile)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func stringPointer(value string) *string { return &value }
