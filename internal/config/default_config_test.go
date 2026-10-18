package config

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfigRoundTripsToRuntimeDefaults(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	require.NoError(t, WriteDefaultConfig(&out))
	var settings Settings
	metadata, err := toml.Decode(out.String(), &settings)
	require.NoError(t, err)
	require.Empty(t, metadata.Undecoded())
	require.Equal(t, DefaultSettings(), settings)
	require.NoError(t, settings.Validate())
	require.Empty(t, settings.Secrets, "examples must not enable credentials")
	require.Contains(t, out.String(), "additional_allowed_hosts = []")
	require.Contains(t, out.String(), "diagnostics = false")
	require.Contains(t, out.String(), `image = "opencode-sandbox:poc"`)
	var again bytes.Buffer
	require.NoError(t, WriteDefaultConfig(&again))
	require.Equal(t, out.String(), again.String())
}

func TestNetworkDiagnosticsConfigMerging(t *testing.T) {
	t.Parallel()
	settings, err := mergeSettings(DefaultSettings(), []byte("[network]\ndiagnostics = true\n"), "/project")
	require.NoError(t, err)
	require.True(t, settings.Network.Diagnostics)
	settings, err = mergeSettings(settings, []byte("[network]\ndiagnostics = false\n"), "/project")
	require.NoError(t, err)
	require.False(t, settings.Network.Diagnostics)
}

func TestEveryConfigFieldHasDocumentation(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	require.NoError(t, WriteDefaultConfig(&out))
	var check func(reflect.Type)
	check = func(typ reflect.Type) {
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			name := strings.Split(field.Tag.Get("toml"), ",")[0]
			if name == "-" {
				continue
			}
			require.NotEmpty(t, name, "%s.%s needs a TOML name", typ.Name(), field.Name)
			description := strings.TrimSpace(field.Tag.Get("description"))
			require.NotEmpty(t, description, "%s.%s needs a description", typ.Name(), field.Name)
			require.Contains(t, out.String(), "# "+description)
			switch field.Type.Kind() {
			case reflect.Struct:
				check(field.Type)
			case reflect.Map:
				require.Contains(t, configExamples, field.Type.Elem(), "%s needs an example", name)
				check(field.Type.Elem())
			default:
				require.Contains(t, out.String(), name+" =", "%s must be included", name)
			}
		}
	}
	check(reflect.TypeFor[Settings]())
}

func TestDefaultConfigSecretExampleIsValid(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	require.NoError(t, WriteDefaultConfig(&out))
	lines := strings.Split(out.String(), "\n")
	for i, line := range lines {
		if content, ok := strings.CutPrefix(line, "# "); ok &&
			(strings.HasPrefix(content, "[") || strings.Contains(content, " = ")) {
			lines[i] = content
		}
	}
	var settings Settings
	metadata, err := toml.Decode(strings.Join(lines, "\n"), &settings)
	require.NoError(t, err)
	require.Empty(t, metadata.Undecoded())
	require.NoError(t, settings.Validate())
	require.Equal(t, configExamples[reflect.TypeFor[SecretSettings]()], settings.Secrets["example"])
}

func TestDefaultConfigRejectsMissingMetadata(t *testing.T) {
	t.Parallel()
	for _, value := range []any{
		struct {
			Value int `toml:"value"`
		}{},
		struct {
			Value int `description:"Some setting."`
		}{},
	} {
		var out bytes.Buffer
		err := writeConfigTable(&out, reflect.ValueOf(value), "", false)
		require.ErrorContains(t, err, "requires TOML name and description")
	}
}

type failingConfigWriter struct{}

func (failingConfigWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestDefaultConfigReportsWriteFailure(t *testing.T) {
	t.Parallel()
	require.ErrorContains(t, WriteDefaultConfig(failingConfigWriter{}), "write failed")
}
