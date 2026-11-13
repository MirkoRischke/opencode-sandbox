package gateway_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	gateway "opencode-sandbox/llm-gateway"

	"github.com/stretchr/testify/require"
)

//nolint:forbidigo // Serial PATH override observes Compose options without touching Docker.
func TestEnsureUsesCustomComposeOptions(t *testing.T) {
	bin := t.TempDir()
	log := filepath.Join(bin, "calls")
	require.NoError(t, os.WriteFile(filepath.Join(bin, "docker"), []byte(`#!/bin/sh
printf '%s\n%s\n' "$*" "$OPENCODE_SANDBOX_GATEWAY_PORT" > "$CALL_LOG"
`), 0700))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CALL_LOG", log)
	for _, tc := range []struct {
		name    string
		options gateway.Options
		want    string
	}{
		{"default project name", gateway.Options{}, "--project-name opencode-sandbox-gateway"},
		{"default host port", gateway.Options{}, "\n4000\n"},
		{"project name", gateway.Options{ProjectName: "isolated-gateway"}, "--project-name isolated-gateway"},
		{"host port", gateway.Options{Port: 14000}, "\n14000\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := gateway.NewWithOptions(t.TempDir(), tc.options)

			err := g.Ensure(context.Background(), io.Discard)

			require.NoError(t, err)
			calls, err := os.ReadFile(log)
			require.NoError(t, err)
			require.Contains(t, string(calls), tc.want)
		})
	}
}
