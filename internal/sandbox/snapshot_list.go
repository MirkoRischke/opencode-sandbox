package sandbox

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	m "github.com/superradcompany/microsandbox/sdk/go"
)

// SnapshotInfo describes a managed workspace snapshot using runtime metadata.
type SnapshotInfo struct {
	Reference string
	CreatedAt time.Time
	SizeBytes *uint64
	Baseline  bool
}

// SnapshotWorkspace groups snapshots by canonical directory identity.
type SnapshotWorkspace struct {
	ID, Name, Path string
	Snapshots      []SnapshotInfo
}

// SnapshotListing separates the current workspace from all other workspaces.
// Current is nil when no directory was supplied (global listing).
type SnapshotListing struct {
	Current    *SnapshotWorkspace
	Workspaces []SnapshotWorkspace
}

// ListSnapshots reads native snapshot metadata without loading config or secrets.
func ListSnapshots(ctx context.Context, directory string) (SnapshotListing, error) {
	var listing SnapshotListing
	if directory != "" {
		repo, err := resolveSnapshotWorkspace(ctx, directory)
		if err != nil {
			return listing, err
		}
		listing.Current = &SnapshotWorkspace{ID: repo.id, Name: repo.name, Path: repo.root}
	}
	if err := configureRuntime(); err != nil {
		return listing, fmt.Errorf("locate Microsandbox runtime: %w", err)
	}
	members, err := m.Snapshot.List(ctx)
	if err != nil {
		return listing, fmt.Errorf("list workspace snapshots: %w", err)
	}
	repositories := map[string]*SnapshotWorkspace{}
	if listing.Current != nil {
		repositories[listing.Current.ID] = listing.Current
	}
	heads := map[string]string{}
	for _, member := range members {
		group := member.Group()
		if group == nil || !strings.HasPrefix(*group, "oc-workspace-") || member.Scope() != m.SnapshotScopeDisk {
			continue
		}
		artifact, err := member.Open(ctx)
		if err != nil {
			return listing, fmt.Errorf("read snapshot %s: %w", member.Reference(), err)
		}
		labels := artifact.Labels()
		if _, managed := labels["opencode-sandbox"]; !managed || labels["kind"] != "workspace-baseline" || labels["workspace-id"] == "" {
			continue
		}
		headID, known := heads[*group]
		if !known {
			head, err := m.Snapshot.Get(ctx, *group)
			if err != nil {
				return listing, fmt.Errorf("find baseline for %s: %w", *group, err)
			}
			headID = head.ID()
			heads[*group] = headID
		}
		r := repositories[labels["workspace-id"]]
		if r == nil {
			r = &SnapshotWorkspace{ID: labels["workspace-id"], Name: labels["workspace-name"], Path: labels["workspace-path"]}
			repositories[r.ID] = r
		}
		r.Snapshots = append(r.Snapshots, SnapshotInfo{
			Reference: member.Reference(), CreatedAt: member.CreatedAt(),
			SizeBytes: member.SizeBytes(), Baseline: member.ID() == headID,
		})
	}
	for _, r := range repositories {
		sort.Slice(r.Snapshots, func(i, j int) bool {
			a, b := r.Snapshots[i], r.Snapshots[j]
			if !a.CreatedAt.Equal(b.CreatedAt) {
				return a.CreatedAt.After(b.CreatedAt)
			}
			return a.Reference < b.Reference
		})
		if r != listing.Current {
			listing.Workspaces = append(listing.Workspaces, *r)
		}
	}
	sort.Slice(listing.Workspaces, func(i, j int) bool {
		a, b := listing.Workspaces[i], listing.Workspaces[j]
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Path < b.Path
	})
	return listing, nil
}
