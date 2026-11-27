package workspace

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CurrentGitRepositoryID discovers the current repository even from a subdirectory.
func CurrentGitRepositoryID(ctx context.Context, directory string) (string, error) {
	common, err := gitPath(ctx, directory, "--git-common-dir", os.Environ())
	if err != nil {
		return "", err
	}
	return GitRepositoryID(common), nil
}

// GitRepositoryID identifies a repository by its canonical Git common directory.
func GitRepositoryID(commonDir string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(commonDir)))
}

// ListedWorktrees reads Git's worktree registry, including prunable entries.
func ListedWorktrees(ctx context.Context, commonDir string) (map[string]bool, error) {
	cmd := exec.CommandContext(ctx, "git", "--git-dir="+commonDir, "worktree", "list", "--porcelain", "-z")
	cmd.Dir = commonDir
	cmd.Env = []string{"GIT_OPTIONAL_LOCKS=0"}
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GIT_") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list Git worktrees in %q: %w", commonDir, err)
	}
	records, valid := strings.CutSuffix(string(output), "\x00\x00")
	if !valid {
		return nil, fmt.Errorf("invalid Git worktree list in %q", commonDir)
	}
	paths := make(map[string]bool)
	for _, record := range strings.Split(records, "\x00\x00") {
		first, _, _ := strings.Cut(record, "\x00")
		path, ok := strings.CutPrefix(first, "worktree ")
		if !ok || !filepath.IsAbs(path) {
			return nil, fmt.Errorf("invalid Git worktree entry in %q", commonDir)
		}
		paths[filepath.Clean(path)] = true
	}
	return paths, nil
}
