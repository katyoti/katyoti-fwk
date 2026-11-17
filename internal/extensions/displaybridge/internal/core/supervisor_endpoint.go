// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/publish"
)

// supervisedProcess is the slice of *publish.ExternalProcess the endpoint
// drives. Tests substitute a fake to count spawns without running bun.
type supervisedProcess interface {
	Start(ctx context.Context) error
	Close() error
	Port() int
	Token() string
	Status() publish.ExternalProcessStatus
}

// errEndpointClosed is the start error once Close has run: a closed bridge
// never spawns bun again.
var errEndpointClosed = errors.New("displaybridge: endpoint closed")

// SupervisorEndpoint is an EndpointProvider backed by a supervised bun
// process it owns. Nothing is spawned at construction: the first call that
// needs bun (BaseURL / Token) starts it, so a binary that boots the bridge
// but never renders a display (a one-shot CLI command) runs no bun at all.
//
// Start is serialized: concurrent first callers wait on one start and share
// its outcome. A failed start is reported to the caller and leaves no
// process behind; the next use tries again with a fresh process (an
// ExternalProcess whose healthz failed is closed and cannot be restarted).
//
// Close terminates the running process, or the one still starting, and
// refuses every later start. The embedding schemaBridge calls it on app
// shutdown (io.Closer convention).
type SupervisorEndpoint struct {
	newProc  func() supervisedProcess
	startCtx context.Context

	// onStarted runs once, after the first successful start (the bridge
	// starts its SSE subscriber there, so the subscriber never spawns bun).
	onStarted     func()
	onStartedOnce sync.Once

	startMu sync.Mutex // serializes starters; never held by Close

	mu       sync.Mutex
	proc     supervisedProcess // the running process; nil until a start succeeds
	starting supervisedProcess // the process a start is bringing up
	closed   bool
}

// NewSupervisorEndpoint builds a SupervisorEndpoint bound to the given
// spec. The homePath is injected as KTW_WORKSPACE on the spawn.
//
// startCtx is the lifetime context for the supervised process: its
// cancellation tears down bun. Use a long-lived ctx (the application's
// own ctx). Per-request contexts must NOT be passed here — they cancel
// after a single bridge call and would kill bun mid-flight.
func NewSupervisorEndpoint(spec publish.ExternalProcessSpec, homePath string, startCtx context.Context) *SupervisorEndpoint {
	return newSupervisorEndpoint(func() supervisedProcess {
		return publish.NewExternalProcess(spec, homePath)
	}, startCtx)
}

func newSupervisorEndpoint(newProc func() supervisedProcess, startCtx context.Context) *SupervisorEndpoint {
	return &SupervisorEndpoint{newProc: newProc, startCtx: startCtx}
}

// ensureStarted returns the running process, starting it on first need.
func (e *SupervisorEndpoint) ensureStarted() (supervisedProcess, error) {
	if p, err := e.current(); p != nil || err != nil {
		return p, err
	}

	e.startMu.Lock()
	defer e.startMu.Unlock()
	// Another starter may have finished while this one waited.
	if p, err := e.current(); p != nil || err != nil {
		return p, err
	}

	p := e.newProc()
	e.mu.Lock()
	e.starting = p
	e.mu.Unlock()

	err := p.Start(e.startCtx)

	e.mu.Lock()
	e.starting = nil
	closed := e.closed
	if err == nil && !closed {
		e.proc = p
	}
	e.mu.Unlock()

	if closed {
		_ = p.Close()
		return nil, errEndpointClosed
	}
	if err != nil {
		_ = p.Close()
		return nil, err
	}
	if e.onStarted != nil {
		e.onStartedOnce.Do(e.onStarted)
	}
	return p, nil
}

func (e *SupervisorEndpoint) current() (supervisedProcess, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil, errEndpointClosed
	}
	return e.proc, nil
}

// BaseURL starts bun on first call (blocking until /healthz returns 2xx
// or the probe times out), then returns the loopback URL with the
// kernel-allocated port. Returns "" if the start failed — the httpClient
// maps this to ErrEndpointUnavailable.
func (e *SupervisorEndpoint) BaseURL() string {
	p, err := e.ensureStarted()
	if err != nil {
		return ""
	}
	port := p.Port()
	if port == 0 {
		return ""
	}
	return fmt.Sprintf("http://127.0.0.1:%s", strconv.Itoa(port))
}

// Token returns the per-spawn session token. Empty if bun isn't started.
func (e *SupervisorEndpoint) Token() string {
	p, err := e.ensureStarted()
	if err != nil {
		return ""
	}
	return p.Token()
}

// Close terminates the supervised bun process (SIGINT + 5s grace), or the
// one still starting, and refuses later starts. Idempotent; spawns nothing
// when bun was never used.
func (e *SupervisorEndpoint) Close() error {
	e.mu.Lock()
	e.closed = true
	p := e.proc
	if p == nil {
		p = e.starting
	}
	e.mu.Unlock()
	if p == nil {
		return nil
	}
	return p.Close()
}

// Status returns the current supervisor status — used by
// useDisplayStatus / debug panels for visibility. Stopped until first use.
func (e *SupervisorEndpoint) Status() publish.ExternalProcessStatus {
	e.mu.Lock()
	p := e.proc
	e.mu.Unlock()
	if p == nil {
		return publish.StatusStopped
	}
	return p.Status()
}
