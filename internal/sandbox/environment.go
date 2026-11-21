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

// configureEnvironment runs only for newly created VMs, including snapshot restores.
// Existing VMs retain the instructions describing their original applied settings.
func (s *Sandbox) configureEnvironment(ctx context.Context, settings config.Settings) error {
	environment := environmentInstructions{
		Unrestricted: settings.Network.Unrestricted,
		AllowedHosts: settings.Network.EffectiveAllowedHosts(),
	}
	for _, mount := range settings.Files {
		if mount.IsCopy() {
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
