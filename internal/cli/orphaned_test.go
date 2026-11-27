package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRemoveDryRunRequiresOrphaned(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"remove", "--dry-run"},
		{"rm", "--all", "--dry-run"},
		{"remove", "--dry-run=false"},
	} {
		_, err := executeCommand(args...)
		require.ErrorContains(t, err, "--dry-run requires --orphaned")
	}
}
