// SPDX-License-Identifier: Apache-2.0

package publish

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/netutil"
)

// ExternalProcessStatus is a coarse-grained health label.
type ExternalProcessStatus string

const (
	StatusStopped  ExternalProcessStatus = "stopped"
	StatusStarting ExternalProcessStatus = "starting"
	StatusRunning  ExternalProcessStatus = "running"
	StatusFailed   ExternalProcessStatus = "failed"
)

// ExternalProcess wraps a single supervised external process.
//
// Lifecycle (single-shot per spawn — restart logic is on top via the
// supervisor watchdog):
//
//	Start → status: starting → healthz probe → status: running
//	Crash → cmd.Wait returns → watchdog applies backoff and respawns
//	Close → SIGTERM → 5s grace → SIGKILL stragglers → status: stopped
//
// Thread-safe.
type ExternalProcess struct {
	Spec     ExternalProcessSpec
	HomePath string // workspace folder; injected as KTW_WORKSPACE at spawn

	mu             sync.RWMutex
	port           int
	token          string
	cmd            *exec.Cmd
	status         ExternalProcessStatus
	exited         chan struct{}
	exitErr        error
	requestedClose bool
	failed         bool
	restartTimes   []time.Time
	watching       bool // the watchdog was launched; only then does Close wait for watcherDone
	watcherDone    chan struct{}
}

// NewExternalProcess prepares a supervisor for the given spec. Does NOT spawn
// — call Start to bring the process online.
func NewExternalProcess(spec ExternalProcessSpec, homePath string) *ExternalProcess {
	return &ExternalProcess{
		Spec:        spec,
		HomePath:    homePath,
		status:      StatusStopped,
		watcherDone: make(chan struct{}),
	}
}

// Port returns the allocated port (0 if not yet spawned or no port allocated).
func (p *ExternalProcess) Port() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.port
}

// Token returns the per-spawn session token (empty if not yet spawned).
func (p *ExternalProcess) Token() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.token
}

// Status returns the current health label.
func (p *ExternalProcess) Status() ExternalProcessStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.status
}

// Start spawns the process and (if HealthzURL is set) waits for healthz to
// return 2xx. Errors if the spawn fails or healthz times out.
//
// Idempotent: if already running, returns nil immediately.
func (p *ExternalProcess) Start(ctx context.Context) error {
	p.mu.Lock()
	if p.status == StatusRunning || p.status == StatusStarting {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	if err := p.spawn(ctx); err != nil {
		p.mu.Lock()
		p.status = StatusFailed
		p.mu.Unlock()
		return err
	}

	if p.Spec.HealthzURL != "" {
		if err := p.waitHealthz(ctx); err != nil {
			_ = p.Close()
			return fmt.Errorf("healthz probe failed: %w", err)
		}
	}

	p.mu.Lock()
	if p.requestedClose {
		// Close ran while the process was still starting: it found no
		// watchdog to wait for, so none may start now — stop the process
		// this spawn left behind instead.
		cmd := p.cmd
		p.mu.Unlock()
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return fmt.Errorf("start %s: closed while starting", p.Spec.Command)
	}
	p.status = StatusRunning
	// Launch watchdog for restart-on-crash. Decided under the same lock
	// Close takes, so Close waits for watcherDone exactly when it will close.
	p.watching = true
	p.mu.Unlock()
	go p.watch(ctx)

	return nil
}

// spawn is one bare-process spawn (no healthz, no restart). Returns when the
// process has been Start()-ed (not necessarily healthy).
func (p *ExternalProcess) spawn(ctx context.Context) error {
	port, err := netutil.AllocFreePort()
	if err != nil {
		return err
	}
	token, err := generateSessionToken()
	if err != nil {
		return err
	}

	subst := substitutor{
		homePath:     p.HomePath,
		port:         strconv.Itoa(port),
		sessionToken: token,
	}

	args := make([]string, len(p.Spec.Args))
	for i, a := range p.Spec.Args {
		args[i] = subst.expand(a)
	}

	cmd := exec.CommandContext(ctx, p.Spec.Command, args...)
	NoConsoleWindow(cmd)
	cmd.Env = append(os.Environ(),
		"KTW_WORKSPACE="+p.HomePath,
		"ALLOC_PORT="+strconv.Itoa(port),
		"SESSION_TOKEN="+token,
	)
	for k, v := range p.Spec.Env {
		cmd.Env = append(cmd.Env, k+"="+subst.expand(v))
	}

	kind := p.Spec.Kind
	if kind == "" {
		kind = p.Spec.Command
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}
	go streamPrefixed(stderr, kind)

	p.mu.Lock()
	p.port = port
	p.token = token
	p.cmd = cmd
	p.status = StatusStarting
	p.exited = make(chan struct{})
	p.exitErr = nil
	p.mu.Unlock()

	if err := cmd.Start(); err != nil {
		// No process, so no Wait goroutine will ever close exited: close it
		// here, or Close would wait on it forever.
		p.mu.Lock()
		close(p.exited)
		p.mu.Unlock()
		return fmt.Errorf("start %s: %w", p.Spec.Command, err)
	}

	// Attach to the supervisor's Job Object so the sidecar dies with us
	// even on parent crash/panic/force-kill. No-op on POSIX (process group
	// + ctx cancellation already handle that). Best-effort: log + continue
	// on failure rather than refuse to spawn.
	if err := AttachToParentJob(cmd.Process); err != nil {
		log.Printf("[publish] failed to attach %s (pid=%d) to parent job: %v — orphan possible on parent crash",
			kind, cmd.Process.Pid, err)
	}

	go func() {
		err := cmd.Wait()
		p.mu.Lock()
		p.exitErr = err
		close(p.exited)
		p.mu.Unlock()
	}()

	return nil
}

// waitHealthz polls HealthzURL until it returns a 2xx response or the
// timeout elapses (Spec.HealthzTimeout, 10s by default).
func (p *ExternalProcess) waitHealthz(ctx context.Context) error {
	subst := substitutor{
		homePath:     p.HomePath,
		port:         strconv.Itoa(p.Port()),
		sessionToken: p.Token(),
	}
	url := subst.expand(p.Spec.HealthzURL)

	wait := p.Spec.HealthzTimeout
	if wait <= 0 {
		wait = 10 * time.Second
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()

	client := &http.Client{Timeout: 2 * time.Second}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	// Try once immediately, then on every tick.
	for {
		req, err := http.NewRequestWithContext(timeoutCtx, http.MethodGet, url, nil)
		if err == nil {
			resp, err := client.Do(req)
			if err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					return nil
				}
			}
		}
		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("healthz timeout (%s)", url)
		case <-ticker.C:
			// retry
		}
	}
}

// watch monitors the running process and restarts it on unexpected exit.
// Terminates cleanly when Close is called or maxRestarts is exceeded.
func (p *ExternalProcess) watch(ctx context.Context) {
	defer close(p.watcherDone)
	for {
		p.mu.RLock()
		exited := p.exited
		p.mu.RUnlock()

		select {
		case <-ctx.Done():
			return
		case <-exited:
			// Process ended.
		}

		p.mu.Lock()
		if p.requestedClose || p.failed {
			p.mu.Unlock()
			return
		}
		if !p.shouldRestartLocked() {
			p.failed = true
			p.status = StatusFailed
			p.mu.Unlock()
			return
		}
		backoff := p.computeBackoffLocked()
		p.mu.Unlock()

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		p.mu.Lock()
		if p.requestedClose {
			p.mu.Unlock()
			return
		}
		p.restartTimes = append(p.restartTimes, time.Now())
		p.mu.Unlock()

		if err := p.spawn(ctx); err != nil {
			p.mu.Lock()
			p.failed = true
			p.status = StatusFailed
			p.mu.Unlock()
			return
		}
		// Re-probe healthz on restart (best-effort; failures don't kill the watcher).
		if p.Spec.HealthzURL != "" {
			if err := p.waitHealthz(ctx); err != nil {
				p.mu.Lock()
				p.status = StatusFailed
				p.mu.Unlock()
				continue // try another restart cycle
			}
		}
		p.mu.Lock()
		p.status = StatusRunning
		p.mu.Unlock()
	}
}

// shouldRestartLocked returns true if the process is allowed to restart.
// Caller must hold p.mu.
//
// Policy: max p.Spec.MaxRestarts (default 5) within the last 5 minutes.
func (p *ExternalProcess) shouldRestartLocked() bool {
	max := p.Spec.MaxRestarts
	if max <= 0 {
		max = 5
	}
	cutoff := time.Now().Add(-5 * time.Minute)
	recent := 0
	for _, t := range p.restartTimes {
		if t.After(cutoff) {
			recent++
		}
	}
	return recent < max
}

// computeBackoffLocked returns the wait before the next restart.
// Exponential: 1, 2, 4, 8 seconds, capped at 60. Caller must hold p.mu.
func (p *ExternalProcess) computeBackoffLocked() time.Duration {
	n := len(p.restartTimes)
	d := time.Duration(1<<uint(n)) * time.Second
	if d > 60*time.Second {
		d = 60 * time.Second
	}
	return d
}

// Close marks the process as deactivating, signals the OS process, and
// waits for both the wait goroutine and the watchdog to drain.
//
// Idempotent.
func (p *ExternalProcess) Close() error {
	p.mu.Lock()
	if p.requestedClose {
		p.mu.Unlock()
		return nil
	}
	p.requestedClose = true
	cmd := p.cmd
	watching := p.watching
	p.mu.Unlock()

	if cmd != nil && cmd.Process != nil {
		// Cross-platform graceful term: os.Interrupt on Unix is SIGINT;
		// on Windows it's CTRL+BREAK semantics handled by the runtime.
		_ = cmd.Process.Signal(os.Interrupt)
	}

	// Wait up to 5 seconds for graceful exit.
	p.mu.RLock()
	exited := p.exited
	p.mu.RUnlock()
	if exited != nil {
		select {
		case <-exited:
			// graceful
		case <-time.After(5 * time.Second):
			if cmd != nil && cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			<-exited
		}
	}

	p.mu.Lock()
	p.status = StatusStopped
	p.mu.Unlock()

	// Watchdog sees requestedClose and exits. A process whose Start failed,
	// or is still probing healthz, has no watchdog — and never will (Start
	// checks requestedClose before launching one) — so there is nothing to
	// wait for.
	if watching {
		<-p.watcherDone
	}
	return nil
}

// substitutor expands ${VAR} placeholders in args/env values.
type substitutor struct {
	homePath     string
	port         string
	sessionToken string
}

// expand replaces ${VAR} occurrences in s with the resolved value.
//
// Built-in variables: KTW_WORKSPACE, ALLOC_PORT, SESSION_TOKEN.
// Unknown variables fall through to os.Getenv.
func (s substitutor) expand(input string) string {
	if !strings.Contains(input, "${") {
		return input
	}
	var b strings.Builder
	for i := 0; i < len(input); {
		if i+1 < len(input) && input[i] == '$' && input[i+1] == '{' {
			end := strings.IndexByte(input[i+2:], '}')
			if end < 0 {
				// unterminated — append rest verbatim
				b.WriteString(input[i:])
				break
			}
			key := input[i+2 : i+2+end]
			b.WriteString(s.resolve(key))
			i += 2 + end + 1
			continue
		}
		b.WriteByte(input[i])
		i++
	}
	return b.String()
}

func (s substitutor) resolve(key string) string {
	switch key {
	case "KTW_WORKSPACE":
		return s.homePath
	case "ALLOC_PORT":
		return s.port
	case "SESSION_TOKEN":
		return s.sessionToken
	default:
		return os.Getenv(key)
	}
}

// generateSessionToken returns a 32-byte crypto/rand value as 64-char hex.
// Used to gate HTTP requests to external-process sidecars.
func generateSessionToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("session token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// streamPrefixed reads from r, prefixes each line with [<kind>] and forwards
// to the standard logger. Non-blocking by design — if the consumer slows
// down, OS pipe back-pressure will eventually slow the sidecar. Failure
// modes (closed pipe, EOF) terminate the goroutine cleanly.
func streamPrefixed(r io.Reader, kind string) {
	prefix := "[" + kind + "] "
	buf := make([]byte, 4096)
	var pending []byte
	for {
		n, err := r.Read(buf)
		if n > 0 {
			pending = append(pending, buf[:n]...)
			for {
				idx := indexByte(pending, '\n')
				if idx < 0 {
					break
				}
				log.Print(prefix + string(pending[:idx+1]))
				pending = pending[idx+1:]
			}
		}
		if err != nil {
			if len(pending) > 0 {
				log.Print(prefix + string(pending))
			}
			return
		}
	}
}

// indexByte avoids an import for a single use.
func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}
