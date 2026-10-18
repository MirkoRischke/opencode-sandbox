package sandbox

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	m "github.com/superradcompany/microsandbox/sdk/go"
)

func TestWriteNetworkDiagnostics(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	repo := t.TempDir()
	out, err := exec.Command("git", "-C", repo, "init").CombinedOutput()
	require.NoError(t, err, string(out))
	root, err := filepath.EvalSymlinks(repo)
	require.NoError(t, err)
	subdir := filepath.Join(repo, "nested")
	require.NoError(t, os.Mkdir(subdir, 0700))
	alias := filepath.Join(t.TempDir(), "alias")
	require.NoError(t, os.Symlink(repo, alias))
	path, err := diagnosticsPath(root)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	data := []byte("{\n  \"denied_targets\": [], \"worktree\": \"example\"\n}\n")
	require.NoError(t, os.WriteFile(path, data, 0600))
	// Neither global nor unapproved repo configuration should be loaded.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	configDir := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "opencode-sandbox")
	require.NoError(t, os.MkdirAll(configDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.toml"), []byte("invalid = ["), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".opencode-sandbox.toml"), []byte("invalid = ["), 0600))

	for _, directory := range []string{repo, subdir, alias} {
		t.Run(directory, func(t *testing.T) {
			var output bytes.Buffer
			require.NoError(t, WriteNetworkDiagnostics(context.Background(), directory, &output))
			require.Equal(t, data, output.Bytes())
		})
	}

	t.Run("writer error", func(t *testing.T) {
		err := WriteNetworkDiagnostics(context.Background(), repo, diagnosticsErrorWriter{})
		require.ErrorIs(t, err, os.ErrPermission)
	})
	t.Run("missing history", func(t *testing.T) {
		require.NoError(t, os.Remove(path))
		var output bytes.Buffer
		err := WriteNetworkDiagnostics(context.Background(), subdir, &output)
		require.ErrorContains(t, err, "no saved network diagnostics")
		require.Empty(t, output.String())
		_, err = os.Stat(path)
		require.True(t, os.IsNotExist(err))
	})
	t.Run("outside Git", func(t *testing.T) {
		var output bytes.Buffer
		err := WriteNetworkDiagnostics(context.Background(), t.TempDir(), &output)
		require.ErrorContains(t, err, "discover worktree")
		require.Empty(t, output.String())
	})
}

type diagnosticsErrorWriter struct{}

func (diagnosticsErrorWriter) Write([]byte) (int, error) {
	return 0, os.ErrPermission
}

func denialEntry(text string) m.LogEntry {
	return m.LogEntry{
		Source: m.LogSourceSystem, Timestamp: time.Date(2026, 10, 18, 14, 0, 0, 0, time.UTC),
		Data: []byte("DEBUG microsandbox_network::engine::tcp::proxy: " + text + "\n"),
	}
}

func TestParseNetworkDenial(t *testing.T) {
	for _, test := range []struct {
		name, text, hostname, ip, protocol, reason string
		port                                       uint16
	}{
		{"TLS", "TLS egress denied by domain policy sni=Registry.NPMJS.org dst=104.16.24.34:443", "registry.npmjs.org", "", "tcp", "domain_policy", 443},
		{"optional SNI", `TCP egress denied by strict hostname policy sni=Some("example.org") dst=1.1.1.1:443`, "example.org", "", "tcp", "strict_hostname_policy", 443},
		{"no SNI", "TCP egress denied by domain policy dst=1.1.1.1:8443 source=cache", "", "1.1.1.1", "tcp", "domain_policy", 8443},
		{"IPv6", "TLS egress denied by default policy sni=example.org dst=[2606:4700::1111]:443", "example.org", "", "tcp", "default_policy", 443},
		{"IPv6 without hostname", "TCP egress denied by domain policy dst=[2606:4700::1111]:443", "", "2606:4700::1111", "tcp", "domain_policy", 443},
		{"DNS", `DNS query denied by network policy domain="Example.org."`, "example.org", "", "dns", "domain_policy", 0},
		{"ICMP", "ICMP echo denied by policy dst=1.1.1.1", "", "1.1.1.1", "icmp", "network_policy", 0},
		{"ICMPv6", "ICMPv6 echo denied by policy dst=2606:4700::1111", "", "2606:4700::1111", "icmpv6", "network_policy", 0},
		{"CONNECT", "CONNECT tunnel denied by strict hostname policy sni=example.org dst=1.1.1.1:443", "example.org", "", "tcp", "strict_hostname_policy", 443},
		{"TLS bypass", "TLS bypass denied by strict hostname policy sni=example.org dst=1.1.1.1:443", "example.org", "", "tcp", "strict_hostname_policy", 443},
	} {
		t.Run(test.name, func(t *testing.T) {
			entry := denialEntry(test.text)
			target, ok := parseNetworkDenial(entry)
			require.True(t, ok)
			require.Equal(t, test.hostname, target.Hostname)
			require.Equal(t, test.ip, target.IP)
			require.Equal(t, test.port, target.Port)
			require.Equal(t, test.protocol, target.Protocol)
			require.Equal(t, test.reason, target.Reason)
			require.EqualValues(t, 1, target.Observations)
			require.Equal(t, entry.Timestamp, target.FirstSeen)
		})
	}
}

func TestNetworkDiagnosticsIgnoreUnrelatedAndGuestLogs(t *testing.T) {
	for _, text := range []string{
		"connected sni=example.org dst=1.1.1.1:443",
		"policy deny response not delivered dst=1.1.1.1:443",
		"ingress denied by policy; sending RST dst=1.1.1.1:443",
		"DNS resolver denied by network policy resolver=1.1.1.1:53",
		"TCP egress denied by domain policy dst=not-an-ip:443",
		"TLS egress denied by domain policy sni=None",
		"TLS egress denied by domain policy sni=https://example.org/private",
	} {
		_, ok := parseNetworkDenial(denialEntry(text))
		require.False(t, ok, text)
	}
	entry := denialEntry("TLS egress denied by domain policy sni=example.org dst=1.1.1.1:443")
	entry.Source = m.LogSourceStdout
	_, ok := parseNetworkDenial(entry)
	require.False(t, ok)
	entry.Source = m.LogSourceSystem
	entry.Data = []byte(strings.ReplaceAll(entry.Text(), "microsandbox_network::", "another_module::"))
	_, ok = parseNetworkDenial(entry)
	require.False(t, ok)
}

func TestNetworkDiagnosticsHistorySurvivesRunsAndVMRecreation(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	worktree := "/projects/repo/worktree"
	entry := denialEntry("TLS egress denied by domain policy sni=example.org dst=1.1.1.1:443")
	d, err := loadDiagnostics(worktree, entry.Timestamp)
	require.NoError(t, err)
	d.observe(entry)
	d.observe(entry)
	require.NoError(t, d.save())
	path := d.path
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())

	// No VM identity is used in the state path; subsequent launches merge history.
	d, err = loadDiagnostics(worktree, entry.Timestamp.Add(time.Hour))
	require.NoError(t, err)
	require.Equal(t, path, d.path)
	entry.Timestamp = entry.Timestamp.Add(time.Hour)
	entry.Data = []byte(strings.ReplaceAll(entry.Text(), "1.1.1.1", "8.8.8.8"))
	d.observe(entry)
	require.NoError(t, d.save())
	require.Len(t, d.overview.DeniedTargets, 1, "changing IP must not split a known hostname")
	target := d.overview.DeniedTargets[0]
	require.EqualValues(t, 3, target.Observations)
	require.Equal(t, entry.Timestamp.Add(-time.Hour), target.FirstSeen)
	require.Equal(t, entry.Timestamp, target.LastSeen)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotContains(t, string(data), "DEBUG")
	require.NotContains(t, string(data), "1.1.1.1")
	files, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	require.Len(t, files, 1, "atomic-save temporary files must be removed")

	other, err := loadDiagnostics("/projects/repo/other-worktree", entry.Timestamp)
	require.NoError(t, err)
	require.NotEqual(t, path, other.path)
	require.Empty(t, other.overview.DeniedTargets)
}

func TestNetworkDiagnosticsKeepDistinctPortsAndIPOnlyTargets(t *testing.T) {
	d := &networkDiagnostics{}
	for _, text := range []string{
		"TCP egress denied by domain policy dst=1.1.1.1:443",
		"TCP egress denied by domain policy dst=1.1.1.1:8443",
		"TCP egress denied by domain policy dst=8.8.8.8:443",
	} {
		d.observe(denialEntry(text))
	}
	require.Len(t, d.overview.DeniedTargets, 3)
}

func TestNetworkDiagnosticsPreserveInvalidHistory(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	path, err := diagnosticsPath("/project")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	for _, data := range []string{`broken`, `{"schema_version":2,"worktree":"/project"}`, `{"schema_version":1,"worktree":"/other"}`} {
		require.NoError(t, os.WriteFile(path, []byte(data), 0600))
		_, err := loadDiagnostics("/project", time.Now())
		require.Error(t, err)
		stored, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, data, string(stored))
	}
}

func TestNetworkDiagnosticsStateHomeFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	path, err := diagnosticsPath("/project")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(path, filepath.Join(home, ".local", "state", "opencode-sandbox", "diagnostics")+string(filepath.Separator)))
}

func TestNetworkDiagnosticsBoundsDistinctTargets(t *testing.T) {
	d := &networkDiagnostics{}
	for port := 1; port <= 4097; port++ {
		entry := denialEntry("TCP egress denied by domain policy dst=1.1.1.1:" + strconv.Itoa(port))
		d.observe(entry)
	}
	require.Len(t, d.overview.DeniedTargets, 4096)
	require.True(t, d.overview.Overflow)
	d.observe(denialEntry("TCP egress denied by domain policy dst=1.1.1.1:1"))
	require.EqualValues(t, 2, d.overview.DeniedTargets[0].Observations)
}

func TestNetworkDiagnosticsDefersWarningsUntilShutdown(t *testing.T) {
	for _, failure := range []string{"collection", "save"} {
		t.Run(failure, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			d, err := loadDiagnostics("/project", time.Now())
			require.NoError(t, err)
			if failure == "save" {
				// An existing regular file cannot be used as the overview directory.
				d.path = filepath.Join(t.TempDir(), "not-a-directory", "network.json")
				require.NoError(t, os.WriteFile(filepath.Dir(d.path), []byte("occupied"), 0600))
			}
			stderr := os.Stderr
			captured, err := os.CreateTemp(t.TempDir(), "stderr")
			require.NoError(t, err)
			os.Stderr = captured
			t.Cleanup(func() {
				os.Stderr = stderr
				require.NoError(t, captured.Close())
			})
			// Drive a background polling failure, then end collection without
			// returning terminal ownership to the caller via finish yet.
			calls := 0
			d.run(func() error {
				calls++
				if calls == 1 {
					d.once.Do(func() { close(d.stop) })
				}
				if failure == "collection" {
					return errors.New("simulated log-stream failure")
				}
				return nil
			})
			messages, err := os.ReadFile(captured.Name())
			require.NoError(t, err)
			require.Empty(t, string(messages), "background failures must not write into the attached TUI")
			d.finish()
			messages, err = os.ReadFile(captured.Name())
			require.NoError(t, err)
			require.Contains(t, string(messages), "warning:")
			require.Contains(t, string(messages), failure)
			d.finish()
			again, err := os.ReadFile(captured.Name())
			require.NoError(t, err)
			require.Equal(t, string(messages), string(again), "Stop followed by Close must report each warning only once")
		})
	}
}

func TestDiagnosticErrorKindDoesNotExposeRuntimeDetails(t *testing.T) {
	require.Equal(t, "timeout", diagnosticErrorKind(context.DeadlineExceeded))
	require.Equal(t, "permission denied", diagnosticErrorKind(os.ErrPermission))
	require.Equal(t, "unknown error", diagnosticErrorKind(errors.New("guest-controlled sensitive details")))
}
