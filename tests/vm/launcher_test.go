package sandbox_test

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLauncherListsVMsOutsideGitWithoutLoadingSettings(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	dir := t.TempDir()
	writeSettingsAt(t, dir, "invalid TOML [")
	binary := buildLauncher(t)
	for _, name := range []string{"list", "ls"} {
		t.Run(name, func(t *testing.T) {
			command := exec.CommandContext(ctx, binary, "--console-log-level", "warn", name)
			command.Env = append(os.Environ(), "XDG_CONFIG_HOME="+dir)
			command.Dir = t.TempDir()

			output, err := command.CombinedOutput()

			require.NoError(t, err, "%s", output)
			if string(output) != "No managed VMs found.\n" {
				require.Contains(t, string(output), "NAME")
				require.Contains(t, string(output), "STATE")
			}
		})
	}
}
