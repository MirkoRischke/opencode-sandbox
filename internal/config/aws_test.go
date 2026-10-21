package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAWSSettingsLayers(t *testing.T) {
	t.Parallel()
	settings, err := mergeSettings(DefaultSettings(), []byte(`[aws]
role = "arn:aws:iam::123456789012:role/Global"
profile = "global"
`), "/global")
	require.NoError(t, err)
	settings, err = mergeSettings(settings, []byte(`[aws]
role = "arn:aws:iam::123456789012:role/Workspace"
`), "/workspace")
	require.NoError(t, err)
	require.Equal(t, AWSSettings{Role: "arn:aws:iam::123456789012:role/Workspace", Profile: "global"}, settings.AWS)
	settings, err = mergeSettings(settings, []byte("[aws]\nprofile = 'workspace'\n"), "/workspace")
	require.NoError(t, err)
	require.Equal(t, "workspace", settings.AWS.Profile)
	require.Equal(t, "arn:aws:iam::123456789012:role/Workspace", settings.AWS.Role)
	settings, err = mergeSettings(settings, []byte("[aws]\nrole = ''\nprofile = ''\n"), "/workspace")
	require.NoError(t, err)
	require.Empty(t, settings.AWS)
}

func TestAWSSettingsValidationDeferredUntilCLIOverrides(t *testing.T) {
	t.Parallel()
	settings, err := mergeSettings(DefaultSettings(), []byte("[aws]\nprofile = 'source'\n"), "/global")
	require.NoError(t, err)
	require.Equal(t, "source", settings.AWS.Profile)
	_, err = mergeSettings(settings, []byte("[aws]\nrole = 123\n"), "/workspace")
	require.Error(t, err)
	_, err = mergeSettings(settings, []byte("[aws]\naccess_key_id = 'unsupported'\n"), "/workspace")
	require.ErrorContains(t, err, "unknown launcher setting")
}
