package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type testPruneInventory struct {
	targets []string
	calls   int
	err     error
}

func (p *testPruneInventory) Targets() []string { return p.targets }
func (p *testPruneInventory) Execute(context.Context) error {
	p.calls++
	return p.err
}

func TestPruneConfirmationAndPreview(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, answer string
		args         []string
		deletes      int
		wantError    bool
	}{
		{name: "confirm", answer: "yes\n", deletes: 1},
		{name: "decline", answer: "n\n", wantError: true},
		{name: "EOF", wantError: true},
		{name: "skip", args: []string{"--yes"}, deletes: 1},
		{name: "dry run", args: []string{"--dry-run"}},
		{name: "dry run wins", args: []string{"--dry-run", "--yes"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			plan := &testPruneInventory{targets: []string{"VM test", "OpenCode history"}}
			cmd := pruneCommand(func(context.Context, bool, bool) (pruneInventory, error) { return plan, nil })
			var output bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetErr(&output)
			cmd.SetIn(strings.NewReader(test.answer))
			cmd.SetArgs(test.args)
			err := cmd.Execute()
			if test.wantError {
				require.ErrorContains(t, err, "not confirmed")
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, test.deletes, plan.calls)
			require.Contains(t, output.String(), "VM test")
			require.Contains(t, output.String(), "permanently deleted")
		})
	}
}

func TestPruneScopesAndErrors(t *testing.T) {
	t.Parallel()
	for _, scope := range []string{"", "--repo", "--all"} {
		t.Run(scope, func(t *testing.T) {
			t.Parallel()
			failure := errors.New("deletion failed")
			plan := &testPruneInventory{targets: []string{"VM test"}, err: failure}
			cmd := pruneCommand(func(_ context.Context, repo, all bool) (pruneInventory, error) {
				require.Equal(t, scope == "--repo", repo)
				require.Equal(t, scope == "--all", all)
				return plan, nil
			})
			var output bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetErr(&output)
			args := []string{"--yes"}
			if scope != "" {
				args = append(args, scope)
			}
			cmd.SetArgs(args)
			require.ErrorIs(t, cmd.Execute(), failure)
			require.Equal(t, 1, plan.calls)
			require.NotContains(t, output.String(), "Prune complete")
			if scope == "--all" {
				require.Contains(t, output.String(), "including unrelated ones")
			}
		})
	}
	for _, args := range [][]string{{"--repo", "--all"}, {"extra"}} {
		cmd := pruneCommand(func(context.Context, bool, bool) (pruneInventory, error) {
			t.Fatal("invalid flags must be rejected before inventory")
			return nil, nil
		})
		cmd.SetArgs(args)
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		require.Error(t, cmd.Execute())
	}
}

func TestPruneEmptyScopeNeedsNoConfirmation(t *testing.T) {
	t.Parallel()
	plan := &testPruneInventory{}
	cmd := pruneCommand(func(context.Context, bool, bool) (pruneInventory, error) { return plan, nil })
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs(nil)
	require.NoError(t, cmd.Execute())
	require.Zero(t, plan.calls)
	require.Contains(t, output.String(), "No resources")
}
