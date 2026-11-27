package sandbox_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/require"
)

func TestLauncherWaitsForGatewayBeforePreparingVMAndRunningStartup(t *testing.T) {
	t.Parallel()
	worktree := newLauncherProbeWorktree(t)
	settings := testSettings(t)
	settings.Snapshot.Command = `
test -z "${LLM_GATEWAY_KEY:-}"
touch vm-prepared
`
	var encoded bytes.Buffer
	require.NoError(t, toml.NewEncoder(&encoded).Encode(settings))
	dir := t.TempDir()
	writeSettingsAt(t, dir, encoded.String())
	bin := t.TempDir()
	docker, err := exec.LookPath("docker")
	require.NoError(t, err)
	// VM preparation and OpenCode must wait for gateway readiness/provisioning.
	require.NoError(t, os.WriteFile(filepath.Join(bin, "docker"), []byte(fmt.Sprintf(`#!/bin/sh
if [ "$1" != compose ]; then exec %q "$@"; fi
case "$*" in
  *" up "*)
    test ! -f "$TEST_WORKTREE/vm-prepared" || exit 19
    touch "$TEST_WORKTREE/gateway-ready" ;;
esac
`, docker)), 0700))
	err = runLauncherProbe(t, worktree, map[string]string{
		"XDG_CONFIG_HOME": dir, "XDG_STATE_HOME": t.TempDir(),
		"PATH":          bin + string(os.PathListSeparator) + os.Getenv("PATH"),
		"TEST_WORKTREE": worktree,
	}, `test -f gateway-ready; test "$LLM_GATEWAY_KEY" = '$MSB_LLM_GATEWAY_KEY'; touch prompt-started`)
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(worktree, "prompt-started"))
}

func TestLauncherDoesNotRunStartupWhenGatewayFails(t *testing.T) {
	t.Parallel()
	worktree := newLauncherProbeWorktree(t)
	dir := t.TempDir()
	writeSettingsAt(t, dir, "")
	bin := t.TempDir()
	docker, err := exec.LookPath("docker")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(bin, "docker"), []byte(fmt.Sprintf(`#!/bin/sh
if [ "$1" != compose ]; then exec %q "$@"; fi
case "$*" in *" up "*) exit 17;; esac
`, docker)), 0700))
	err = runLauncherProbe(t, worktree, map[string]string{
		"XDG_CONFIG_HOME": dir, "XDG_STATE_HOME": t.TempDir(),
		"PATH": bin + string(os.PathListSeparator) + os.Getenv("PATH"),
	}, `touch prompt-started`)
	require.ErrorContains(t, err, "start LLM gateway")
	require.NoFileExists(t, filepath.Join(worktree, "prompt-started"))
}
