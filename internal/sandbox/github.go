package sandbox

import (
	"context"
	"fmt"
)

func (s *Sandbox) configureGitHub(ctx context.Context, enabled bool) error {
	if !enabled {
		return nil
	}
	if err := s.Exec(ctx, nil, nil, nil, "gh", "auth", "setup-git", "--hostname", "github.com"); err != nil {
		return fmt.Errorf("configure guest GitHub CLI (rebuild the image and recreate this VM if GitHub support is missing): %w", err)
	}
	return nil
}
