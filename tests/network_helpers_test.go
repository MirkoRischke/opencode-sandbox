package sandbox_test

import (
	"context"
	"testing"
	"time"

	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func startSandboxWithSettings(t *testing.T, settings sandbox.Settings) *sandbox.Sandbox {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	s, err := sandbox.Prepare(ctx, newWorktree(t), nil, nil, settings)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		assert.NoError(t, s.Remove(ctx))
	})
	return s
}

func requireNetworkBlocked(t *testing.T, s *sandbox.Sandbox, url string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err := s.Exec(ctx, nil, nil, nil, "curl", "--noproxy", "*", "--fail", "--silent", "--show-error", "--max-time", "5", url)
	require.Error(t, err, "%s must be blocked", url)
	require.NoError(t, ctx.Err(), "guest curl must fail, not the SDK deadline")
}
