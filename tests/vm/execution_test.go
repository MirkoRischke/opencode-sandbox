package sandbox_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSandboxCommandReceivesStdin(t *testing.T) {
	t.Parallel()
	s := startSandbox(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	input := "hello from host\n"
	var output bytes.Buffer

	err := s.Exec(ctx, strings.NewReader(input), &output, nil, "cat")

	require.NoError(t, err)
	require.Equal(t, input, output.String())
}

func TestSandboxCommandFailureIsReported(t *testing.T) {
	t.Parallel()
	s := startSandbox(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := s.Exec(ctx, nil, nil, nil, "bash", "-c", "exit 7")

	require.ErrorContains(t, err, "exited with code 7")
}
