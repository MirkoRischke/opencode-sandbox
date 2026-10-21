package sandbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	launcherconfig "opencode-sandbox/internal/config"
	"opencode-sandbox/internal/logging"
	"opencode-sandbox/internal/workspace"

	m "github.com/superradcompany/microsandbox/sdk/go"
)

type Sandbox struct {
	Name        string
	Worktree    string
	vm          *m.Sandbox
	diagnostics *networkDiagnostics
	logger      *slog.Logger
}

func (s *Sandbox) log() *slog.Logger {
	if s.logger != nil {
		return s.logger
	}
	return logging.FromContext(context.Background())
}

// Prepare resolves an exact directory and prepares its workspace VM.
// GitHub credentials stay in the host proxy; the guest gets only a placeholder.
// When diagnostics are enabled, supply exactly one host diagnostics configuration.
func Prepare(ctx context.Context, directory string, secrets []Secret, settings launcherconfig.Settings, diagnostics ...DiagnosticsOptions) (*Sandbox, error) {
	w, err := workspace.Resolve(ctx, directory)
	if err != nil {
		return nil, err
	}
	return PrepareWorkspace(ctx, w, secrets, settings, diagnostics...)
}

// PrepareWorkspace creates or resumes one VM for an already resolved workspace.
func PrepareWorkspace(ctx context.Context, w workspace.Workspace, secrets []Secret, settings launcherconfig.Settings, diagnostics ...DiagnosticsOptions) (*Sandbox, error) {
	if err := settings.Validate(); err != nil {
		return nil, err
	}
	if err := settings.ValidateFileSources(); err != nil {
		return nil, err
	}
	diagnosticOptions, err := resolveDiagnostics(settings.Network.Diagnostics, diagnostics)
	if err != nil {
		return nil, err
	}
	if err := validateMounts(w, settings); err != nil {
		return nil, err
	}
	if err := configureRuntime(); err != nil {
		return nil, fmt.Errorf("locate Microsandbox runtime: %w", err)
	}
	s := &Sandbox{Name: sandboxName(w), Worktree: w.Root(), logger: logging.FromContext(ctx).With("vm", sandboxName(w))}
	s.log().Info("Preparing VM")
	started := time.Now().UTC()
	var created bool
	s.vm, created, err = connectOrRestore(ctx, s.Name, w, secrets, settings, sandboxOptions(w, w.Root(), settings))
	if err != nil {
		s.log().Error("Prepare VM failed", "error", err)
		return nil, fmt.Errorf("prepare sandbox: %w", err)
	}
	s.log().Info("VM connected", "created", created, "duration_ms", time.Since(started).Milliseconds())
	if settings.Network.Diagnostics {
		s.diagnostics = startNetworkDiagnostics(ctx, s.vm, w.Root(), started, diagnosticOptions)
	}
	if created {
		if err := s.copyFiles(ctx, settings); err != nil {
			cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			return nil, errors.Join(err, s.Remove(cleanup))
		}
		if err := s.configureEnvironment(ctx, settings); err != nil {
			cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			return nil, errors.Join(err, s.Remove(cleanup))
		}
	}
	if err := s.initializeProject(ctx, secrets); err != nil {
		return nil, errors.Join(err, s.Shutdown())
	}
	s.log().Info("VM ready", "duration_ms", time.Since(started).Milliseconds())
	return s, nil
}

func sandboxOptions(w workspace.Workspace, guestRoot string, settings launcherconfig.Settings) []m.SandboxOption {
	mounts := workspaceMounts(w, guestRoot)
	mounts["/var/lib/docker"] = m.Mount.Owned(m.OwnedVolumeOptions{
		Kind: m.VolumeKindDisk, SizeMiB: settings.VM.DockerDiskMiB,
	})
	addReadOnlyMounts(mounts, settings)
	opts := []m.SandboxOption{
		m.WithImage(settings.VM.Image), m.WithPullPolicy(m.PullPolicyNever),
		m.WithCPUs(settings.VM.CPUs), m.WithMemory(settings.VM.MemoryMiB),
		m.WithRootDisk(m.RootDisk.Managed(settings.VM.RootDiskMiB)),
		m.WithWorkdir(guestRoot), m.WithMounts(mounts), m.WithDetached(),
		m.WithNetwork(networkPolicy(settings.Network)), m.WithInterceptTLS(),
		m.WithLabel("opencode-sandbox", ""),
	}
	if settings.Network.Diagnostics {
		opts = append(opts, m.WithLogLevel(m.LogLevelDebug))
	}
	return opts
}

// initializeProject is shared by worktree launches and fresh preparation VMs.
// Session-specific OpenCode configuration/authentication happens only on launch.
func (s *Sandbox) initializeProject(ctx context.Context, secrets []Secret) error {
	if err := s.EnsureDocker(ctx); err != nil {
		return err
	}
	githubEnabled := false
	for _, secret := range secrets {
		githubEnabled = githubEnabled || secret.entry.EnvVar == "GH_TOKEN"
	}
	if err := s.configureGitHub(ctx, githubEnabled); err != nil {
		return err
	}
	// mise discovers supported filenames (including .mise.toml and mise/config.toml).
	// Ignore the image's global config when deciding whether this project uses mise.
	// Trust first: even config discovery parses trust-sensitive environment settings.
	err := s.Exec(ctx, nil, os.Stdout, os.Stderr, "bash", "-eu", "-c", `
mise trust --all
configs=$(mise config ls --no-header --no-truncate)
if printf '%s\n' "$configs" | grep -F -- "$PWD/" >/dev/null; then
  mise install
fi`)
	if err != nil {
		return fmt.Errorf("initialize project tools: %w", err)
	}
	return nil
}

func (s *Sandbox) Exec(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, command ...string) error {
	if len(command) == 0 {
		return fmt.Errorf("missing guest command")
	}
	opts := []m.ExecOption{m.WithExecCwd(s.Worktree)}
	if deadline, ok := ctx.Deadline(); ok {
		opts = append(opts, m.WithExecTimeout(time.Until(deadline)))
	}
	if stdin != nil {
		opts = append(opts, m.WithExecStdinPipe())
	}
	h, err := s.vm.ExecStream(ctx, command[0], command[1:], opts...)
	if err != nil {
		return err
	}
	defer h.Close() //nolint:errcheck // Release the stream after completion or failure; preserve the command result.
	var inputDone chan error
	if stdin != nil {
		sink := h.TakeStdin()
		if sink == nil {
			return fmt.Errorf("guest stdin unavailable")
		}
		defer sink.Close() //nolint:errcheck // Fallback cleanup; the input goroutine checks its own Close error.
		inputDone = make(chan error, 1)
		go func() {
			_, err := io.Copy(sink, stdin)
			closeErr := sink.Close()
			if err == nil {
				err = closeErr
			}
			inputDone <- err
		}()
	}
	exitCode := -1
	for {
		event, err := h.Recv(ctx)
		if err != nil {
			return err
		}
		switch event.Kind {
		case m.ExecEventStdout:
			if stdout != nil {
				if _, err := stdout.Write(event.Data); err != nil {
					return err
				}
			}
		case m.ExecEventStderr:
			if stderr != nil {
				if _, err := stderr.Write(event.Data); err != nil {
					return err
				}
			}
		case m.ExecEventExited:
			exitCode = event.ExitCode
		case m.ExecEventFailed, m.ExecEventStdinError:
			return fmt.Errorf("guest command failed (details suppressed)")
		case m.ExecEventDone:
			if inputDone != nil {
				select {
				case err := <-inputDone:
					if err != nil {
						return fmt.Errorf("write guest stdin failed")
					}
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return checkExit(exitCode)
		}
	}
}

func (s *Sandbox) Open(ctx context.Context) error {
	s.log().Info("Attaching guest terminal")
	terminalEnv := make(map[string]string)
	for _, key := range []string{"TERM", "COLORTERM"} {
		if value := os.Getenv(key); value != "" {
			terminalEnv[key] = value
		}
	}
	// Keep the terminal attached until the guest shell exits, not just OpenCode.
	code, err := s.vm.AttachWith(ctx, "bash", []string{"-c", "opencode; exec bash -i"},
		m.WithAttachCwd(s.Worktree), m.WithAttachEnv(terminalEnv))
	s.log().Info("Guest terminal detached", "exit_code", code, "error", err)
	if err != nil {
		return err
	}
	return checkExit(code)
}

func (s *Sandbox) Remove(ctx context.Context) (err error) {
	s.log().Info("Removing VM")
	defer func() { s.log().Info("VM removal finished", "error", err) }()
	defer func() { err = errors.Join(err, s.Close()) }()
	if s.diagnostics != nil {
		if err := s.Stop(ctx); err != nil {
			return err
		}
	}
	return s.vm.Destroy(ctx)
}

// Close releases the SDK connection without stopping this persistent VM.
func (s *Sandbox) Close() error {
	s.diagnostics.finish()
	return s.vm.Close()
}

func (s *Sandbox) Stop(ctx context.Context) error {
	s.log().Info("Stopping VM")
	err := s.vm.Stop(ctx)
	s.diagnostics.finish()
	s.log().Info("VM stop finished", "error", err)
	return err
}

// Shutdown stops the VM without deleting its disk, then releases the connection.
// Cleanup uses an independent deadline, even when the launch context was canceled.
func (s *Sandbox) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stopErr := s.Stop(ctx)
	if stopErr != nil {
		stopErr = fmt.Errorf("stop sandbox: %w", stopErr)
	}
	return errors.Join(stopErr, s.Close())
}

func checkExit(code int) error {
	if code != 0 {
		return fmt.Errorf("guest command exited with code %d", code)
	}
	return nil
}
