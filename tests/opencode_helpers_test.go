package sandbox_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/require"
)

func promptOpenCode(t *testing.T, s *sandbox.Sandbox, prompt string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	model := os.Getenv("OPENCODE_TEST_MODEL")
	if model == "" {
		model = "openai/gpt-6-luna"
	}
	args := []string{"opencode", "run", "--format", "json", "--model", model, prompt}
	var output bytes.Buffer
	err := s.Exec(ctx, nil, &output, nil, args...)
	require.NoError(t, err, "OpenAI request failed (output suppressed to protect credentials)")
	return readReply(t, output.Bytes())
}

func readReply(t *testing.T, output []byte) string {
	t.Helper()
	var reply strings.Builder
	decoder := json.NewDecoder(bytes.NewReader(output))
	for {
		var event struct {
			Type string `json:"type"`
			Part struct {
				Text string `json:"text"`
			} `json:"part"`
		}
		err := decoder.Decode(&event)
		if err == io.EOF {
			break
		}
		require.NoError(t, err, "decode OpenCode event")
		require.NotEqual(t, "error", event.Type, "OpenCode reported a failed request")
		if event.Type == "text" {
			reply.WriteString(event.Part.Text)
		}
	}
	return strings.TrimSpace(reply.String())
}
