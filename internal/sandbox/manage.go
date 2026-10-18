package sandbox

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"opencode-sandbox/internal/workspace"

	m "github.com/superradcompany/microsandbox/sdk/go"
)

// List returns all labeled VMs, including stopped VMs, sorted by name.
func List(ctx context.Context) ([]*m.SandboxHandle, error) {
	if err := configureRuntime(); err != nil {
		return nil, fmt.Errorf("locate Microsandbox runtime: %w", err)
	}
	var vms []*m.SandboxHandle
	cursor := ""
	for {
		opts := []m.SandboxListOption{m.WithListLabels(map[string]string{"opencode-sandbox": ""})}
		if cursor != "" {
			opts = append(opts, m.WithListCursor(cursor))
		}
		page, err := m.ListSandboxesWith(ctx, opts...)
		if err != nil {
			return nil, fmt.Errorf("list managed VMs: %w", err)
		}
		vms = append(vms, page.Sandboxes...)
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}
	sort.Slice(vms, func(i, j int) bool { return vms[i].Name() < vms[j].Name() })
	return vms, nil
}

// Remove stops and deletes only the VM for the exact directory.
// Missing VMs are a no-op; workspace snapshots are left untouched.
func Remove(ctx context.Context, directory string, observers ...RemovalObserver) error {
	w, err := workspace.Resolve(ctx, directory)
	if err != nil {
		return err
	}
	vms, err := List(ctx)
	if err != nil {
		return err
	}
	name := sandboxName(w)
	for _, vm := range vms {
		if vm.Name() == name {
			if err := vm.Destroy(ctx, m.WithDestroyTimeout(30*time.Second)); err != nil {
				return fmt.Errorf("remove %s: %w", name, err)
			}
			notifyRemoval(observers, name, nil)
			return nil
		}
	}
	return nil
}

// RemoveAll stops and deletes all labeled VMs, attempting every target on failure.
func RemoveAll(ctx context.Context, observers ...RemovalObserver) error {
	// Collect all pages before deleting, so removal cannot disturb pagination.
	vms, err := List(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, vm := range vms {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		if err := vm.Destroy(ctx, m.WithDestroyTimeout(30*time.Second)); err != nil {
			errs = append(errs, fmt.Errorf("remove %s: %w", vm.Name(), err))
			continue
		}
		notifyRemoval(observers, vm.Name(), nil)
	}
	return errors.Join(errs...)
}
