package sandbox_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenCodeCanReachOpenAIWithoutMise(t *testing.T) {
	s := startSandbox(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	require.NoError(t, s.ImportOpenAI(ctx))

	reply := promptOpenCode(t, s, "Reply exactly with auth ok")

	require.Equal(t, "auth ok", reply)
}

func TestOpenCodeCanReachOpenAIWithMise(t *testing.T) {
	worktree := newWorktree(t)
	require.NoError(t, os.WriteFile(filepath.Join(worktree, "mise.toml"), []byte("[tools]\nbun = \"1.3.14\"\n"), 0644))
	s := prepareSandbox(t, worktree)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	require.NoError(t, s.ImportOpenAI(ctx))
	execInSandbox(t, s, "bun", "--version")

	reply := promptOpenCode(t, s, "Reply exactly with auth ok")

	require.Equal(t, "auth ok", reply)
}
