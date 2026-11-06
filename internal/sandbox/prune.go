package sandbox

import (
	"context"
	"errors"
	"fmt"
)

// PruneOptions selects workspace resources; All also includes shared MSB storage.
type PruneOptions struct {
	Directory  string
	StateDir   string
	Repository bool
	All        bool
}

type pruneStep struct {
	name string
	run  func(context.Context) error
}

// PrunePlan inventories deletions without performing them. Execute is the only
// destructive operation; callers can show Targets before requesting approval.
type PrunePlan struct {
	runtime  []pruneStep
	local    []pruneStep
	metadata []pruneStep
	shared   []pruneStep
}

// PlanPrune discovers workspace resources and, for --all, shared MSB storage.
func PlanPrune(ctx context.Context, options PruneOptions) (*PrunePlan, error) {
	scope, err := resolvePruneScope(ctx, options)
	if err != nil {
		return nil, err
	}
	inventory, err := discoverPruneResources(ctx)
	if err != nil {
		return nil, err
	}
	plan, err := planWorkspacePrune(scope, inventory)
	if err != nil {
		return nil, err
	}
	if options.All {
		if err := plan.addSharedMSB(ctx); err != nil {
			return nil, err
		}
	}
	return plan, nil
}

func (p *PrunePlan) Targets() []string {
	var targets []string
	for _, phase := range [][]pruneStep{p.runtime, p.local, p.metadata, p.shared} {
		for _, step := range phase {
			targets = append(targets, step.name)
		}
	}
	return targets
}

// Execute removes VMs/snapshots before host data, then shared MSB storage.
// Failed workspace cleanup retains ownership records so a retry can find them.
func (p *PrunePlan) Execute(ctx context.Context) error {
	workspaceErr := p.removeWorkspaceResources(ctx)
	sharedErr := runPruneSteps(ctx, p.shared)
	return errors.Join(workspaceErr, sharedErr)
}

func (p *PrunePlan) removeWorkspaceResources(ctx context.Context) error {
	if err := runPruneSteps(ctx, p.runtime); err != nil {
		// Do not remove a database that a surviving VM could still be writing.
		return err
	}
	if err := runPruneSteps(ctx, p.local); err != nil {
		return err
	}
	return runPruneSteps(ctx, p.metadata)
}

func runPruneSteps(ctx context.Context, steps []pruneStep) error {
	var errs []error
	for _, step := range steps {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		if err := step.run(ctx); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", step.name, err))
		}
	}
	return errors.Join(errs...)
}
