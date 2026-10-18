package sandbox

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	m "github.com/superradcompany/microsandbox/sdk/go"
)

// SnapshotInfo describes a managed repository snapshot using runtime metadata.
type SnapshotInfo struct {
	Reference string
	CreatedAt time.Time
	SizeBytes *uint64
	Baseline  bool
}

// SnapshotRepository groups snapshots by repository identity, not display name.
type SnapshotRepository struct {
	ID, Name, Path string
	Snapshots      []SnapshotInfo
}

// SnapshotListing separates the current repository from all other repositories.
// Current is nil when the working directory is outside Git.
type SnapshotListing struct {
	Current      *SnapshotRepository
	Repositories []SnapshotRepository
}

// ListSnapshots reads native snapshot metadata without loading config or secrets.
func ListSnapshots(ctx context.Context, directory string) (SnapshotListing, error) {
	var listing SnapshotListing
	repo, err := snapshotRepo(ctx, directory)
	if err == nil {
		listing.Current = &SnapshotRepository{ID: repo.id, Name: repo.name, Path: filepath.Dir(repo.common)}
	} else if !strings.Contains(err.Error(), "not a git repository") {
		return listing, err
	}
	if err := configureRuntime(); err != nil {
		return listing, fmt.Errorf("locate Microsandbox runtime: %w", err)
	}
	members, err := m.Snapshot.List(ctx)
	if err != nil {
		return listing, fmt.Errorf("list repository snapshots: %w", err)
	}
	repositories := map[string]*SnapshotRepository{}
	if listing.Current != nil {
		repositories[listing.Current.ID] = listing.Current
	}
	heads := map[string]string{}
	for _, member := range members {
		group := member.Group()
		if group == nil || !strings.HasPrefix(*group, "oc-repo-") || member.Scope() != m.SnapshotScopeDisk {
			continue
		}
		artifact, err := member.Open(ctx)
		if err != nil {
			return listing, fmt.Errorf("read snapshot %s: %w", member.Reference(), err)
		}
		labels := artifact.Labels()
		if _, managed := labels["opencode-sandbox"]; !managed || labels["kind"] != "repo-baseline" || labels["repo-id"] == "" {
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
		r := repositories[labels["repo-id"]]
		if r == nil {
			r = &SnapshotRepository{ID: labels["repo-id"], Name: labels["repo-name"], Path: labels["repo-path"]}
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
			listing.Repositories = append(listing.Repositories, *r)
		}
	}
	sort.Slice(listing.Repositories, func(i, j int) bool {
		a, b := listing.Repositories[i], listing.Repositories[j]
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Path < b.Path
	})
	return listing, nil
}
