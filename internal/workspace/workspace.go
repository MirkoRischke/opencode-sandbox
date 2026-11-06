// Package workspace resolves an exact host directory and its optional Git mounts.
// It has no dependency on launcher configuration or the VM runtime.
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

// Workspace is a validated directory, independent of its repository type.
type Workspace interface {
	Root() string
	ID() string
	// RepositoryID groups linked Git worktrees; non-Git directories use ID.
	RepositoryID() string
	RepositoryName() string
	Mounts(guestRoot string) []Mount
}

// Mount describes a writable workspace or Git metadata bind mount.
type Mount struct {
	Source, Target string
}

type base struct {
	root string
}

func (b base) Root() string           { return b.root }
func (b base) ID() string             { return fmt.Sprintf("%x", sha256.Sum256([]byte(b.root))) }
func (b base) RepositoryID() string   { return b.ID() }
func (b base) RepositoryName() string { return filepath.Base(b.root) }

// Directory is a workspace without Git metadata at its root.
type Directory struct{ base }

// GitRepository is a checkout with its own Git metadata.
type GitRepository struct {
	base
	metadata gitMetadata
}

// GitWorktree is a linked checkout sharing another checkout's Git metadata.
type GitWorktree struct {
	base
	metadata gitMetadata
}

type gitMetadata struct {
	dir, common string
}

func (r GitRepository) RepositoryID() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(r.metadata.common)))
}

func (w GitWorktree) RepositoryID() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(w.metadata.common)))
}

func (r GitRepository) RepositoryName() string {
	return filepath.Base(filepath.Dir(r.metadata.common))
}

func (w GitWorktree) RepositoryName() string {
	return filepath.Base(filepath.Dir(w.metadata.common))
}

func (d Directory) Mounts(guestRoot string) []Mount {
	return []Mount{{Source: d.root, Target: guestRoot}}
}

func (r GitRepository) Mounts(guestRoot string) []Mount {
	return gitMounts(r.root, guestRoot, r.metadata)
}

func (w GitWorktree) Mounts(guestRoot string) []Mount {
	return gitMounts(w.root, guestRoot, w.metadata)
}

func gitMounts(root, guestRoot string, metadata gitMetadata) []Mount {
	mounts := []Mount{{Source: root, Target: guestRoot}}
	for _, path := range []string{metadata.common, metadata.dir} {
		covered := false
		for _, mount := range mounts {
			if mount.Source == mount.Target && within(path, mount.Source) {
				covered = true
				break
			}
		}
		if !covered {
			mounts = append(mounts, Mount{Source: path, Target: path})
		}
	}
	return mounts
}

// Resolve never searches parents for a workspace or for Git metadata. A present
// but invalid .git marker is an error, not a non-Git fallback.
func Resolve(ctx context.Context, directory string) (Workspace, error) {
	root, err := canonicalRoot(directory)
	if err != nil {
		return nil, err
	}
	b := base{root: root}
	if _, err := os.Lstat(filepath.Join(root, ".git")); os.IsNotExist(err) {
		return Directory{base: b}, nil
	} else if err != nil {
		return nil, fmt.Errorf("inspect workspace Git metadata: %w", err)
	}
	dir, err := gitPath(ctx, root, "--absolute-git-dir")
	if err != nil {
		return nil, err
	}
	common, err := gitPath(ctx, root, "--git-common-dir")
	if err != nil {
		return nil, err
	}
	top, err := gitPath(ctx, root, "--show-toplevel")
	if err != nil {
		return nil, err
	}
	if top != root {
		return nil, fmt.Errorf("invalid Git metadata in %q: refers to a different workspace %q", root, top)
	}
	metadata := gitMetadata{dir: dir, common: common}
	if dir != common {
		return GitWorktree{base: b, metadata: metadata}, nil
	}
	return GitRepository{base: b, metadata: metadata}, nil
}

func canonicalRoot(directory string) (string, error) {
	root, err := filepath.Abs(directory)
	if err != nil {
		return "", fmt.Errorf("resolve workspace: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve workspace: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("resolve workspace: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspace %q is not a directory", root)
	}
	if err := validateRoot(root, info); err != nil {
		return "", err
	}
	return root, nil
}

func gitPath(ctx context.Context, root, option string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "--path-format=absolute", option)
	// Host Git overrides must not redirect discovery to a different checkout.
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GIT_") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("read Git metadata in %q: %w: %s", root, err, out)
	}
	path, err := filepath.EvalSymlinks(strings.TrimSpace(string(out)))
	if err != nil {
		return "", fmt.Errorf("resolve Git metadata in %q: %w", root, err)
	}
	return path, nil
}

func within(path, parent string) bool {
	return path == parent || strings.HasPrefix(path, parent+string(filepath.Separator))
}
