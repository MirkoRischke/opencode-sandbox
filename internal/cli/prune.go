package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"opencode-sandbox/internal/sandbox"

	"github.com/spf13/cobra"
)

type pruneInventory interface {
	Targets() []string
	Execute(context.Context) error
}

type pruneFlags struct {
	repo   bool
	all    bool
	yes    bool
	dryRun bool
}

func newPruneCommand() *cobra.Command {
	return pruneCommand(func(ctx context.Context, repo, all bool) (pruneInventory, error) {
		stateDir, err := hostStateDir(os.Getenv("XDG_STATE_HOME"), os.Getenv("HOME"))
		if err != nil {
			return nil, err
		}
		return sandbox.PlanPrune(ctx, sandbox.PruneOptions{
			Directory: ".", StateDir: stateDir, Repository: repo, All: all,
		})
	})
}

func pruneCommand(inventory func(context.Context, bool, bool) (pruneInventory, error)) *cobra.Command {
	var flags pruneFlags
	cmd := &cobra.Command{
		Use: "prune", Short: "Delete this workspace's VMs, snapshots, history, logs, and diagnostics",
		Long: "Delete all workspace resources, including conversation history. Running VMs are stopped and removed.\n\n" +
			"Default: current directory/worktree. --repo includes other tracked worktrees, even deleted ones.\n" +
			"--all includes every managed workspace plus ALL Microsandbox images and runtime caches,\n" +
			"including images used outside this tool. MSB retains backing still owned by unrelated resources.\n" +
			"Configuration, credentials, the gateway, Docker resources, and workspace source files are preserved.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			plan, err := inventory(cmd.Context(), flags.repo, flags.all)
			if err != nil {
				return err
			}
			return runPrune(cmd, plan, flags)
		},
	}
	cmd.Flags().BoolVar(&flags.repo, "repo", false, "Delete resources for this repo and all its tracked worktrees")
	cmd.Flags().BoolVar(&flags.all, "all", false, "Delete all managed workspace resources AND all MSB images/runtime caches")
	cmd.Flags().BoolVarP(&flags.yes, "yes", "y", false, "Skip deletion confirmation")
	cmd.Flags().BoolVar(&flags.dryRun, "dry-run", false, "List targets without deleting anything")
	cmd.MarkFlagsMutuallyExclusive("repo", "all")
	return cmd
}

func runPrune(cmd *cobra.Command, plan pruneInventory, flags pruneFlags) error {
	targets := plan.Targets()
	if len(targets) == 0 {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "No resources to prune.")
		return err
	}
	if err := showPruneTargets(cmd, targets, flags.all); err != nil {
		return err
	}
	if flags.dryRun {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "Dry run: nothing deleted.")
		return err
	}
	if !flags.yes {
		if err := confirmPrune(cmd); err != nil {
			return err
		}
	}
	if err := plan.Execute(cmd.Context()); err != nil {
		return err
	}
	_, err := fmt.Fprintln(cmd.OutOrStdout(), "Prune complete.")
	return err
}

func showPruneTargets(cmd *cobra.Command, targets []string, all bool) error {
	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Prune targets (conversation history is permanently deleted):"); err != nil {
		return err
	}
	for _, target := range targets {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  %q\n", target); err != nil {
			return err
		}
	}
	if all {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "WARNING: --all also deletes ALL MSB images and prunes machine-wide runtime caches, including unrelated ones.")
		return err
	}
	return nil
}

func confirmPrune(cmd *cobra.Command) error {
	if _, err := fmt.Fprint(cmd.ErrOrStderr(), "Permanently delete these resources? [y/N] "); err != nil {
		return err
	}
	answer, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil {
		return fmt.Errorf("prune not confirmed (use --yes for unattended deletion): %w", err)
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer != "y" && answer != "yes" {
		return fmt.Errorf("prune not confirmed")
	}
	return nil
}
