package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
)

// ImportOpenAI seeds only the active subscription credential, once per VM.
// Secrets remain in memory and travel through stdin; diagnostics are suppressed.
func (s *Sandbox) ImportOpenAI(ctx context.Context) error {
	const marker = "/root/.opencode-sandbox-auth-ready"
	if s.Exec(ctx, nil, nil, nil, "test", "-f", marker) == nil {
		return nil
	}
	data, err := exec.CommandContext(ctx, "opencode", "auth", "export", "openai").Output()
	if err != nil {
		return errors.New("export host OpenAI credentials failed (output suppressed)")
	}
	var records []json.RawMessage
	if json.Unmarshal(data, &records) != nil {
		return errors.New("invalid host credential export")
	}
	var selected []json.RawMessage
	for _, record := range records {
		var credential struct {
			IntegrationID string `json:"integrationID"`
			Active        bool   `json:"active"`
			Value         struct {
				Type string `json:"type"`
			} `json:"value"`
		}
		if json.Unmarshal(record, &credential) != nil {
			return errors.New("invalid credential entry")
		}
		if credential.IntegrationID != "openai" || !credential.Active {
			continue
		}
		if credential.Value.Type != "oauth" {
			return errors.New("active OpenAI credential must use subscription OAuth")
		}
		selected = append(selected, record)
	}
	if len(selected) != 1 {
		return errors.New("expected exactly one active host OpenAI credential")
	}
	payload, err := json.Marshal(selected)
	if err != nil {
		return errors.New("encode selected credential failed")
	}
	if s.Exec(ctx, bytes.NewReader(payload), nil, nil, "opencode", "auth", "import") != nil {
		return errors.New("import OpenAI credential failed (output suppressed)")
	}
	return s.Exec(ctx, nil, nil, nil, "touch", marker)
}
