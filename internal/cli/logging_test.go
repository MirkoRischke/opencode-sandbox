package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"opencode-sandbox/internal/logging"

	"github.com/stretchr/testify/require"
)

func TestLoggingSetupFailureIsNonfatal(t *testing.T) {
	t.Parallel()
	state := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(state, nil, 0600))
	var warnings bytes.Buffer
	ctx, finish := startLogging(context.Background(), state, "/project", "launch", &warnings)
	logging.FromContext(ctx).Info("No file available")
	finish(nil)
	require.Contains(t, warnings.String(), "warning: launcher logging unavailable")
}
