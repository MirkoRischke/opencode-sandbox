package cli

import (
	"fmt"
	"io"
	"text/tabwriter"

	"opencode-sandbox/internal/sandbox"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
)

func newSnapshotListCommand() *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List all repository snapshots, highlighting the current repository",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			listing, err := sandbox.ListSnapshots(cmd.Context(), ".")
			if err != nil {
				return err
			}
			return writeSnapshotListing(cmd.OutOrStdout(), listing)
		},
	}
}

func writeSnapshotListing(out io.Writer, listing sandbox.SnapshotListing) error {
	if listing.Current != nil {
		if _, err := fmt.Fprintf(out, "Current repository: %s\n%s\n\n", listing.Current.Name, listing.Current.Path); err != nil {
			return err
		}
		if err := writeSnapshotTable(out, listing.Current.Snapshots); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(out, "\nOther repositories"); err != nil {
			return err
		}
	} else if _, err := fmt.Fprintln(out, "All repositories"); err != nil {
		return err
	}
	if len(listing.Repositories) == 0 {
		_, err := fmt.Fprintln(out, "\nNo snapshots found.")
		return err
	}
	for _, repo := range listing.Repositories {
		if _, err := fmt.Fprintf(out, "\n%s · %s\n", repo.Name, repo.Path); err != nil {
			return err
		}
		if err := writeSnapshotTable(out, repo.Snapshots); err != nil {
			return err
		}
	}
	return nil
}

func writeSnapshotTable(out io.Writer, snapshots []sandbox.SnapshotInfo) error {
	if len(snapshots) == 0 {
		_, err := fmt.Fprintln(out, "No snapshot.")
		return err
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "SNAPSHOT\tCREATED (UTC)\tSIZE\tBASELINE"); err != nil {
		return err
	}
	for _, snapshot := range snapshots {
		baseline := "no"
		if snapshot.Baseline {
			baseline = "yes"
		}
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", snapshot.Reference,
			snapshot.CreatedAt.UTC().Format("2006-01-02 15:04"), snapshotSize(snapshot.SizeBytes), baseline); err != nil {
			return err
		}
	}
	return w.Flush()
}

func snapshotSize(size *uint64) string {
	if size == nil {
		return "—"
	}
	return humanize.IBytes(*size)
}
