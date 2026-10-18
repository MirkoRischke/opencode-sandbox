package sandbox

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

type worktreePaths struct {
	root   string
	common string
}

func worktreeRoot(ctx context.Context, directory string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", directory, "rev-parse", "--show-toplevel").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("discover worktree: %w: %s", err, out)
	}
	return filepath.EvalSymlinks(strings.TrimSpace(string(out)))
}

func discoverWorktree(ctx context.Context, directory string) (worktreePaths, error) {
	root, err := worktreeRoot(ctx, directory)
	if err != nil {
		return worktreePaths{}, err
	}
	common, err := commonGitDir(ctx, directory)
	if err != nil {
		return worktreePaths{}, err
	}
	return worktreePaths{root: root, common: common}, nil
}

func commonGitDir(ctx context.Context, directory string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", directory, "rev-parse", "--path-format=absolute", "--git-common-dir").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("discover repository: %w: %s", err, out)
	}
	return filepath.EvalSymlinks(strings.TrimSpace(string(out)))
}
