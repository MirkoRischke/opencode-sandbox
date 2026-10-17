package sandbox

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
)

func sandboxName(worktree, commonGitDir string) string {
	repo := namePart(filepath.Base(filepath.Dir(commonGitDir)))
	tree := namePart(filepath.Base(worktree))
	// Identity follows the canonical worktree path, not the current branch.
	hash := sha256.Sum256([]byte(worktree))
	return fmt.Sprintf("oc-%s-%s-%x", repo, tree, hash[:4])
}

func namePart(name string) string {
	parts := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	})
	name = strings.Join(parts, "-")
	if name == "" {
		return "unnamed"
	}
	// Bound both labels so the full name stays below the runtime's 128-byte limit.
	if len(name) > 48 {
		name = strings.TrimRight(name[:48], "-")
	}
	return name
}
