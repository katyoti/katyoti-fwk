// SPDX-License-Identifier: Apache-2.0

// Package engine provides the Engine type — the single runtime object that hosts
// ONE container with named slots, connects to the Session Manager, and manages
// lifecycle. Engine is pure infrastructure: it does not own plugin registries,
// task registries, or extension initialization — those are agent/consumer concerns.
//
// Any binary (ktw-agents, ktw-gui, sidecars) creates an Engine the same way:
//
//	e, err := engine.New(cfg, container)
//	err = e.Run(ctx)
package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	ktspendpoint "github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/endpoint"
	ktspjsonrpc "github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/jsonrpc"
	"github.com/katyoti/katyoti-fwk/internal/kts/sm"

	"github.com/google/uuid"
)

// Engine hosts ONE container with named slots and provides infrastructure.
// There is one Engine per process/binary. It generates a UUID at startup
// and registers with the Session Manager for inter-Engine routing.
type Engine struct {
	// Identity
	id   string // UUID generated at startup
	name string // human-readable name (e.g., "ktw-agents")
	cfg  EngineConfig

	// Container — the single container this Engine hosts
	container ktsp.Container

	// Infrastructure
	observers []Observer
	logger    *slog.Logger

	// SM connection
	smTransport ktsp.Transport       // transport to SM (channel or WS)
	sessionID   string               // session ID from SM
	sm          *sm.Manager          // local SM (only in embedded mode)
	cm          *sm.ContainerManager // local CM (only in embedded mode)

	// Lifecycle
	cancel   context.CancelFunc
	stop     chan struct{} // closed by Shutdown: Run ends, even one not started yet
	stopOnce sync.Once
	wg       sync.WaitGroup
	mu       sync.Mutex
}

// Observer is the agent.Observer interface re-exported for convenience.
// It is defined in internal/agent/services.go.
type Observer = interface {
	OnEvent(ctx context.Context, eventType string, turn int, data any)
	Close() error
}

// New creates a new Engine from a config and a pre-built container.
// The container should already have its slots registered.
// Call Run() to start the Engine's lifecycle.
func New(cfg EngineConfig, container ktsp.Container) (*Engine, error) {
	if container == nil {
		return nil, fmt.Errorf("engine: container must not be nil")
	}

	id := shortUUID()
	name := cfg.Name
	if name == "" {
		name = "engine-" + id
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})).With("engine", id, "container", name)

	e := &Engine{
		id:        id,
		name:      name,
		cfg:       cfg,
		container: container,
		logger:    logger,
		stop:      make(chan struct{}),
	}

	return e, nil
}

// ID returns the Engine's UUID.
func (e *Engine) ID() string { return e.id }

// Name returns the Engine's human-readable name.
func (e *Engine) Name() string { return e.name }

// Container returns the Engine's container.
func (e *Engine) Container() ktsp.Container { return e.container }

// Logger returns the Engine's structured logger.
func (e *Engine) Logger() *slog.Logger { return e.logger }

// Observers returns the list of active observers.
func (e *Engine) Observers() []Observer { return e.observers }

// NewContext creates an EngineContext for use by slot handlers.
func (e *Engine) NewContext(slotConfig map[string]any) EngineContext {
	return newEngineContext(e, slotConfig)
}

// SMTransport returns the transport to the Session Manager (nil if not connected).
func (e *Engine) SMTransport() ktsp.Transport { return e.smTransport }

// Run starts the Engine lifecycle:
//  1. Set up observers (trace, local journal, SM emitter)
//  2. Connect to SM (embedded or remote)
//  3. Register container with SM
//  4. Block until context is cancelled or Shutdown is called
func (e *Engine) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	e.mu.Lock()
	e.cancel = cancel
	e.mu.Unlock()

	defer cancel()
	// A Shutdown — also one that came before Run started — ends the run.
	go func() {
		select {
		case <-e.stop:
			cancel()
		case <-ctx.Done():
		}
	}()

	// 1. Set up observers
	e.setupObservers()

	// 2. Connect to SM
	if err := e.connectSM(ctx); err != nil {
		return fmt.Errorf("engine: connect SM: %w", err)
	}
	e.logger.Info("connected to SM", "mode", e.cfg.SMMode)

	// 3. Register container with SM
	if err := e.registerWithSM(ctx); err != nil {
		return fmt.Errorf("engine: register with SM: %w", err)
	}
	e.logger.Info("container registered", "id", e.id)

	// 4. Block until shutdown
	<-ctx.Done()
	e.logger.Info("engine shutting down")

	// Cleanup
	e.closeObservers()
	if e.smTransport != nil {
		e.smTransport.Close()
	}

	return nil
}

// Shutdown gracefully stops the Engine.
func (e *Engine) Shutdown() {
	e.stopOnce.Do(func() { close(e.stop) })
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cancel != nil {
		e.cancel()
	}
}

// --- Internal helpers ---

func shortUUID() string {
	u := uuid.New()
	return u.String()[:8] // first 8 hex chars for readability
}

func (e *Engine) setupObservers() {
	if e.cfg.Trace {
		e.observers = append(e.observers, NewTraceObserver(os.Stderr))
	}
	// TODO: LocalEmitter (journal) will be added when SessionRecorder is wired
	// TODO: EmitterStub (to SM journal) will be added when SM transport is wired
}

func (e *Engine) connectSM(ctx context.Context) error {
	switch e.cfg.SMMode {
	case SMEmbedded, "":
		return e.connectSMEmbedded(ctx)

	case SMRemote:
		return e.connectSMRemote(ctx)

	case SMProvided:
		return e.connectSMProvided()

	case SMStandalone:
		return fmt.Errorf("SMStandalone mode is for SM-only processes, not Engine processes")

	default:
		return fmt.Errorf("unknown SMMode: %q", e.cfg.SMMode)
	}
}

// connectSMEmbedded creates a local SM in-process with a ChannelTransport.
func (e *Engine) connectSMEmbedded(ctx context.Context) error {
	cm := sm.NewContainerManager()
	sm := sm.NewManager(cm)

	result, err := sm.Handle(ctx, "session", ktsp.DefaultInstance, "create", nil)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	var info struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(result, &info); err != nil {
		return fmt.Errorf("parse session: %w", err)
	}

	e.smTransport = ktspendpoint.NewLocal(sm)
	e.sessionID = info.SessionID
	e.sm = sm
	e.cm = cm
	return nil
}

// connectSMRemote connects to an external SM via WebSocket.
func (e *Engine) connectSMRemote(ctx context.Context) error {
	if e.cfg.SMURL == "" {
		return fmt.Errorf("SMRemote mode requires SMURL")
	}

	wsTransport, err := ktspjsonrpc.Dial(ctx, e.cfg.SMURL)
	if err != nil {
		return fmt.Errorf("connect to SM at %s: %w", e.cfg.SMURL, err)
	}

	// Create a session on the remote SM.
	createReq, _ := ktsp.NewRequest("engine-create", "session/create", nil)
	createResp, err := wsTransport.Send(ctx, createReq)
	if err != nil {
		wsTransport.Close()
		return fmt.Errorf("create remote session: %w", err)
	}
	if createResp.Error != nil {
		wsTransport.Close()
		return fmt.Errorf("create remote session: %s", createResp.Error.Message)
	}

	var info struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(createResp.Result, &info); err != nil {
		wsTransport.Close()
		return fmt.Errorf("parse remote session: %w", err)
	}

	e.smTransport = wsTransport
	e.sessionID = info.SessionID
	return nil
}

// connectSMProvided uses a pre-built SM transport (for topology-driven launcher).
func (e *Engine) connectSMProvided() error {
	if e.cfg.SMTransport == nil {
		return fmt.Errorf("SMProvided mode requires SMTransport in config")
	}
	e.smTransport = e.cfg.SMTransport
	e.sessionID = e.cfg.SessionID
	return nil
}

func (e *Engine) registerWithSM(ctx context.Context) error {
	if e.smTransport == nil {
		return nil
	}

	// In embedded mode, register directly in the session (Path A — no RPC needed).
	if e.sm != nil {
		sess, ok := e.sm.GetSession(e.sessionID)
		if !ok {
			return fmt.Errorf("session %q not found in local SM", e.sessionID)
		}
		ct := ktspendpoint.NewLocal(e.container)
		_, err := sess.Register("", e.name, e.container, ct)
		return err
	}

	// Remote/provided mode: register via RPC (Path B).
	sender := ktsp.NewSender(e.smTransport)
	_, err := sender.Call(ctx, "registry/register", map[string]string{
		"sessionId": e.sessionID,
		"name":      e.name,
	})
	if err != nil {
		return fmt.Errorf("register with SM: %w", err)
	}
	return nil
}

// SessionID returns the session ID (empty if not connected to SM).
func (e *Engine) SessionID() string { return e.sessionID }

// SM returns the local Session Manager (only set in embedded mode).
func (e *Engine) SM() *sm.Manager { return e.sm }

// CM returns the local ContainerManager (only set in embedded mode).
func (e *Engine) CM() *sm.ContainerManager { return e.cm }

func (e *Engine) closeObservers() {
	for _, obs := range e.observers {
		obs.Close()
	}
}
