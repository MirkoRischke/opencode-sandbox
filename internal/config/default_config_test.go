package config

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
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
	require.Empty(t, settings.Files, "examples must not expose host files")
	require.Empty(t, settings.Network.HostToVM, "examples must not expose VM ports")
	require.Equal(t, []uint16{4000}, settings.Network.VMToHost.Ports, "only the host gateway is exposed by default")
	require.Contains(t, out.String(), "allowed_hosts = []")
	require.Contains(t, out.String(), "diagnostics = false")
	require.Contains(t, out.String(), "unrestricted = false")
	require.Contains(t, out.String(), "[aws]")
	require.Contains(t, out.String(), `role = ""`)
	require.Contains(t, out.String(), `profile = ""`)
	require.Empty(t, settings.AWS, "defaults must not enable AWS")
	require.NotEmpty(t, DefaultImage())
	require.False(t, strings.ContainsAny(DefaultImage(), " \t\r\n"))
	require.Contains(t, out.String(), `image = "`+DefaultImage()+`"`)
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

func TestDefaultNetworkAllowlist(t *testing.T) {
	t.Parallel()
	settings := DefaultSettings()
	require.Empty(t, settings.Network.AllowedHosts)
	require.False(t, settings.Network.Diagnostics)
	require.False(t, settings.Network.Unrestricted)
	require.Empty(t, settings.Secrets)
}

func TestSnapshotCommandConfigMerging(t *testing.T) {
	t.Parallel()
	settings, err := mergeSettings(DefaultSettings(), []byte("[snapshot]\ncommand = './scripts/prepare.sh'\n"), "/project")
	require.NoError(t, err)
	require.Equal(t, "./scripts/prepare.sh", settings.Snapshot.Command)
	settings, err = mergeSettings(settings, []byte("[snapshot]\ncommand = ''\n"), "/project")
	require.NoError(t, err)
	require.Empty(t, settings.Snapshot.Command)
	settings.Snapshot.Command = "echo bad\x00"
	require.ErrorContains(t, settings.Validate(), "snapshot.command")
}

func TestStartupConfigMerging(t *testing.T) {
	t.Parallel()
	defaults := DefaultSettings()
	require.Equal(t, []string{"opencode"}, defaults.Startup.Command)
	settings, err := mergeSettings(defaults, []byte(`
[startup]
command = ["my-agent", "--flag"]
`), "/project")
	require.NoError(t, err)
	require.Equal(t, []string{"my-agent", "--flag"}, settings.Startup.Command)
	settings, err = mergeSettings(settings, []byte(`
[startup]
command = ["other-agent", "two words", ""]
`), "/project")
	require.NoError(t, err)
	require.Equal(t, []string{"other-agent", "two words", ""}, settings.Startup.Command)
}

func TestStartupConfigRejectsInvalidValues(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, setting, want string }{
		{"empty command", `command = []`, "startup.command"},
		{"blank executable", `command = [" "]`, "startup.command"},
		{"NUL in argument", `command = ["agent", "\u0000"]`, "startup.command[1]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := mergeSettings(DefaultSettings(), []byte("[startup]\n"+tc.setting), "/project")
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestUnrestrictedNetworkConfigMerging(t *testing.T) {
	t.Parallel()
	settings := DefaultSettings()
	require.False(t, settings.Network.Unrestricted)
	settings.Network.AllowedHosts = []string{"github.com"}
	settings, err := mergeSettings(settings, []byte("[network]\nunrestricted = true\n"), "/project")
	require.NoError(t, err)
	require.True(t, settings.Network.Unrestricted)
	require.NoError(t, settings.Validate())
	settings, err = mergeSettings(settings, []byte("[network]\ndiagnostics = true\n"), "/project")
	require.NoError(t, err)
	require.True(t, settings.Network.Unrestricted, "omitted fields inherit")
	settings, err = mergeSettings(settings, []byte("[network]\nunrestricted = false\n"), "/project")
	require.NoError(t, err)
	require.False(t, settings.Network.Unrestricted)
	require.Equal(t, []string{"github.com"}, settings.Network.AllowedHosts)
	_, err = mergeSettings(settings, []byte("[network]\nunrestricted = 'true'\n"), "/project")
	require.Error(t, err)
}

func TestEveryConfigFieldHasDocumentation(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	require.NoError(t, WriteDefaultConfig(&out))
	requireDocumentedFields(t, out.String(), reflect.TypeFor[Settings]())
}

func requireDocumentedFields(t *testing.T, output string, typ reflect.Type) {
	t.Helper()
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		name := strings.Split(field.Tag.Get("toml"), ",")[0]
		if name == "-" {
			continue
		}
		require.NotEmpty(t, name, "%s.%s needs a TOML name", typ.Name(), field.Name)
		description := strings.TrimSpace(field.Tag.Get("description"))
		require.NotEmpty(t, description, "%s.%s needs a description", typ.Name(), field.Name)
		require.Contains(t, output, "# "+description)
		switch field.Type.Kind() {
		case reflect.Pointer:
			if _, ok := configExamples[field.Type.Elem()]; ok {
				require.Contains(t, output, "# ["+name+"]")
				requireDocumentedFields(t, output, field.Type.Elem())
				continue
			}
			require.Contains(t, output, name+" =", "%s must be included", name)
		case reflect.Struct:
			requireDocumentedFields(t, output, field.Type)
		case reflect.Map:
			require.Contains(t, configExamples, field.Type.Elem(), "%s needs an example", name)
			requireDocumentedFields(t, output, field.Type.Elem())
		case reflect.Slice:
			require.Contains(t, output, name+" =", "%s must be included", name)
			if field.Type.Elem().Kind() == reflect.Struct {
				require.Contains(t, configExamples, field.Type.Elem())
				requireDocumentedFields(t, output, field.Type.Elem())
			}
		default:
			require.Contains(t, output, name+" =", "%s must be included", name)
		}
	}
}

func TestDefaultConfigSecretExampleIsValid(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	require.NoError(t, WriteDefaultConfig(&out))
	lines := strings.Split(out.String(), "\n")
	start := slices.Index(lines, "# [secrets.example]")
	require.NotEqual(t, -1, start, "secret example must be present")
	for i, line := range lines[start:] {
		if strings.HasPrefix(line, "[") || strings.HasPrefix(line, "# Remove the empty ") {
			break
		}
		if line == "# keyring = {}" {
			continue // Alternative to the file source, not an additional source.
		}
		if content, ok := strings.CutPrefix(line, "# "); ok &&
			(strings.HasPrefix(content, "[") || strings.Contains(content, " = ")) {
			lines[start+i] = content
		}
	}
	var settings Settings
	metadata, err := toml.Decode(strings.Join(lines, "\n"), &settings)
	require.NoError(t, err)
	require.Empty(t, metadata.Undecoded())
	require.NoError(t, settings.Validate())
	require.Equal(t, configExamples[reflect.TypeFor[SecretSettings]()], settings.Secrets["example"])
}

func TestDefaultConfigFileExampleIsValid(t *testing.T) {
	t.Parallel()
	settings := loadDefaultArrayExample(t, "files")
	require.Len(t, settings.Files, 1)
	require.Equal(t, "/root/.config/opencode/skills", settings.Files[0].Target)
	require.True(t, filepath.IsAbs(settings.Files[0].Source))
}

func TestDefaultConfigStateExampleIsValid(t *testing.T) {
	t.Parallel()
	settings := loadDefaultArrayExample(t, "state")
	require.Equal(t, []StateSettings{{Name: "agent-sessions", Target: "/var/lib/agent-sessions"}}, settings.State)
}

func loadDefaultArrayExample(t *testing.T, name string) Settings {
	t.Helper()
	var out bytes.Buffer
	require.NoError(t, WriteDefaultConfig(&out))
	lines := strings.Split(strings.Replace(out.String(), name+" = []\n", "", 1), "\n")
	start := slices.Index(lines, "# [["+name+"]]")
	require.NotEqual(t, -1, start, "array example must be present")
	for i, line := range lines[start:] {
		if line == "" {
			break
		}
		content := strings.TrimPrefix(line, "# ")
		if strings.HasPrefix(content, "[[") || strings.Contains(content, " = ") {
			lines[start+i] = content
		}
	}
	configDir := t.TempDir()
	dir := filepath.Join(configDir, "opencode-sandbox")
	require.NoError(t, os.Mkdir(dir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"), []byte(strings.Join(lines, "\n")), 0600))
	settings, err := LoadSettings(configDir)
	require.NoError(t, err)
	return settings
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
