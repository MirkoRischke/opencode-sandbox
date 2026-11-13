// Package gateway manages the shared host LiteLLM Compose project.
package gateway

import (
	"context"
	"crypto/rand"
	"embed"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

//go:embed compose.yaml config.yaml
var assets embed.FS

type Gateway struct {
	directory   string
	projectName string
	port        uint16
	baseURL     string
	client      *http.Client
}

// Options isolates a gateway's Compose resources and host listener. Zero values
// retain the shared project name and port 4000.
type Options struct {
	ProjectName string
	Port        uint16
}

// New uses the XDG data root, independently of the current source checkout.
func New(dataDir string) *Gateway {
	return NewWithOptions(dataDir, Options{})
}

// NewWithOptions configures an independent Compose project and gateway API port.
func NewWithOptions(dataDir string, options Options) *Gateway {
	if options.ProjectName == "" {
		options.ProjectName = "opencode-sandbox-gateway"
	}
	if options.Port == 0 {
		options.Port = 4000
	}
	return &Gateway{
		directory:   filepath.Join(dataDir, "opencode-sandbox", "gateway"),
		projectName: options.ProjectName,
		port:        options.Port,
		baseURL:     fmt.Sprintf("http://127.0.0.1:%d", options.Port),
		client:      &http.Client{Timeout: 15 * time.Second},
	}
}

func (g *Gateway) Directory() string { return g.directory }

// Ensure serializes initialization/startup across launchers. Compose health checks
// establish readiness of our project, not merely an unrelated listener on :4000.
func (g *Gateway) Ensure(ctx context.Context, output io.Writer) error {
	if err := os.MkdirAll(g.directory, 0700); err != nil {
		return fmt.Errorf("create gateway directory: %w", err)
	}
	lock, err := os.OpenFile(filepath.Join(g.directory, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("open gateway lock: %w", err)
	}
	defer lock.Close() //nolint:errcheck // Closing releases the advisory lock.
	if err := acquire(ctx, lock); err != nil {
		return err
	}
	if err := g.materialize(); err != nil {
		return err
	}
	// Reconcile even healthy containers so bundled Compose updates take effect.
	if err := g.compose(ctx, nil, output, output, "up", "--wait", "--wait-timeout", "180"); err != nil {
		return fmt.Errorf("start LLM gateway: %w", err)
	}
	return nil
}

// Stop halts the shared containers without removing credentials or data. It uses
// the startup lock so a concurrent Ensure cannot race the Compose operation.
func (g *Gateway) Stop(ctx context.Context, output io.Writer) error {
	lock, err := os.OpenFile(filepath.Join(g.directory, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if errors.Is(err, os.ErrNotExist) {
		return nil // No installation exists yet; don't create one just to stop it.
	}
	if err != nil {
		return fmt.Errorf("open gateway lock: %w", err)
	}
	defer lock.Close() //nolint:errcheck // Closing releases the advisory lock.
	if err := acquire(ctx, lock); err != nil {
		return err
	}
	if err := g.compose(ctx, nil, output, output, "stop"); err != nil {
		return fmt.Errorf("stop LLM gateway: %w", err)
	}
	return nil
}

// Authenticate runs a human-driven device login against the persistent auth volume.
// Access tokens are deliberately not printed by the Python expression.
func (g *Gateway) Authenticate(ctx context.Context, provider string, input io.Reader, output, stderr io.Writer) error {
	var command string
	switch provider {
	case "chatgpt":
		command = "from litellm.llms.chatgpt.authenticator import Authenticator; Authenticator().get_access_token()"
	case "copilot":
		command = "from litellm.llms.github_copilot.authenticator import Authenticator; Authenticator().get_api_key()"
	default:
		return fmt.Errorf("unknown gateway provider %q; choose chatgpt or copilot", provider)
	}
	if err := g.Ensure(ctx, stderr); err != nil {
		return err
	}
	if err := g.compose(ctx, input, output, stderr, "exec", "-T", "litellm", "python", "-u", "-c", command); err != nil {
		return fmt.Errorf("authenticate gateway with %s: %w", provider, err)
	}
	return nil
}

func (g *Gateway) compose(ctx context.Context, input io.Reader, output, stderr io.Writer, args ...string) error {
	command := exec.CommandContext(ctx, "docker", append([]string{
		"compose", "--project-directory", g.directory, "--project-name", g.projectName,
		"--file", filepath.Join(g.directory, "compose.yaml"), "--env-file", filepath.Join(g.directory, ".env"),
	}, args...)...)
	command.Dir = g.directory
	command.Env = append(os.Environ(), fmt.Sprintf("OPENCODE_SANDBOX_GATEWAY_PORT=%d", g.port))
	command.Stdin, command.Stdout, command.Stderr = input, output, stderr
	return command.Run()
}

func acquire(ctx context.Context, file *os.File) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			return fmt.Errorf("lock gateway startup: %w", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (g *Gateway) materialize() error {
	for _, name := range []string{"compose.yaml", "config.yaml"} {
		data, err := assets.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read bundled gateway %s: %w", name, err)
		}
		// Compose is managed by the installed CLI; preserve user configuration.
		if name == "compose.yaml" {
			if err := os.WriteFile(filepath.Join(g.directory, name), data, 0600); err != nil {
				return fmt.Errorf("write gateway %s: %w", name, err)
			}
			continue
		}
		if err := writeOnce(filepath.Join(g.directory, name), data); err != nil {
			return err
		}
	}
	path := filepath.Join(g.directory, ".env")
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect gateway environment: %w", err)
	}
	var env strings.Builder
	for _, name := range []string{"LITELLM_MASTER_KEY", "LITELLM_SALT_KEY", "POSTGRES_PASSWORD"} {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return fmt.Errorf("generate gateway secret: %w", err)
		}
		fmt.Fprintf(&env, "%s=sk-%x\n", name, key)
	}
	return writeOnce(path, []byte(env.String()))
}

func writeOnce(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("create gateway file: %w", err)
	}
	_, writeErr := file.Write(data)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return fmt.Errorf("write gateway file: %w", err)
	}
	return nil
}
