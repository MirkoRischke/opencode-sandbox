// Command upgrade-microsandbox updates the SDK, runtime, and dotfiles pins together.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const sdk = "github.com/superradcompany/microsandbox/sdk/go"

var exactVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

func main() {
	dotfiles := flag.String("dotfiles", "../dotfiles", "path to the dotfiles repository")
	flag.Parse()
	if err := upgrade(*dotfiles, flag.Args()); err != nil {
		fmt.Fprintln(os.Stderr, "Upgrade failed:", err)
		fmt.Fprintln(os.Stderr, "Changes are left for inspection; Chezmoi was not applied.")
		os.Exit(1)
	}
}

func upgrade(dotfiles string, args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("expected at most one version argument")
	}
	version := "latest"
	if len(args) == 1 {
		version = args[0]
	}
	if version == "latest" {
		cmd := exec.Command("mise", "latest", "microsandbox")
		cmd.Stderr = os.Stderr
		output, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("resolve latest runtime: %w", err)
		}
		version = strings.TrimSpace(string(output))
	}
	version = strings.TrimPrefix(version, "v")
	if !exactVersion.MatchString(version) {
		return fmt.Errorf("expected an exact stable version, got %q", version)
	}

	config := filepath.Join(dotfiles, "home/private_dot_config/mise/config.toml.tmpl")
	// Check both pins before changing dependencies or installing anything.
	if _, err := updatedPin("mise.toml", version); err != nil {
		return err
	}
	if _, err := updatedPin(config, version); err != nil {
		return err
	}

	fmt.Println("Upgrading Microsandbox SDK and runtime to", version)
	// Ensure the matching SDK exists before editing go.mod.
	if err := run("go", "mod", "download", sdk+"@v"+version); err != nil {
		return err
	}
	if err := run("go", "get", sdk+"@v"+version); err != nil {
		return err
	}
	if err := run("go", "mod", "tidy"); err != nil {
		return err
	}
	if err := writePin("mise.toml", version); err != nil {
		return err
	}
	if err := run("mise", "exec", "microsandbox@"+version, "--", "mise", "run", "verify"); err != nil {
		return err
	}
	if err := run("mise", "run", "install"); err != nil {
		return err
	}
	if err := writePin(config, version); err != nil {
		return err
	}
	fmt.Println("Launcher installed; dotfiles pin updated:", config)
	fmt.Println("Review both repositories, then apply your Mise configuration through Chezmoi.")
	fmt.Println("Until then, the global runtime selection may still differ from the installed launcher.")
	return nil
}

func run(name string, args ...string) error {
	fmt.Println("+", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func updatedPin(path, version string) ([]byte, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return replacePin(content, version, path)
}

func replacePin(content []byte, version, path string) ([]byte, error) {
	pattern := regexp.MustCompile(`(?m)^microsandbox = "[^"\r\n]+"`)
	if len(pattern.FindAllIndex(content, -1)) != 1 {
		return nil, fmt.Errorf("expected exactly one microsandbox pin in %s", path)
	}
	return pattern.ReplaceAll(content, []byte(`microsandbox = "`+version+`"`)), nil
}

func writePin(path, version string) error {
	content, err := updatedPin(path, version)
	if err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}
