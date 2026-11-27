package sandbox

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"text/template"

	"opencode-sandbox/internal/config"
)

const (
	sandboxConfigDir    = "/etc/opencode-sandbox"
	sandboxInstructions = sandboxConfigDir + "/agent-sandbox-instructions.md"
)

//go:embed agent-sandbox-instructions.md.tmpl
var sandboxInstructionsSource string

var sandboxInstructionsTemplate = template.Must(template.New("sandbox-instructions").Parse(sandboxInstructionsSource))

type environmentInstructions struct {
	Unrestricted   bool
	AllowedHosts   []string
	ReadOnlyMounts []string
}

func renderEnvironmentInstructions(environment environmentInstructions) ([]byte, error) {
	var contents bytes.Buffer
	if err := sandboxInstructionsTemplate.Execute(&contents, environment); err != nil {
		return nil, fmt.Errorf("render sandbox instructions: %w", err)
	}
	return contents.Bytes(), nil
}

func (s *Sandbox) configureGitHubInstructions(ctx context.Context, auth *githubAuth) error {
	var environment struct {
		Mode string
		Repo string
	}
	if auth != nil {
		environment.Mode = "read-only"
		if auth.repo != "" {
			environment.Mode = "read-write"
			environment.Repo = auth.owner + "/" + auth.repo
		}
	}
	var contents bytes.Buffer
	if err := sandboxInstructionsTemplate.ExecuteTemplate(&contents, "github", environment); err != nil {
		return fmt.Errorf("render sandbox GitHub instructions: %w", err)
	}
	// Keep the existing instructions about applied network and mount settings.
	// The launcher owns the GitHub section at the end of this file.
	err := s.Exec(ctx, &contents, nil, nil, "bash", "-eu", "-c", `
file=$1
tmp=$(mktemp "$file.XXXXXX")
trap 'rm -f -- "$tmp"' EXIT
if [[ -f "$file" ]]; then
  sed '/^<!-- opencode-sandbox:github -->$/,$d' "$file" > "$tmp"
fi
cat >> "$tmp"
chmod 644 "$tmp"
mv -f -- "$tmp" "$file"
`, "sandbox-github-instructions", sandboxInstructions)
	if err != nil {
		return fmt.Errorf("configure sandbox GitHub instructions: %w", err)
	}
	return nil
}

// configureEnvironment runs only for newly created VMs, including snapshot restores.
// Existing VMs retain the instructions describing their original applied settings.
func (s *Sandbox) configureEnvironment(ctx context.Context, settings config.Settings) error {
	environment := environmentInstructions{
		Unrestricted: settings.Network.Unrestricted,
		AllowedHosts: settings.Network.EffectiveAllowedHosts(),
	}
	for _, mount := range settings.Files {
		if mount.IsCopy() || mount.IsWritableMount() {
			continue
		}
		environment.ReadOnlyMounts = append(environment.ReadOnlyMounts, mount.Target)
	}
	contents, err := renderEnvironmentInstructions(environment)
	if err != nil {
		return err
	}
	err = s.Exec(ctx, bytes.NewReader(contents), nil, nil, "bash", "-eu", "-c", `
mkdir -p "$1"
cat > "$2"
`, "sandbox-environment", sandboxConfigDir, sandboxInstructions)
	if err != nil {
		return fmt.Errorf("configure sandbox instructions: %w", err)
	}
	return nil
}
