package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
)

// ImportOpenAI seeds only the active subscription credential, once per VM.
// Secrets remain in memory and travel through stdin; payloads are not printed.
func (s *Sandbox) ImportOpenAI(ctx context.Context) error {
	const marker = "/root/.opencode-sandbox-auth-ready"
	err := s.Exec(ctx, nil, nil, nil, "test", "-f", marker)
	if err == nil {
		return nil
	}
	var exit *guestExitError
	if !errors.As(err, &exit) || exit.code != 1 {
		return fmt.Errorf("check guest OpenAI credential marker: %w", err)
	}
	data, err := exec.CommandContext(ctx, "opencode", "auth", "export", "openai").Output()
	if err != nil {
		return fmt.Errorf("export host OpenAI credentials: %w", err)
	}
	payload, err := selectOpenAICredential(data)
	if err != nil {
		return err
	}
	if err := s.Exec(ctx, bytes.NewReader(payload), nil, nil, "opencode", "auth", "import"); err != nil {
		return fmt.Errorf("import OpenAI credential: %w", err)
	}
	return s.Exec(ctx, nil, nil, nil, "touch", marker)
}

// Select one active subscription credential, preserving provider-specific fields.
func selectOpenAICredential(data []byte) ([]byte, error) {
	var records []json.RawMessage
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("invalid host credential export: %w", err)
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
		if err := json.Unmarshal(record, &credential); err != nil {
			return nil, fmt.Errorf("invalid credential entry: %w", err)
		}
		if credential.IntegrationID != "openai" || !credential.Active {
			continue
		}
		if credential.Value.Type != "oauth" {
			return nil, errors.New("active OpenAI credential must use subscription OAuth")
		}
		selected = append(selected, record)
	}
	if len(selected) != 1 {
		return nil, errors.New("expected exactly one active host OpenAI credential")
	}
	payload, err := json.Marshal(selected)
	if err != nil {
		return nil, fmt.Errorf("encode selected credential: %w", err)
	}
	return payload, nil
}
