package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"time"
)

// EnsureDocker starts the guest's Docker daemon when installed. Configuration
// belongs to the image; no custom guest script, init system, or host socket is
// required. The daemon outlives this exec session, but not the VM.
func (s *Sandbox) EnsureDocker(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	var output bytes.Buffer
	if err := s.Exec(ctx, nil, &output, &output, "bash", "-eu", "-c", ensureDockerScript); err != nil {
		return fmt.Errorf("initialize guest Docker: %w: %s", err, output.String())
	}
	return nil
}

const ensureDockerScript = `
command -v dockerd >/dev/null 2>&1 || exit 0
mkdir -p /run/opencode-sandbox
exec 9>/run/opencode-sandbox/docker.lock
flock -w 10 9
ready() {
  timeout 2 docker --host unix:///var/run/docker.sock info >/dev/null 2>&1
}
ready && exit 0
# Microsandbox's layered root preserves /run across cold boots. PID files from
# our previous boot can point to unrelated, reused PIDs and prevent both Docker
# and its managed containerd from starting. Never clear them on a same-boot
# reconnect, and never remove any persistent Docker data.
boot_id=$(cat /proc/sys/kernel/random/boot_id)
boot_file=/run/opencode-sandbox/docker.boot-id
if [ -f "$boot_file" ] && [ "$(cat "$boot_file")" != "$boot_id" ]; then
  rm -f /var/run/docker.pid /var/run/docker/containerd/containerd.pid
fi
printf '%s\n' "$boot_id" > "$boot_file"
mkdir -p /var/log/opencode-sandbox
log=/var/log/opencode-sandbox/dockerd.log
# A new session and closed exec pipes/lock keep the daemon independent of the
# SDK command's process group, transport, and timeout. Never truncate its log.
setsid -f dockerd </dev/null >>"$log" 2>&1 9>&-
deadline=$((SECONDS + 60))
until ready; do
  if (( SECONDS >= deadline )); then
    echo 'Docker did not become ready within 60 seconds. Recent daemon logs:' >&2
    tail -n 80 "$log" >&2
    exit 1
  fi
  sleep 1
done
`
