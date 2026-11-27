package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"opencode-sandbox/internal/sandbox"
	"opencode-sandbox/internal/workspace"

	"github.com/spf13/cobra"
)

func newStateCommand() *cobra.Command {
	state := &cobra.Command{
		Use:   "state",
		Short: "Locate persistent workspace state",
		Args:  cobra.NoArgs,
		RunE:  requireSubcommand,
	}
	var repo, all bool
	path := &cobra.Command{
		Use:   "path",
		Short: "Print the absolute path to a persistent state directory",
		Long:  "Print the absolute path to a persistent state directory. If it does not exist yet, warn on stderr without creating it.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := hostStateDir(os.Getenv("XDG_STATE_HOME"), os.Getenv("HOME"))
			if err != nil {
				return err
			}
			dir, err = filepath.Abs(dir)
			if err != nil {
				return fmt.Errorf("resolve host state directory: %w", err)
			}
			target := sandbox.StateRoot(dir)
			if !all {
				target, err = scopedStatePath(cmd.Context(), dir, repo)
				if err != nil {
					return err
				}
			}
			info, err := os.Stat(target)
			switch {
			case errors.Is(err, os.ErrNotExist):
				if _, err := fmt.Fprintln(cmd.ErrOrStderr(), "Warning: persistent state directory does not exist yet"); err != nil {
					return err
				}
			case err != nil:
				return fmt.Errorf("inspect persistent state directory: %w", err)
			case !info.IsDir():
				return fmt.Errorf("persistent state path %q is not a directory", target)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), target)
			return err
		},
	}
	path.Flags().BoolVar(&repo, "repo", false, "Show the current Git repository's state directory, including deleted worktrees")
	path.Flags().BoolVar(&all, "all", false, "Show the top-level state directory for all workspaces")
	path.MarkFlagsMutuallyExclusive("repo", "all")
	state.AddCommand(path)
	return state
}

func scopedStatePath(ctx context.Context, stateDir string, repo bool) (string, error) {
	w, err := workspace.Resolve(ctx, ".")
	if err != nil {
		return "", err
	}
	if !repo {
		return sandbox.WorkspaceStatePath(stateDir, w), nil
	}
	if _, nonGit := w.(workspace.Directory); nonGit {
		return "", fmt.Errorf("--repo requires a Git checkout or worktree root")
	}
	return sandbox.RepositoryStatePath(stateDir, w), nil
}
