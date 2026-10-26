package config

import (
	"bytes"
	"errors"
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
	require.Empty(t, settings.Ports, "examples must not expose guest ports")
	require.Contains(t, out.String(), "additional_allowed_hosts = []")
	require.Contains(t, out.String(), "diagnostics = false")
	require.Contains(t, out.String(), "unrestricted = false")
	require.Contains(t, out.String(), "[aws]")
	require.Contains(t, out.String(), `role = ""`)
	require.Contains(t, out.String(), `profile = ""`)
	require.Empty(t, settings.AWS, "defaults must not enable AWS")
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

func TestDefaultNetworkAllowlist(t *testing.T) {
	t.Parallel()
	settings := DefaultSettings()
	require.Equal(t, []string{
		"chatgpt.com", "api.openai.com", "auth.openai.com",
		"models.dev", "mcp.context7.com", "knowledge-mcp.global.api.aws",
		"github.com", "api.github.com", "raw.githubusercontent.com",
		"codeload.github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com",
		"mise-versions.jdx.dev", "dl.google.com", "proxy.golang.org",
		"sum.golang.org", "storage.googleapis.com", "registry.npmjs.org",
		"auth.docker.io", "registry-1.docker.io", "production.cloudflare.docker.com",
		"production.cloudfront.docker.com",
		"docker-images-prod.6aa30f8b08e16409b46e0173d6de2f56.r2.cloudflarestorage.com",
		"public.ecr.aws",
	}, settings.Network.AllowedHosts)
	require.Empty(t, settings.Network.AdditionalAllowedHosts)
	require.False(t, settings.Network.Diagnostics)
	require.False(t, settings.Network.Unrestricted)
	require.Empty(t, settings.Secrets)

	settings.Network.AllowedHosts[0] = "changed.example.com"
	require.Equal(t, "chatgpt.com", DefaultSettings().Network.AllowedHosts[0])
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

func TestUnrestrictedNetworkConfigMerging(t *testing.T) {
	t.Parallel()
	settings := DefaultSettings()
	require.False(t, settings.Network.Unrestricted)
	settings.Network.AllowedHosts = []string{"github.com"}
	settings.Network.AdditionalAllowedHosts = []string{"registry.npmjs.org"}
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
	require.Equal(t, []string{"registry.npmjs.org"}, settings.Network.AdditionalAllowedHosts)
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
	for i, line := range lines {
		if line == "# keyring = {}" {
			continue // Alternative to the file source, not an additional source.
		}
		if strings.HasPrefix(line, "# [[ports]]") || strings.HasPrefix(line, "# host =") || strings.HasPrefix(line, "# guest =") || strings.HasPrefix(line, "# [[files]]") || strings.HasPrefix(line, "# source =") || strings.HasPrefix(line, "# target =") || strings.HasPrefix(line, "# mode =") {
			continue
		}
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

func TestDefaultConfigFileExampleIsValid(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	require.NoError(t, WriteDefaultConfig(&out))
	lines := strings.Split(strings.Replace(out.String(), "files = []\n", "", 1), "\n")
	start := slices.Index(lines, "# [[files]]")
	require.NotEqual(t, -1, start, "file mapping example must be present")
	for i, line := range lines[start:] {
		if strings.HasPrefix(line, "[") {
			break
		}
		content := strings.TrimPrefix(line, "# ")
		if strings.HasPrefix(content, "[[") || strings.Contains(content, " = ") {
			lines[start+i] = content
		}
	}
	settings, err := mergeSettings(DefaultSettings(), []byte(strings.Join(lines, "\n")), t.TempDir())
	require.NoError(t, err)
	require.Len(t, settings.Files, 1)
	require.Equal(t, "/root/.config/opencode/skills", settings.Files[0].Target)
	require.True(t, filepath.IsAbs(settings.Files[0].Source))
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
