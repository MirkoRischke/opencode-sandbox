package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	launcherconfig "opencode-sandbox/internal/config"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/require"
)

func TestConfigDirectoryUsesXDGWhenSet(t *testing.T) {
	t.Parallel()
	want := t.TempDir()
	dir, err := hostConfigDir(want, "")
	require.NoError(t, err)
	require.Equal(t, want, dir)
}

func TestStateDirectoryUsesXDGWhenSet(t *testing.T) {
	t.Parallel()
	want := t.TempDir()
	dir, err := hostStateDir(want, "")
	require.NoError(t, err)
	require.Equal(t, want, dir)
}

func TestSettingsUseHomeWhenXDGIsUnset(t *testing.T) {
	t.Parallel()
	dir, err := hostConfigDir("", t.TempDir())
	require.NoError(t, err)
	path := filepath.Join(dir, "opencode-sandbox", "config.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	require.NoError(t, os.WriteFile(path, []byte("[network]\nallowed_hosts = []\n"), 0600))
	settings, err := launcherconfig.LoadSettings(dir)
	require.NoError(t, err)
	require.Empty(t, settings.Network.AllowedHosts)
}

func TestRepoSettingsTrustFallsBackToHome(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	state, err := hostStateDir("", home)
	require.NoError(t, err)
	root := t.TempDir()
	output, err := exec.Command("git", "-C", root, "init").CombinedOutput()
	require.NoError(t, err, "%s", output)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".opencode-sandbox.toml"), []byte("# config\n"), 0600))
	paths := launcherconfig.Paths{ConfigDir: t.TempDir(), StateDir: state}
	_, err = paths.LoadWorkspaceSettings(root, func(string, []byte, launcherconfig.Settings) error { return nil })
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(home, ".local/state/opencode-sandbox/trust.json"))
	require.NoError(t, err)
}

func TestConfigValidate(t *testing.T) {
	if os.Getenv("OPENCODE_SANDBOX_VALIDATE_TEST") == "1" {
		output, err := executeCommand("config", "validate")
		if wantError := os.Getenv("OPENCODE_SANDBOX_VALIDATE_ERROR"); wantError != "" {
			require.ErrorContains(t, err, wantError)
			require.Empty(t, output)
		} else {
			require.NoError(t, err)
			require.Equal(t, "Configuration valid.\n", output)
		}
		return
	}
	t.Parallel()
	for _, tc := range []struct {
		name      string
		global    string
		workspace string
		wantError string
	}{
		{name: "defaults"},
		{name: "workspace overrides global", global: "[aws]\nprofile = 'dev'\n", workspace: "[aws]\nrole = 'arn:aws:iam::123456789012:role/dev'\n[logging]\nconsole_level = 'debug'\n"},
		{name: "invalid global syntax", global: "[", wantError: `opencode-sandbox/config.toml": toml:`},
		{name: "invalid workspace syntax", workspace: "[", wantError: `.opencode-sandbox.toml": toml:`},
		{name: "unknown key", workspace: "[vm]\ncores = 2\n", wantError: "unknown launcher setting: vm.cores"},
		{name: "incorrect type", workspace: "[vm]\ncpus = 'two'\n", wantError: "vm.cpus"},
		{name: "resource limit", workspace: "[vm]\ncpus = 0\n", wantError: "vm.cpus must be greater than zero"},
		{name: "port range", workspace: "[network.vm_to_host]\nports = [0]\n", wantError: "port must be between 1 and 65535"},
		{name: "invalid logging", workspace: "[logging]\nconsole_level = 'trace'\n", wantError: "logging.console_level"},
		{name: "aws dependency", workspace: "[aws]\nprofile = 'dev'\n", wantError: "requires aws.role"},
		{name: "invalid role", workspace: "[aws]\nrole = 'invalid'\n", wantError: "aws.role"},
		{name: "invalid secret", workspace: "[secrets.test]\nguest.env = 'TOKEN'\n", wantError: "requires exactly one source"},
		{name: "overlapping mappings", workspace: "[[files]]\nsource = '.'\ntarget = '/opt/test'\n[[files]]\nsource = '.'\ntarget = '/opt/test/sub'\n", wantError: "overlap"},
		{name: "missing file source", workspace: "[[files]]\nsource = './missing'\ntarget = '/opt/test'\n", wantError: "files[0].source"},
		{name: "credentials not read", workspace: "[secrets.test]\nsource.file = './missing-credentials'\nguest.env = 'TOKEN'\nallowed_hosts = ['example.com']\nheaders = ['Authorization']\n[secrets.other]\nsource.keyring = {}\nguest.env = 'OTHER_TOKEN'\nallowed_hosts = ['example.com']\nheaders = ['Authorization']\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			configDir := t.TempDir()
			stateDir := t.TempDir()
			if tc.global != "" {
				dir := filepath.Join(configDir, "opencode-sandbox")
				require.NoError(t, os.Mkdir(dir, 0700))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"), []byte(tc.global), 0600))
			}
			if tc.workspace != "" {
				require.NoError(t, os.WriteFile(filepath.Join(root, ".opencode-sandbox.toml"), []byte(tc.workspace), 0600))
			}
			// A corrupt trust store must neither block validation nor be modified.
			dir := filepath.Join(stateDir, "opencode-sandbox")
			require.NoError(t, os.Mkdir(dir, 0700))
			trust := filepath.Join(dir, "trust.json")
			require.NoError(t, os.WriteFile(trust, []byte("invalid trust"), 0600))
			executable, err := os.Executable()
			require.NoError(t, err)
			command := exec.CommandContext(t.Context(), executable, "-test.run=^TestConfigValidate$")
			command.Dir = root
			command.Env = append(os.Environ(),
				"XDG_CONFIG_HOME="+configDir, "XDG_STATE_HOME="+stateDir,
				"OPENCODE_SANDBOX_VALIDATE_TEST=1", "OPENCODE_SANDBOX_VALIDATE_ERROR="+tc.wantError)
			output, err := command.CombinedOutput()
			require.NoError(t, err, "%s", output)
			data, err := os.ReadFile(trust)
			require.NoError(t, err)
			require.Equal(t, "invalid trust", string(data))
			entries, err := os.ReadDir(root)
			require.NoError(t, err)
			if tc.workspace == "" {
				require.Empty(t, entries)
			} else {
				require.Len(t, entries, 1)
			}
		})
	}
}

func TestConfigShow(t *testing.T) {
	configDir := t.TempDir()
	stateDir := t.TempDir()
	parent := t.TempDir()
	workspace := filepath.Join(parent, "project")
	globalConfig := filepath.Join(configDir, "opencode-sandbox", "config.toml")
	parentConfig := filepath.Join(parent, ".opencode-sandbox.toml")
	workspaceConfig := filepath.Join(workspace, ".opencode-sandbox.toml")
	trust := filepath.Join(stateDir, "opencode-sandbox", "trust.json")
	for path, content := range map[string]string{
		globalConfig:    "[vm]\ncpus = 3\n[network]\nallowed_hosts = ['global.example.com']\n",
		parentConfig:    "[[files]]\nsource = '.'\ntarget = '/opt/shared'\n[network]\nallowed_hosts = ['parent.example.com']\n[network.vm_to_host]\nports = []\n",
		workspaceConfig: "[vm]\ncpus = 4\n[secrets.token]\nsource.file = './missing-credentials'\nguest.env = 'TOKEN'\nallowed_hosts = ['example.com']\nheaders = ['Authorization']\n",
		trust:           "invalid trust",
	} {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
		require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	}
	t.Chdir(workspace)
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("XDG_STATE_HOME", stateDir)

	var combined bytes.Buffer
	show := func() (string, string, error) {
		t.Helper()
		combined.Reset()
		var stdout, stderr bytes.Buffer
		cmd := NewCommand()
		cmd.SetArgs([]string{"config", "show"})
		cmd.SetOut(io.MultiWriter(&stdout, &combined))
		cmd.SetErr(io.MultiWriter(&stderr, &combined))
		err := cmd.ExecuteContext(t.Context())
		return stdout.String(), stderr.String(), err
	}

	stdout, stderr, err := show()
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("\nConfiguration sources (merge order):\n  built-in defaults\n  %q\n  %q\n  %q\n",
		globalConfig, parentConfig, workspaceConfig), stderr)
	require.Equal(t, stdout+stderr, combined.String())
	var settings launcherconfig.Settings
	_, err = toml.Decode(stdout, &settings)
	require.NoError(t, err)
	require.Equal(t, uint8(4), settings.VM.CPUs)
	require.Equal(t, launcherconfig.DefaultSettings().VM.MemoryMiB, settings.VM.MemoryMiB)
	require.Equal(t, []string{"global.example.com", "parent.example.com"}, settings.Network.AllowedHosts)
	require.Equal(t, []uint16{}, settings.Network.VMToHost.Ports)
	require.Equal(t, parent, settings.Files[0].Source)
	require.Equal(t, filepath.Join(workspace, "missing-credentials"), settings.Secrets["token"].Source.File)
	data, err := os.ReadFile(trust)
	require.NoError(t, err)
	require.Equal(t, "invalid trust", string(data))

	require.NoError(t, os.WriteFile(workspaceConfig, []byte("[aws]\nprofile = 'dev'\n"), 0600))
	stdout, stderr, err = show()
	require.ErrorContains(t, err, "requires aws.role")
	require.Empty(t, stdout)
	require.Empty(t, stderr)
}
