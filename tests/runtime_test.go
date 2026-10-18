package sandbox_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSandboxWorksWithMiseShimOnPath(t *testing.T) {
	t.Parallel()
	if os.Getenv("SANDBOX_TEST_MISE_SHIM") == "1" {
		s := startSandbox(t)
		execInSandbox(t, s, "git", "--version")
		return
	}
	mise, err := exec.LookPath("mise")
	require.NoError(t, err)
	shimDir := t.TempDir()
	require.NoError(t, os.Symlink(mise, filepath.Join(shimDir, "msb")))
	binary, err := os.Executable()
	require.NoError(t, err)
	command := exec.Command(binary, "-test.run=^TestSandboxWorksWithMiseShimOnPath$", "-test.v", "-test.timeout=2m")
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "MSB_PATH=") && !strings.HasPrefix(env, "MSB_LIBKRUNFW_PATH=") && !strings.HasPrefix(env, "PATH=") {
			command.Env = append(command.Env, env)
		}
	}
	command.Env = append(command.Env,
		"PATH="+shimDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"SANDBOX_TEST_MISE_SHIM=1")

	output, err := command.CombinedOutput()

	require.NoError(t, err, "launcher must resolve the real runtime behind a mise shim: %s", output)
}
