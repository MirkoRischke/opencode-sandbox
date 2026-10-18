package cli

import (
	"bytes"
	"errors"
	"testing"

	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/require"
)

func TestReportRemovals(t *testing.T) {
	t.Parallel()
	size := uint64(1024)
	failure := errors.New("deletion failed")
	for _, tc := range []struct {
		name, kind string
		sizes      []*uint64
		err        error
		want       string
	}{
		{name: "empty", kind: "VM", want: "Nothing to delete.\n"},
		{name: "empty snapshots", kind: "snapshot", want: "Nothing to delete.\n"},
		{name: "one VM", kind: "VM", sizes: []*uint64{nil}, want: "Deleted VM item\nDeleted 1 VM\n"},
		{name: "multiple VMs omit sizes", kind: "VM", sizes: []*uint64{&size, &size},
			want: "Deleted VM item\nDeleted VM item\nDeleted 2 VMs\n"},
		{name: "known snapshots", kind: "snapshot", sizes: []*uint64{&size, &size},
			want: "Deleted snapshot item (1.0 KiB)\nDeleted snapshot item (1.0 KiB)\nDeleted 2 snapshots · total reported size: 2.0 KiB\n"},
		{name: "mixed sizes", kind: "snapshot", sizes: []*uint64{&size, nil},
			want: "Deleted snapshot item (1.0 KiB)\nDeleted snapshot item (size unknown)\nDeleted 2 snapshots · total reported size: 1.0 KiB + 1 unknown\n"},
		{name: "unknown size", kind: "snapshot", sizes: []*uint64{nil},
			want: "Deleted snapshot item (size unknown)\nDeleted 1 snapshot · total reported size: unknown\n"},
		{name: "partial VM failure", kind: "VM", sizes: []*uint64{nil}, err: failure,
			want: "Deleted VM item\nDeleted 1 VM\n"},
		{name: "partial snapshot failure", kind: "snapshot", sizes: []*uint64{&size}, err: failure,
			want: "Deleted snapshot item (1.0 KiB)\nDeleted 1 snapshot · total reported size: 1.0 KiB\n"},
		{name: "failure without deletions", kind: "VM", err: failure, want: "Deleted 0 VMs\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			err := reportRemovals(&out, tc.kind, func(observer sandbox.RemovalObserver) error {
				for _, size := range tc.sizes {
					before := out.Len()
					observer("item", size)
					require.Greater(t, out.Len(), before, "each deletion must be printed immediately")
				}
				return tc.err
			})
			require.ErrorIs(t, err, tc.err)
			require.Equal(t, tc.want, out.String())
		})
	}
}

func TestReportRemovalsPreservesOutputAndDeletionFailures(t *testing.T) {
	t.Parallel()
	failure := errors.New("deletion failed")
	var observed int
	err := reportRemovals(failingSnapshotWriter{}, "VM", func(observer sandbox.RemovalObserver) error {
		for _, name := range []string{"first", "second"} {
			observer(name, nil)
			observed++
		}
		return failure
	})
	require.Equal(t, 2, observed, "logging errors must not interrupt deletion")
	require.ErrorIs(t, err, failure)
	require.ErrorContains(t, err, "write failed")
}

func TestReportRemovalsPropagatesSummaryWriteFailure(t *testing.T) {
	t.Parallel()
	err := reportRemovals(failingSnapshotWriter{}, "VM", func(sandbox.RemovalObserver) error { return nil })
	require.ErrorContains(t, err, "write failed")
}
