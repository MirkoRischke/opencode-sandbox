package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/require"
)

func TestSnapshotListingSections(t *testing.T) {
	t.Parallel()
	size := uint64(2560 * 1024 * 1024)
	created := time.Date(2026, 10, 18, 16, 32, 0, 0, time.FixedZone("local", 2*60*60))
	listing := sandbox.SnapshotListing{
		Current: &sandbox.SnapshotRepository{Name: "current", Path: "/projects/current", Snapshots: []sandbox.SnapshotInfo{
			{Reference: "current:prepared-new", CreatedAt: created, SizeBytes: &size, Baseline: true},
			{Reference: "current:prepared-old", CreatedAt: created.Add(-time.Hour)},
		}},
		Repositories: []sandbox.SnapshotRepository{
			{Name: "other", Path: "/projects/other", Snapshots: []sandbox.SnapshotInfo{
				{Reference: "other:prepared-new", CreatedAt: created, Baseline: true},
			}},
		},
	}
	var out bytes.Buffer
	require.NoError(t, writeSnapshotListing(&out, listing))
	text := out.String()
	require.Contains(t, text, "Current repository: current\n/projects/current\n")
	require.Contains(t, text, "Other repositories\n\nother · /projects/other")
	require.Less(t, strings.Index(text, "current:prepared-new"), strings.Index(text, "Other repositories"))
	require.Less(t, strings.Index(text, "Other repositories"), strings.Index(text, "other:prepared-new"))
	require.Contains(t, text, "CREATED (UTC)")
	var rows []string
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, ":prepared-") {
			rows = append(rows, strings.Join(strings.Fields(line), " "))
		}
	}
	require.Equal(t, []string{
		"current:prepared-new 2026-10-18 14:32 2.5 GiB yes",
		"current:prepared-old 2026-10-18 13:32 — no",
		"other:prepared-new 2026-10-18 14:32 — yes",
	}, rows)
}

func TestSnapshotListingEmptyStates(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		listing sandbox.SnapshotListing
		want    string
	}{
		{"outside Git", sandbox.SnapshotListing{}, "All repositories\n\nNo snapshots found.\n"},
		{"current has no snapshot", sandbox.SnapshotListing{Current: &sandbox.SnapshotRepository{Name: "repo", Path: "/repo"}},
			"Current repository: repo\n/repo\n\nNo snapshot.\n\nOther repositories\n\nNo snapshots found.\n"},
		{"outside Git with snapshots", sandbox.SnapshotListing{Repositories: []sandbox.SnapshotRepository{{Name: "repo", Path: "/repo"}}},
			"All repositories\n\nrepo · /repo\nNo snapshot.\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			require.NoError(t, writeSnapshotListing(&out, tc.listing))
			require.Equal(t, tc.want, out.String())
		})
	}
}

func TestSnapshotSize(t *testing.T) {
	t.Parallel()
	require.Equal(t, "—", snapshotSize(nil))
	for _, tc := range []struct {
		size uint64
		want string
	}{
		{0, "0 B"}, {1023, "1023 B"}, {1024, "1.0 KiB"},
		{1024 * 1024, "1.0 MiB"}, {1536 * 1024 * 1024, "1.5 GiB"},
		{1 << 40, "1.0 TiB"}, {1 << 50, "1.0 PiB"}, {1 << 60, "1.0 EiB"},
	} {
		require.Equal(t, tc.want, snapshotSize(&tc.size))
	}
}

type failingSnapshotWriter struct{}

func (failingSnapshotWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestSnapshotListingPropagatesWriteError(t *testing.T) {
	t.Parallel()
	require.ErrorContains(t, writeSnapshotListing(failingSnapshotWriter{}, sandbox.SnapshotListing{}), "write failed")
	require.ErrorContains(t, writeSnapshotTable(failingSnapshotWriter{}, []sandbox.SnapshotInfo{{Reference: "snapshot"}}), "write failed")
}
