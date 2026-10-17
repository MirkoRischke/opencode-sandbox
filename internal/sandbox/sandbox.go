package sandbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	m "github.com/superradcompany/microsandbox/sdk/go"
)

type Sandbox struct {
	Name     string
	Worktree string
	vm       *m.Sandbox
}

// Prepare creates or resumes one VM per worktree and initializes project tools.
// GitHub credentials stay in the host proxy; the guest gets only a placeholder.
func Prepare(ctx context.Context, directory string, config, githubToken []byte, settings Settings) (*Sandbox, error) {
	if err := settings.Network.validate(); err != nil {
		return nil, err
	}
	git := func(args ...string) (string, error) {
		out, err := exec.CommandContext(ctx, "git", append([]string{"-C", directory}, args...)...).CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("discover worktree: %w: %s", err, out)
		}
		return strings.TrimSpace(string(out)), nil
	}
	root, err := git("rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	common, err := git("rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return nil, err
	}
	common, err = filepath.EvalSymlinks(common)
	if err != nil {
		return nil, err
	}
	s := &Sandbox{Name: sandboxName(root, common), Worktree: root}
	if err := configureRuntime(); err != nil {
		return nil, fmt.Errorf("locate Microsandbox runtime: %w", err)
	}
	owner := m.MountOptions{Owner: &m.MountOwner{UID: 0, GID: 0}}
	mounts := map[string]m.MountConfig{root: m.Mount.Bind(root, owner)}
	// A regular checkout already includes its .git directory in the root mount.
	if !strings.HasPrefix(common, root+string(filepath.Separator)) {
		mounts[common] = m.Mount.Bind(common, owner)
	}
	opts := []m.SandboxOption{
		m.WithImage("opencode-sandbox:poc"), m.WithPullPolicy(m.PullPolicyNever),
		m.WithCPUs(2), m.WithMemory(2048), m.WithRootDisk(m.RootDisk.Managed(4096)),
		m.WithWorkdir(root), m.WithMounts(mounts), m.WithDetached(),
		m.WithNetwork(settings.Network.policy()), m.WithInterceptTLS(),
		m.WithLabel("app", "opencode-sandbox"),
	}
	s.vm, err = connectWithGitHub(ctx, s.Name, githubToken, opts)
	if err != nil {
		return nil, fmt.Errorf("prepare sandbox: %w", err)
	}
	initialized := false
	defer func() {
		if !initialized {
			if err := s.Shutdown(); err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
		}
	}()
	if err := s.configureGitHub(ctx, githubToken != nil); err != nil {
		return nil, err
	}
	// mise discovers supported filenames (including .mise.toml and mise/config.toml).
	// Ignore the image's global config when deciding whether this project uses mise.
	// Trust first: even config discovery parses trust-sensitive environment settings.
	err = s.Exec(ctx, nil, os.Stdout, os.Stderr, "bash", "-eu", "-c", `
mise trust --all
configs=$(mise config ls --no-header --no-truncate)
if printf '%s\n' "$configs" | grep -F -- "$PWD/" >/dev/null; then
  mise install
fi`)
	if err != nil {
		return nil, fmt.Errorf("initialize project tools: %w", err)
	}
	if err := s.configureOpenCode(ctx, config); err != nil {
		return nil, fmt.Errorf("configure guest OpenCode: %w", err)
	}
	initialized = true
	return s, nil
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
	defer h.Close()
	var inputDone chan error
	if stdin != nil {
		sink := h.TakeStdin()
		if sink == nil {
			return fmt.Errorf("guest stdin unavailable")
		}
		defer sink.Close()
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
	terminalEnv := make(map[string]string)
	for _, key := range []string{"TERM", "COLORTERM"} {
		if value := os.Getenv(key); value != "" {
			terminalEnv[key] = value
		}
	}
	code, err := s.vm.AttachWith(ctx, "opencode", nil,
		m.WithAttachCwd(s.Worktree), m.WithAttachEnv(terminalEnv))
	if err != nil {
		return err
	}
	return checkExit(code)
}

func (s *Sandbox) Remove(ctx context.Context) error {
	defer s.Close()
	return s.vm.Destroy(ctx)
}

// Close releases the SDK connection without stopping this persistent VM.
func (s *Sandbox) Close() error {
	return s.vm.Close()
}

func (s *Sandbox) Stop(ctx context.Context) error {
	return s.vm.Stop(ctx)
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
