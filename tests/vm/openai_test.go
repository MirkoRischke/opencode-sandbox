//go:build live

package sandbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenCodeCanReachOpenAI(t *testing.T) {
	t.Parallel()
	s := startSandbox(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	require.NoError(t, s.ImportOpenAI(ctx))

	reply := promptOpenCode(t, s, "Reply exactly with auth ok")

	require.Equal(t, "auth ok", reply)
}
