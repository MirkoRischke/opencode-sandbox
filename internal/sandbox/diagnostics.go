package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"opencode-sandbox/internal/logging"
	"opencode-sandbox/internal/workspace"

	m "github.com/superradcompany/microsandbox/sdk/go"
)

// Diagnostics are host-owned cumulative state, independent of VM storage.
type networkOverview struct {
	SchemaVersion int            `json:"schema_version"`
	Worktree      string         `json:"worktree"`
	UpdatedAt     time.Time      `json:"updated_at"`
	Overflow      bool           `json:"overflow,omitempty"`
	DeniedTargets []deniedTarget `json:"denied_targets"`
}

type deniedTarget struct {
	Hostname     string    `json:"hostname,omitempty"`
	IP           string    `json:"ip,omitempty"`
	Port         uint16    `json:"port,omitempty"`
	Protocol     string    `json:"protocol,omitempty"`
	Reason       string    `json:"reason,omitempty"`
	Observations uint64    `json:"observations"`
	FirstSeen    time.Time `json:"first_seen"`
	LastSeen     time.Time `json:"last_seen"`
}

type networkDiagnostics struct {
	vm                *m.Sandbox
	path              string
	overview          networkOverview
	since             time.Time
	cursor            string
	dirty             bool
	stop              chan struct{}
	done              chan struct{}
	once              sync.Once
	reportOnce        sync.Once
	collectionFailure error
	saveFailure       error
	logger            *slog.Logger
}

// DiagnosticsOptions supplies host-owned state explicitly. Logging uses the
// invocation logger from context, independently of diagnostic storage.
type DiagnosticsOptions struct {
	StateDir string
}

func resolveDiagnostics(enabled bool, options []DiagnosticsOptions) (DiagnosticsOptions, error) {
	if !enabled {
		return DiagnosticsOptions{}, nil
	}
	if len(options) != 1 || options[0].StateDir == "" {
		return DiagnosticsOptions{}, fmt.Errorf("network diagnostics require an explicit host state directory")
	}
	return options[0], nil
}

// WriteNetworkDiagnostics prints the worktree's saved JSON without starting a VM
// or loading configuration. The saved bytes are preserved unchanged.
func WriteNetworkDiagnostics(ctx context.Context, directory, stateDir string, w io.Writer) error {
	wspace, err := workspace.Resolve(ctx, directory)
	if err != nil {
		return err
	}
	root := wspace.Root()
	path := diagnosticsPath(stateDir, root)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("no saved network diagnostics for %s; enable network.diagnostics and launch the sandbox to collect them: %w", root, err)
	}
	if err != nil {
		return fmt.Errorf("read network diagnostics: %w", err)
	}
	_, err = w.Write(data)
	return err
}

func diagnosticsPath(stateDir, worktree string) string {
	identity := sha256.Sum256([]byte(worktree))
	return filepath.Join(stateDir, "opencode-sandbox", "diagnostics", fmt.Sprintf("%x", identity[:16]), "network.json")
}

func loadDiagnostics(worktree string, since time.Time, options DiagnosticsOptions) (*networkDiagnostics, error) {
	path := diagnosticsPath(options.StateDir, worktree)
	d := &networkDiagnostics{
		path: path, since: since,
		overview: networkOverview{SchemaVersion: 1, Worktree: worktree, DeniedTargets: []deniedTarget{}},
		stop:     make(chan struct{}), done: make(chan struct{}),
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		d.dirty = true
		return d, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &d.overview); err != nil {
		return nil, err
	}
	if d.overview.SchemaVersion != 1 || d.overview.Worktree != worktree {
		return nil, fmt.Errorf("unsupported or mismatched network overview")
	}
	return d, nil
}

func startNetworkDiagnostics(ctx context.Context, vm *m.Sandbox, worktree string, since time.Time, options DiagnosticsOptions) *networkDiagnostics {
	logger := logging.FromContext(ctx).With("component", "network_diagnostics")
	d, err := loadDiagnostics(worktree, since, options)
	if err != nil {
		logger.Warn("Network diagnostics unavailable; existing history was not changed", "error", err)
		return nil
	}
	d.vm = vm
	d.logger = logger
	logger.Info("Network diagnostics started", "path", d.path)
	go d.run(d.collect)
	return d
}

func (d *networkDiagnostics) run(collect func() error) {
	defer close(d.done)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		// Snapshot streams terminate at EOF. Polling permits periodic saves and
		// a final drain after VM stop without cancelling a live follower.
		d.poll(collect)
		select {
		case <-d.stop:
			// Stop may have happened during the previous snapshot.
			d.poll(collect)
			return
		case <-ticker.C:
		}
	}
}

func (d *networkDiagnostics) poll(collect func() error) {
	d.collectionFailure = collect()
	if err := d.save(); err != nil && d.saveFailure == nil {
		d.saveFailure = err
	}
}

func (d *networkDiagnostics) collect() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	opts := m.LogStreamOptions{Sources: []m.LogSource{m.LogSourceSystem}}
	if d.cursor != "" {
		opts.FromCursor = d.cursor
	} else {
		// Never replay a previous launcher run's records into cumulative counts.
		opts.Since = d.since
	}
	stream, err := d.vm.LogStream(ctx, opts)
	if err != nil {
		return err
	}
	defer stream.Close() //nolint:errcheck // Release the snapshot; collection errors are reported separately.
	for {
		entry, err := stream.Recv(ctx)
		if err != nil || entry == nil {
			return err
		}
		d.observe(*entry)
		d.cursor = entry.Cursor
	}
}

func (d *networkDiagnostics) finish() {
	if d == nil {
		return
	}
	d.once.Do(func() { close(d.stop) })
	<-d.done
	// The caller has returned from the attached terminal before Stop/Close.
	// Never let a collector goroutine write into OpenCode's active TUI.
	d.reportOnce.Do(func() {
		if d.logger == nil {
			return
		}
		if d.collectionFailure != nil {
			d.logger.Warn("Network diagnostics incomplete: collection failed", "error", d.collectionFailure, "summary", d.path)
		}
		if d.saveFailure != nil {
			d.logger.Warn("Network diagnostics incomplete: save failed", "error", d.saveFailure, "summary", d.path)
		}
	})
}

func (d *networkDiagnostics) observe(entry m.LogEntry) {
	target, ok := parseNetworkDenial(entry)
	if !ok {
		return
	}
	for i := range d.overview.DeniedTargets {
		existing := &d.overview.DeniedTargets[i]
		if existing.Hostname != target.Hostname || existing.IP != target.IP || existing.Port != target.Port ||
			existing.Protocol != target.Protocol || existing.Reason != target.Reason {
			continue
		}
		existing.Observations++
		if target.FirstSeen.Before(existing.FirstSeen) {
			existing.FirstSeen = target.FirstSeen
		}
		if target.LastSeen.After(existing.LastSeen) {
			existing.LastSeen = target.LastSeen
		}
		d.dirty = true
		return
	}
	// A guest can generate arbitrary destinations; bound host summary size.
	if len(d.overview.DeniedTargets) >= 4096 {
		d.overview.Overflow = true
	} else {
		d.overview.DeniedTargets = append(d.overview.DeniedTargets, target)
	}
	d.dirty = true
}

func (d *networkDiagnostics) save() error {
	if !d.dirty {
		return nil
	}
	d.overview.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(d.overview, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(d.path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(d.path), ".network-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name()) //nolint:errcheck // Best-effort cleanup if the atomic replacement fails.
	_, writeErr := file.Write(append(data, '\n'))
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(file.Name(), d.path); err != nil {
		return err
	}
	d.dirty = false
	return nil
}

// This intentionally recognizes only v0.7.8 network-policy diagnostics, not
// arbitrary errors, secret violations, ingress events, or guest output.
var denialMessages = []struct{ message, protocol, reason string }{
	{"DNS query denied by network policy", "dns", "domain_policy"},
	{"TCP egress denied by strict hostname policy", "tcp", "strict_hostname_policy"},
	{"TCP egress denied by domain policy", "tcp", "domain_policy"},
	{"TLS egress denied by domain policy", "tcp", "domain_policy"},
	{"TLS egress denied by default policy", "tcp", "default_policy"},
	{"TLS bypass denied by strict hostname policy", "tcp", "strict_hostname_policy"},
	{"CONNECT tunnel denied by strict hostname policy", "tcp", "strict_hostname_policy"},
	{"ICMP echo denied by policy", "icmp", "network_policy"},
	{"ICMPv6 echo denied by policy", "icmpv6", "network_policy"},
}

var diagnosticField = regexp.MustCompile(`\b(domain|sni|dst)=(Some\("[^"\s]*"\)|"[^"\s]*"|[^\s]+)`)

func parseNetworkDenial(entry m.LogEntry) (deniedTarget, bool) {
	text := entry.Text()
	if entry.Source != m.LogSourceSystem || !strings.Contains(text, "microsandbox_network::") {
		return deniedTarget{}, false
	}
	var target deniedTarget
	for _, kind := range denialMessages {
		if strings.Contains(text, kind.message) {
			target.Protocol, target.Reason = kind.protocol, kind.reason
			break
		}
	}
	if target.Protocol == "" {
		return target, false
	}
	for _, match := range diagnosticField.FindAllStringSubmatch(text, -1) {
		target.parseField(match[1], match[2])
	}
	if target.Hostname != "" {
		// IP rotation must not split a hostname's cumulative history.
		target.IP = ""
	}
	if target.Hostname == "" && target.IP == "" {
		return target, false
	}
	target.Observations, target.FirstSeen, target.LastSeen = 1, entry.Timestamp.UTC(), entry.Timestamp.UTC()
	return target, true
}

func (target *deniedTarget) parseField(field, raw string) {
	value := strings.TrimSuffix(strings.TrimPrefix(raw, "Some("), ")")
	value = strings.Trim(value, `"`)
	switch field {
	case "domain", "sni":
		value = strings.ToLower(strings.TrimSuffix(value, "."))
		if len(value) <= 253 && hostnameForDiagnostics(value) {
			target.Hostname = value
		}
	case "dst":
		if host, port, err := net.SplitHostPort(value); err == nil && net.ParseIP(host) != nil {
			n, err := strconv.ParseUint(port, 10, 16)
			if err == nil {
				target.IP, target.Port = host, uint16(n)
			}
		} else if net.ParseIP(value) != nil {
			target.IP = value
		}
	}
}

var diagnosticHostname = regexp.MustCompile(`^[a-z0-9._-]+$`)

func hostnameForDiagnostics(host string) bool {
	return host != "none" && diagnosticHostname.MatchString(host)
}
