package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWorkspaceTrustRetainsTrailingDecodeFailure(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "trust.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"version":1,"workspaces":{}} invalid`), 0600))

	_, err := loadTrustStore(path)

	require.ErrorContains(t, err, "invalid trailing data in workspace trust")
	var cause *json.SyntaxError
	require.ErrorAs(t, err, &cause)
}

func TestWorkspaceTrustRejectsValidTrailingJSON(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "trust.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"version":1,"workspaces":{}} {}`), 0600))

	_, err := loadTrustStore(path)

	require.EqualError(t, err, "invalid trailing data in workspace trust")
}
