package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"opencode-sandbox/internal/logging"

	m "github.com/superradcompany/microsandbox/sdk/go"
)

// Docker is the source of truth; Microsandbox stores identity-addressed imports.
// Only fresh image-based creations cross this seam, never resumes or restores.
func resolveImage(ctx context.Context, reference string) (string, error) {
	r := imageResolver{
		docker: func(ctx context.Context, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, "docker", args...).CombinedOutput()
		},
		cached: func(ctx context.Context, reference string) (bool, error) {
			_, err := m.Image.Get(ctx, reference)
			if m.IsKind(err, m.ErrImageNotFound) {
				return false, nil
			}
			return err == nil, err
		},
		load: func(ctx context.Context, archive, reference string) error {
			_, err := m.Image.Load(ctx, archive, reference)
			return err
		},
	}
	return r.resolve(ctx, reference)
}

type imageResolver struct {
	docker func(context.Context, ...string) ([]byte, error)
	cached func(context.Context, string) (bool, error)
	load   func(context.Context, string, string) error
}

func (r imageResolver) resolve(ctx context.Context, reference string) (resolved string, err error) {
	output, err := r.docker(ctx, "image", "inspect", "--format", "{{.Id}}", reference)
	if err != nil {
		// Do not turn daemon, context, or permission errors into registry pulls.
		if ctx.Err() != nil || !strings.Contains(string(output), "No such image:") {
			return "", dockerImageError("inspect", reference, output, err)
		}
		logging.FromContext(ctx).Info("Pulling Docker image", "image", reference)
		if output, err := r.docker(ctx, "pull", reference); err != nil {
			return "", dockerImageError("pull", reference, output, err)
		}
		output, err = r.docker(ctx, "image", "inspect", "--format", "{{.Id}}", reference)
		if err != nil {
			return "", dockerImageError("inspect", reference, output, err)
		}
	}
	id := strings.TrimSpace(string(output))
	hash := strings.TrimPrefix(id, "sha256:")
	if len(hash) != 64 || strings.Trim(hash, "0123456789abcdef") != "" || id == hash {
		return "", fmt.Errorf("inspect Docker image %s: invalid image ID %q", reference, id)
	}
	resolved = "opencode-sandbox-cache:sha256-" + hash
	cached, err := r.cached(ctx, resolved)
	if err != nil {
		return "", fmt.Errorf("find imported image %s: %w", resolved, err)
	}
	if cached {
		return resolved, nil
	}
	archive, err := os.CreateTemp("", "opencode-sandbox-image-*.tar")
	if err != nil {
		return "", fmt.Errorf("create image archive: %w", err)
	}
	defer func() { err = errors.Join(err, os.Remove(archive.Name())) }()
	if err := archive.Close(); err != nil {
		return "", fmt.Errorf("close image archive: %w", err)
	}
	logging.FromContext(ctx).Info("Importing Docker image into Microsandbox", "image", reference, "id", id)
	// Save by immutable ID, not a tag that could move after inspection.
	if output, err := r.docker(ctx, "image", "save", "--output", archive.Name(), id); err != nil {
		return "", dockerImageError("save", reference, output, err)
	}
	if err := r.load(ctx, archive.Name(), resolved); err != nil {
		return "", fmt.Errorf("import Docker image %s: %w", reference, err)
	}
	return resolved, nil
}

func dockerImageError(operation, reference string, output []byte, err error) error {
	return fmt.Errorf("docker %s image %s: %w: %s", operation, reference, err, strings.TrimSpace(string(output)))
}
