package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// The SDK does not search PATH. Select the existing CLI bundle once, before
// constructing SDK handles; its adjacent libkrunfw is resolved by the SDK.
// Explicit SDK runtime overrides always win. Never install a second runtime.
var configureRuntime = sync.OnceValue(func() error {
	if os.Getenv("MSB_PATH") != "" || os.Getenv("MSB_LIBKRUNFW_PATH") != "" {
		return nil
	}
	path, err := runtimeCLI()
	if err != nil {
		return err
	}
	return os.Setenv("MSB_PATH", path)
})

// runtimeCLI also resolves the executable when only the SDK library was overridden.
func runtimeCLI() (string, error) {
	if path := os.Getenv("MSB_PATH"); path != "" {
		return path, nil
	}
	path, err := exec.LookPath("msb")
	if err != nil {
		return "", err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	// mise shims symlink to mise itself, not to the managed executable.
	if filepath.Base(path) == "mise" {
		output, err := exec.Command(path, "which", "msb").Output()
		if err != nil {
			return "", fmt.Errorf("resolve Microsandbox mise shim: %w", err)
		}
		path, err = filepath.EvalSymlinks(strings.TrimSpace(string(output)))
		if err != nil {
			return "", err
		}
	}
	return path, nil
}
