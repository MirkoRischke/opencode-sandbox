package sandbox

import (
	"context"
	"fmt"
	"sort"
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
	repositories, err := collectSnapshots(ctx, members, listing.Current)
	if err != nil {
		return listing, err
	}
	listing.sortWorkspaces(repositories)
	return listing, nil
}

func collectSnapshots(ctx context.Context, members []*m.SnapshotHandle, current *SnapshotWorkspace) (map[string]*SnapshotWorkspace, error) {
	repositories := map[string]*SnapshotWorkspace{}
	if current != nil {
		repositories[current.ID] = current
	}
	heads := map[string]string{}
	for _, member := range members {
		repo, managed, err := readSnapshotWorkspace(ctx, member)
		if err != nil {
			return nil, err
		}
		if !managed {
			continue
		}
		headID, known := heads[repo.group]
		if !known {
			head, err := m.Snapshot.Get(ctx, repo.group)
			if err != nil {
				return nil, fmt.Errorf("find baseline for %s: %w", repo.group, err)
			}
			headID = head.ID()
			heads[repo.group] = headID
		}
		r := repositories[repo.id]
		if r == nil {
			r = &SnapshotWorkspace{ID: repo.id, Name: repo.name, Path: repo.root}
			repositories[r.ID] = r
		}
		r.Snapshots = append(r.Snapshots, SnapshotInfo{
			Reference: member.Reference(), CreatedAt: member.CreatedAt(),
			SizeBytes: member.SizeBytes(), Baseline: member.ID() == headID,
		})
	}
	return repositories, nil
}

func (listing *SnapshotListing) sortWorkspaces(repositories map[string]*SnapshotWorkspace) {
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
}
