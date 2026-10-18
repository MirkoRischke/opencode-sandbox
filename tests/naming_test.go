package sandbox_test

import (
	"context"
	"testing"
	"time"

	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/require"
)

func TestSandboxNameRemainsStableWhenBranchChanges(t *testing.T) {
	t.Parallel()
	s := startSandbox(t)
	runGit(t, "-C", s.Worktree, "switch", "-c", "another-branch")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	resumed, err := sandbox.Prepare(ctx, s.Worktree, nil, nil, testSettings(t))

	require.NoError(t, err)
	defer func() { require.NoError(t, resumed.Close()) }()
	require.Equal(t, s.Name, resumed.Name)
}
