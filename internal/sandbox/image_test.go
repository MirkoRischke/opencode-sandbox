package sandbox

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveImage(t *testing.T) {
	t.Parallel()
	id := "sha256:" + strings.Repeat("a", 64)
	internal := "opencode-sandbox-cache:sha256-" + strings.Repeat("a", 64)
	failure := errors.New("command failed")
	for _, tt := range []struct {
		name         string
		missing      bool
		cached       bool
		inspectError string
		pullError    bool
		saveError    bool
		cacheError   bool
		loadError    bool
		invalidID    bool
		wantError    string
	}{
		{name: "local image imported"},
		{name: "cached image reused", cached: true},
		{name: "missing image pulled", missing: true},
		{name: "pulled image already imported", missing: true, cached: true},
		{name: "daemon error does not pull", inspectError: "Cannot connect to the Docker daemon", wantError: "inspect"},
		{name: "pull failure", missing: true, pullError: true, wantError: "pull"},
		{name: "cache lookup failure", cacheError: true, wantError: "find imported image"},
		{name: "save failure cleans archive", saveError: true, wantError: "save"},
		{name: "import failure cleans archive", loadError: true, wantError: "import Docker image"},
		{name: "invalid image ID", invalidID: true, wantError: "invalid image ID"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var commands [][]string
			var archive string
			var loaded bool
			r := imageResolver{
				docker: func(_ context.Context, args ...string) ([]byte, error) {
					commands = append(commands, args)
					switch {
					case args[0] == "pull":
						require.True(t, tt.missing, "local images must never be pulled")
						require.Equal(t, []string{"pull", "example:test"}, args)
						if tt.pullError {
							return []byte("access denied"), failure
						}
					case args[1] == "inspect":
						require.Equal(t, []string{"image", "inspect", "--format", "{{.Id}}", "example:test"}, args)
						if tt.inspectError != "" {
							return []byte(tt.inspectError), failure
						}
						if tt.missing && len(commands) == 1 {
							return []byte("Error response from daemon: No such image: example:test"), failure
						}
						if tt.invalidID {
							return []byte("not-an-id"), nil
						}
						return []byte(id + "\n"), nil
					case args[1] == "save":
						require.Equal(t, []string{"image", "save", "--output"}, args[:3])
						require.Equal(t, id, args[4], "export the resolved ID, not a mutable tag")
						archive = args[3]
						require.FileExists(t, archive)
						if tt.saveError {
							return []byte("save failed"), failure
						}
						require.NoError(t, os.WriteFile(archive, []byte("archive"), 0600))
					default:
						t.Fatalf("unexpected docker command: %v", args)
					}
					return nil, nil
				},
				cached: func(_ context.Context, ref string) (bool, error) {
					require.Equal(t, internal, ref)
					if tt.cacheError {
						return false, failure
					}
					return tt.cached, nil
				},
				load: func(_ context.Context, path, ref string) error {
					loaded = true
					require.Equal(t, archive, path)
					require.Equal(t, internal, ref)
					contents, err := os.ReadFile(path)
					require.NoError(t, err)
					require.Equal(t, "archive", string(contents))
					if tt.loadError {
						return failure
					}
					return nil
				},
			}
			resolved, err := r.resolve(context.Background(), "example:test")
			if tt.wantError != "" {
				require.ErrorContains(t, err, tt.wantError)
				if !tt.invalidID {
					require.ErrorIs(t, err, failure)
				}
			} else {
				require.NoError(t, err)
				require.Equal(t, internal, resolved)
				require.Equal(t, !tt.cached, loaded)
			}
			if archive != "" {
				require.NoFileExists(t, archive)
			}
		})
	}
}

func TestResolveImageRebuiltTag(t *testing.T) {
	t.Parallel()
	ids := []string{"sha256:" + strings.Repeat("a", 64), "sha256:" + strings.Repeat("b", 64)}
	imports := map[string]bool{}
	var current, saves int
	r := imageResolver{
		docker: func(_ context.Context, args ...string) ([]byte, error) {
			if args[1] == "inspect" {
				return []byte(ids[current]), nil
			}
			require.Equal(t, "save", args[1])
			saves++
			return nil, nil
		},
		cached: func(_ context.Context, ref string) (bool, error) { return imports[ref], nil },
		load: func(_ context.Context, _, ref string) error {
			imports[ref] = true
			return nil
		},
	}
	first, err := r.resolve(context.Background(), "example:dev")
	require.NoError(t, err)
	again, err := r.resolve(context.Background(), "example:dev")
	require.NoError(t, err)
	require.Equal(t, first, again)
	require.Equal(t, 1, saves)
	current = 1
	rebuilt, err := r.resolve(context.Background(), "example:dev")
	require.NoError(t, err)
	require.NotEqual(t, first, rebuilt)
	require.Equal(t, 2, saves)
	delete(imports, rebuilt) // An externally cleared Microsandbox cache is repopulated.
	_, err = r.resolve(context.Background(), "example:dev")
	require.NoError(t, err)
	require.Equal(t, 3, saves)
}
