package sandbox

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	m "github.com/superradcompany/microsandbox/sdk/go"
)

type pruneVM struct {
	name        string
	workspaceID string
	remove      func(context.Context) error
}

type pruneSnapshot struct {
	workspace workspaceSnapshot
	remove    func(context.Context) error
}

type pruneResources struct {
	vms       []pruneVM
	snapshots []pruneSnapshot
}

func discoverPruneResources(ctx context.Context) (pruneResources, error) {
	var inventory pruneResources
	vms, err := List(ctx)
	if err != nil {
		return inventory, err
	}
	for _, vm := range vms {
		config, err := vm.Config()
		if err != nil {
			return inventory, fmt.Errorf("read VM ownership for %s: %w", vm.Name(), err)
		}
		inventory.vms = append(inventory.vms, pruneVM{
			name:        vm.Name(),
			workspaceID: config.Labels["workspace-id"],
			remove: func(ctx context.Context) error {
				return vm.Destroy(ctx, m.WithDestroyForce(), m.WithDestroyTimeout(30*time.Second))
			},
		})
	}
	inventory.snapshots, err = discoverPruneSnapshots(ctx)
	return inventory, err
}

func discoverPruneSnapshots(ctx context.Context) ([]pruneSnapshot, error) {
	members, err := m.Snapshot.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list workspace snapshots: %w", err)
	}
	groups := map[string]workspaceSnapshot{}
	for _, member := range members {
		identity, managed, err := readSnapshotWorkspace(ctx, member)
		if err != nil {
			return nil, err
		}
		if managed {
			groups[identity.group] = identity
		}
	}
	var snapshots []pruneSnapshot
	for _, identity := range groups {
		snapshots = append(snapshots, pruneSnapshot{workspace: identity, remove: func(ctx context.Context) error {
			return identity.removeLocked(ctx)
		}})
	}
	sort.Slice(snapshots, func(i, j int) bool { return snapshots[i].workspace.group < snapshots[j].workspace.group })
	return snapshots, nil
}

func (p *PrunePlan) addSharedMSB(ctx context.Context) error {
	images, err := m.Image.List(ctx)
	if err != nil {
		return fmt.Errorf("list all MSB images: %w", err)
	}
	sort.Slice(images, func(i, j int) bool { return images[i].Reference() < images[j].Reference() })
	for _, image := range images {
		p.shared = append(p.shared, pruneStep{
			name: "MSB image " + image.Reference(),
			run:  func(ctx context.Context) error { return m.Image.Remove(ctx, image.Reference(), true) },
		})
	}
	// Also collect untagged backing left by earlier removals.
	p.shared = append(p.shared, pruneStep{
		name: "unreferenced MSB image backing (owned backing is retained)",
		run:  pruneMSBImageBacking,
	})
	preview, err := m.PruneStorage(ctx, m.StoragePruneOptions{DryRun: true})
	if err != nil {
		return fmt.Errorf("inventory MSB runtime caches: %w", err)
	}
	for _, entry := range preview.Entries {
		if entry.Error != nil {
			return fmt.Errorf("inventory MSB cache %q: %s", entry.Path, *entry.Error)
		}
	}
	p.shared = append(p.shared, pruneStep{
		name: "all MSB runtime caches (runtime-owned files may be retained)",
		run:  pruneMSBCaches,
	})
	return nil
}

func pruneMSBImageBacking(ctx context.Context) error {
	report, err := m.Image.Prune(ctx)
	if err != nil {
		return err
	}
	if report.SkippedInUse != 0 {
		return fmt.Errorf("MSB retained %d busy image cache entries", report.SkippedInUse)
	}
	return nil
}

func pruneMSBCaches(ctx context.Context) error {
	// Rescan after VM/snapshot removal: previously owned RAM may now be free.
	// The MSB interface always respects ownership; it has no force option.
	report, err := m.PruneStorage(ctx, m.StoragePruneOptions{})
	if err != nil {
		return err
	}
	return cachePruneErrors(report)
}

func cachePruneErrors(report *m.StoragePruneReport) error {
	var errs []error
	for _, entry := range report.Entries {
		if entry.State == "removed" {
			continue
		}
		detail := entry.State
		if entry.Error != nil {
			detail += ": " + *entry.Error
		}
		errs = append(errs, fmt.Errorf("MSB cache %q retained: %s", entry.Path, detail))
	}
	if report.Truncated {
		errs = append(errs, fmt.Errorf("MSB cache report truncated; cleanup may be incomplete"))
	}
	return errors.Join(errs...)
}
