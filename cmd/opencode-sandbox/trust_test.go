package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"opencode-sandbox/internal/sandbox"
)

func TestConfirmRepoConfig(t *testing.T) {
	for _, answer := range []string{"y\n", "yes\n", "YES\n", "n\n", "\n", "", "yes"} {
		t.Run(answer, func(t *testing.T) {
			var output bytes.Buffer
			settings := sandbox.DefaultSettings()
			settings.Secrets = map[string]sandbox.SecretSettings{"github": {
				File: "creds/token", Env: "GH_TOKEN", AllowedHosts: []string{"api.github.com"}, Headers: []string{"authorization"},
			}}
			err := confirmRepoConfig(strings.NewReader(answer), &output, "/repo/.opencode-sandbox.toml", []byte("# override\n\x1b[2J\n"), settings)
			approved := answer == "y\n" || answer == "yes\n" || answer == "YES\n"
			if (err == nil) != approved {
				t.Fatalf("answer %q: unexpected result %v", answer, err)
			}
			for _, want := range []string{"# override", "api.github.com", "creds/token", "host credential access", "\\x1b"} {
				if !strings.Contains(output.String(), want) {
					t.Errorf("prompt missing %q", want)
				}
			}
			if strings.ContainsRune(output.String(), '\x1b') {
				t.Fatal("unescaped terminal control character")
			}
		})
	}
}

func TestApproveRepoConfigRejectsPipedInput(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	original := os.Stdin
	os.Stdin = reader
	defer func() { os.Stdin = original }()
	if _, err := writer.WriteString("yes\n"); err != nil {
		t.Fatal(err)
	}
	err = approveRepoConfig("/repo/.opencode-sandbox.toml", nil, sandbox.DefaultSettings())
	if err == nil || !strings.Contains(err.Error(), "launch interactively") {
		t.Fatalf("expected non-interactive refusal, got %v", err)
	}
}
