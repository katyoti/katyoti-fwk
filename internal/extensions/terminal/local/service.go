// SPDX-License-Identifier: Apache-2.0

// Package local implements the KTSP Terminal service that runs inside a
// long-lived ktw-agents core. The service exposes the existing terminal
// protocol (events + commands) over the core's NATS bus so that remote
// clients (typically ktw-gui) can drive multiple tasks against one core
// without spawning per-task subprocesses, and can share a session
// across multiple GUI instances ("cowork") via NATS pub/sub fan-out.
//
// Wire model (post bus-split-ktw-gui-hub-cutover):
//
//   - Request/reply: StartTask / Cmd / Cancel on
//     `ktw.<tenant>.universe.<u>.slot.terminal.<method>` (handled by
//     the standard slot subscriber wired in `sc ktsp nats`).
//   - Streaming events: published by THIS service to
//     `ktw.<tenant>.universe.<u>.session.<sid>.task.<runID>.event.<type>`
//     where `<type>` ∈ {"event", "done"}. Subscribers (ktw-gui's
//     terminalRouter) use `bus.Subscribe` on `…session.<sid>.>` to fan
//     in every event in the session.
//
// Boundaries:
//   - The core continues to run task execution exactly as before via
//     the canonical ServeTask machinery (Room + Translator + HumanGate +
//     TaskService.Run). Nothing in the task pipeline changes.
//   - This service is a thin bridge that wraps each task in an in-process
//     Channel transport, then forwards channel events out as NATS
//     publishes on session-scoped subjects.
//
// See: internal/extensions/terminal/api/types.go for the wire contract.
package local

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/usermsg"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	subjapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/presets"
	"github.com/katyoti/katyoti-fwk/internal/extensions/terminal/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal/adapter"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal/room"
	tserver "github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal/server"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
)

// Event-type tokens published as the last segment of the session-scoped
// subject. The wildcard subscription `…session.<sid>.>` covers all of them.
const (
	eventTypeEvent = "event"
	eventTypeDone  = "done"
)

// defaultSessionID is used when StartTaskInput.SessionID is empty —
// the v1 ktw-gui convention is one session per universe.
const defaultSessionID = "default"

// Extension publishes the TerminalService into the application's KTSP
// container. AutoKTSPContainer auto-generates the slot from the
// reflective interface — this service participates like any other
// PublishedService.
func Extension() application.Extension {
	return appcore.PublishSimpleService[api.TerminalService](
		newService,
		appcore.WithServiceDef(api.TerminalServiceDef),
		appcore.WithKTSPContainer(),
		// CLI surface is not generated; the service is a wire contract
		// for KTSP clients (ktw-gui), not a CLI tool.
		appcore.WithKTSPSlotTags(map[string]string{"cli": "false"}),
	)
}

// newService is the publish-framework factory.
//
// IMPORTANT: the factory's ctx carries the *Application via WithApplication
// (set by the bootstrap path). KTSP request-handler ctx, by contrast, is
// `context.Background()` — no application attached. We therefore capture
// the *Application here and reuse it when spawning per-run contexts in
// StartTask, so ServeTask + downstream code (Room, EventBus, TaskService
// resolution, etc.) get a properly-initialized ctx.
func newService(ctx context.Context) (api.TerminalService, error) {
	app := application.AppFromContext(ctx)
	if app == nil {
		return nil, fmt.Errorf("Terminal service factory: no Application in ctx — service cannot resolve runtime deps")
	}
	return &service{
		app:  app,
		runs: map[string]*run{},
	}, nil
}

// service is the singleton TerminalService implementation. Holds the
// active runs map; thread-safe.
type service struct {
	app  *application.Application
	mu   sync.Mutex
	runs map[string]*run
}

// run is one in-flight task tracked by the service.
type run struct {
	id        string
	sessionID string
	cancel    context.CancelFunc
	// clientCh is the client-facing end of the in-process Channel pair.
	// The bridge goroutine pulls events out of it and forwards them as
	// bus publishes; commands flowing the other way (from the KTSP peer)
	// are written into it via Send.
	clientCh terminal.TerminalTransport
	// bus + scope captured at StartTask time and reused by the bridge
	// goroutine to emit notifications outside the request/response cycle.
	bus        mbapi.MessageBus
	tenantID   string
	universeID string
	done       chan struct{}
}

// StartTask spawns the named task inside this core and starts streaming
// terminal events back to subscribers via the bus, on session-scoped
// subjects. Returns a stable runID immediately; the task continues
// asynchronously.
//
// If in.RunID is supplied, the server uses it verbatim — this is the
// recommended pattern: the client pre-registers its notification consumer
// using the runID it generated, eliminating the race where early events
// could arrive before the client side has finished registering.
//
// Subject layout (per (modules/bus/decisions/OD-002-how-are-the-bus-s-subjects-streams-and-buckets.cwm.md)):
//
//	Event: ktw.<tenant>.universe.<u>.session.<sid>.task.<runID>.event.event
//	Done:  ktw.<tenant>.universe.<u>.session.<sid>.task.<runID>.event.done
//
// Tenant + universe are inherited from the inbound request's wire headers
// (set by every messagebus.Sender). Session defaults to "default" when
// in.SessionID is empty.
func (s *service) StartTask(ctx context.Context, in api.StartTaskInput) (api.StartTaskOutput, error) {
	if in.TaskName == "" {
		log.Print("[Terminal] StartTask: missing taskName")
		return api.StartTaskOutput{}, fmt.Errorf("Terminal.StartTask: taskName is required")
	}
	if s.app == nil {
		return api.StartTaskOutput{}, fmt.Errorf("Terminal.StartTask: no Application bound (factory was bypassed)")
	}
	bus, err := application.AppUseService[mbapi.MessageBus](s.app)
	if err != nil {
		return api.StartTaskOutput{}, fmt.Errorf("Terminal.StartTask: resolve MessageBus: %w", err)
	}
	tenantID := messagebus.TenantFromContext(ctx)
	universeID := messagebus.UniverseFromContext(ctx)
	if tenantID == "" || universeID == "" {
		return api.StartTaskOutput{}, fmt.Errorf("Terminal.StartTask: missing scope headers (tenant=%q universe=%q) — request must arrive via the messagebus Sender", tenantID, universeID)
	}
	// `in.SessionID` here is the BUS session id (defaults to "default")
	// — used for bus subject scoping. The JOURNAL session id is
	// generated separately below (UUIDv7 from engine.GenerateSessionID)
	// and returned in StartTaskOutput.SessionID so the caller can
	// subscribe to journal events / open the output view without
	// guessing which directory ServeTask will create.
	busSessionID := in.SessionID
	if busSessionID == "" {
		busSessionID = defaultSessionID
	}

	runID := in.RunID
	if runID == "" {
		generated, err := newRunID()
		if err != nil {
			return api.StartTaskOutput{}, fmt.Errorf("Terminal.StartTask: generate runID: %w", err)
		}
		runID = generated
	} else {
		s.mu.Lock()
		_, dup := s.runs[runID]
		s.mu.Unlock()
		if dup {
			return api.StartTaskOutput{}, fmt.Errorf("Terminal.StartTask: runID %q is already active", runID)
		}
	}

	// Pre-compute the journal session id (UUIDv7). We pass it down into
	// ServeTask → TaskService.Run via TaskOpts.JournalSessionID so the
	// recorder's on-disk directory matches what we return synchronously
	// here. Without this, ktw-gui (and any other client) would have to
	// list sessions and guess by task name + recency to find the new
	// session's journal.
	journalSessionID := subjapi.GenerateSessionID(in.TaskName)
	log.Printf("[Terminal] StartTask: runID=%s busSession=%s journalSession=%s task=%q clientLabel=%q",
		runID, busSessionID, journalSessionID, in.TaskName, in.ClientLabel)

	// In-process Channel pair. One end goes to ServeTask (the canonical
	// task runner); the other end is bridged to the bus.
	serverCh, clientCh := adapter.NewChannelPair(64)

	// Background context for the run: independent of the request ctx so
	// the task survives after StartTask has returned. We attach the
	// *Application captured at factory-construction time — the request
	// ctx doesn't carry it (KTSP dispatch ctx is bare).
	runCtx, cancel := context.WithCancel(application.WithApplication(context.Background(), s.app))

	r := &run{
		id:         runID,
		sessionID:  busSessionID,
		cancel:     cancel,
		clientCh:   clientCh,
		bus:        bus,
		tenantID:   tenantID,
		universeID: universeID,
		done:       make(chan struct{}),
	}

	s.mu.Lock()
	s.runs[runID] = r
	s.mu.Unlock()

	// Bridge: forward channel events out as bus publishes on
	// session-scoped subjects.
	go s.forwardEvents(runCtx, r)

	// Set the pre-computed journal session id on the inbound request so
	// the underlying TaskService.Run reuses it verbatim — both the
	// recorder's on-disk directory and the TaskRunResponse.SessionID
	// match what we return here.
	in.SessionID = journalSessionID

	// Run the task. Errors flow into the Done notification.
	go s.runTask(runCtx, r, in, serverCh)

	return api.StartTaskOutput{RunID: runID, SessionID: journalSessionID}, nil
}

// Cmd forwards a raw terminal-protocol command into the running task's
// input channel. The command is opaque to this service — wire-compatible
// with the existing NDJSON terminal protocol.
func (s *service) Cmd(ctx context.Context, in api.CmdInput) (api.CmdOutput, error) {
	r := s.lookup(in.RunID)
	if r == nil {
		return api.CmdOutput{}, fmt.Errorf("Terminal.Cmd: runID %q not found (already completed or never started)", in.RunID)
	}
	if len(in.Command) == 0 {
		return api.CmdOutput{}, fmt.Errorf("Terminal.Cmd: command is required")
	}
	// Forward the raw command into the client end of the channel — that
	// is, into the read side that ServeTask is listening on.
	if err := r.clientCh.Send(ctx, json.RawMessage(in.Command)); err != nil {
		return api.CmdOutput{}, fmt.Errorf("Terminal.Cmd: forward to runID %q: %w", in.RunID, err)
	}
	return api.CmdOutput{}, nil
}

// Cancel signals the run to stop. Idempotent.
func (s *service) Cancel(_ context.Context, in api.CancelInput) (api.CancelOutput, error) {
	r := s.lookup(in.RunID)
	if r == nil {
		// Already done or unknown — idempotent success.
		return api.CancelOutput{}, nil
	}
	r.cancel()
	return api.CancelOutput{}, nil
}

// ListSessions returns every session the core's Room currently holds —
// the live "running sessions" registry across ALL runs on this core.
// Sourced from the Room singleton (the session manager service), which
// ServeTask shares; NOT the on-disk session journals.
func (s *service) ListSessions(_ context.Context, _ api.ListSessionsInput) (api.ListSessionsOutput, error) {
	if s.app == nil {
		return api.ListSessionsOutput{}, fmt.Errorf("Terminal.ListSessions: no Application bound (factory was bypassed)")
	}
	r, err := application.AppUseService[*room.Room](s.app)
	if err != nil {
		return api.ListSessionsOutput{}, fmt.Errorf("Terminal.ListSessions: resolve Room: %w", err)
	}
	sessions := r.ListSessions()
	out := api.ListSessionsOutput{Sessions: make([]api.SessionInfo, 0, len(sessions))}
	for _, ses := range sessions {
		out.Sessions = append(out.Sessions, api.SessionInfo{
			ID:              ses.ID,
			ParentSessionID: ses.ParentSessionID,
			Name:            ses.Name,
			Status:          ses.Status,
			TaskID:          ses.TaskID,
			ParentTaskID:    ses.ParentTaskID,
			MessageCount:    ses.MessageCount,
			StartedAt:       ses.StartedAt,
		})
	}
	return out, nil
}

// ── internal ─────────────────────────────────────────────────────────────

func (s *service) lookup(runID string) *run {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runs[runID]
}

func (s *service) remove(runID string) {
	s.mu.Lock()
	delete(s.runs, runID)
	s.mu.Unlock()
}

// forwardEvents pulls terminal events from the client-side channel and
// publishes them on the session-scoped event subject. The payload is the
// raw terminal-protocol event JSON (no wrapper envelope — runID is in
// the subject path, event type is the trailing `event` token).
//
// Exits when the channel closes (run terminated) — at that point the
// runTask goroutine is responsible for emitting the final Done
// notification.
func (s *service) forwardEvents(ctx context.Context, r *run) {
	ch, err := r.clientCh.Receive(ctx)
	if err != nil {
		return
	}
	subject := subjects.TaskEventSubject(r.tenantID, r.universeID, r.sessionID, r.id, eventTypeEvent)
	for raw := range ch {
		if err := r.bus.Publish(ctx, subject, raw); err != nil {
			log.Printf("[Terminal] forwardEvents: publish failed for runID=%s: %v", r.id, err)
		}
	}
}

// runTask hosts ServeTask on the server-side channel and emits the final
// Done notification when it returns. Cleans up the runs map.
func (s *service) runTask(
	ctx context.Context,
	r *run,
	in api.StartTaskInput,
	serverCh terminal.TerminalTransport,
) {
	defer close(r.done)
	defer s.remove(r.id)
	defer serverCh.Close()
	defer r.clientCh.Close()

	// Build ConfigOverride for session-resume + plugin-context envelope.
	// When the caller passed pre-recorded InitialMessages (from a
	// session's llm.jsonl), the agent loop runs with
	// `HistoryMode: "full_fork"` and replays them instead of starting
	// fresh. When the caller passed a Context envelope, it is forwarded
	// as `ConfigOverride["context"]` → `AgentRunRequest.Context` →
	// `WithPluginContext(ctx, ...)`. The two paths are independent and
	// compose: a resumed session that wants a node-aware envelope sets
	// both.
	var configOverride map[string]any
	if len(in.InitialMessages) > 0 {
		// Decode each raw JSON into a generic shape so the agent factory
		// gets `map[string]any` entries (its existing fork-mode decoder
		// re-marshals these into agent.api.Message). Keeps this layer
		// free of the agent's import.
		msgs := make([]any, 0, len(in.InitialMessages))
		for _, raw := range in.InitialMessages {
			var m any
			if err := json.Unmarshal(raw, &m); err != nil {
				log.Printf("[Terminal] runTask: skip malformed initialMessages entry: %v", err)
				continue
			}
			msgs = append(msgs, m)
		}
		if len(msgs) > 0 {
			configOverride = map[string]any{
				presets.OverrideKeyInitialMessages: msgs,
				presets.OverrideKeyHistoryMode:     string(presets.HistoryFullFork),
			}
		}
	}
	if len(in.Context) > 0 {
		if configOverride == nil {
			configOverride = make(map[string]any, 1)
		}
		// Copy so downstream mutation (e.g. agent factory adding
		// "user_input") doesn't leak back to the caller's map.
		envelope := make(map[string]any, len(in.Context))
		for k, v := range in.Context {
			envelope[k] = v
		}
		configOverride["context"] = envelope
	}

	err := tserver.ServeTask(ctx, serverCh, tserver.TaskOpts{
		TaskName:       in.TaskName,
		Input:          in.Input,
		WorkspaceRoot:  in.WorkspaceRoot,
		DryRun:         in.DryRun,
		ConfigOverride: configOverride,
		// SessionID was set on `in` by StartTask before forking this
		// goroutine — pre-computed UUIDv7 forwarded so the recorder
		// uses the same id we returned in StartTaskOutput.SessionID.
		SessionID: in.SessionID,
		// The run's conversation is its chat panel on the universe's panel
		// subjects: the panel server keeps it, any screen follows it, and
		// the person's message on it is the run's next turn.
		Panels: r.panelWire(),
	})

	// Publish the terminal Done notification on the session-scoped done
	// subject. We use a fresh ctx because the per-run ctx is about to be
	// torn down by the deferred cancels.
	donePayload, mErr := json.Marshal(doneNotification(r.id, err))
	if mErr != nil {
		log.Printf("[Terminal] runTask: marshal Done for runID=%s: %v", r.id, mErr)
		return
	}
	subject := subjects.TaskEventSubject(r.tenantID, r.universeID, r.sessionID, r.id, eventTypeDone)
	if pErr := r.bus.Publish(context.Background(), subject, donePayload); pErr != nil {
		log.Printf("[Terminal] runTask: publish Done for runID=%s: %v", r.id, pErr)
	}
}

// panelWire is the run's reach to the universe's panel subjects, on the
// bus and in the scope it was started in.
func (r *run) panelWire() *tserver.PanelWire {
	if r.bus == nil {
		return nil
	}
	bus := r.bus
	return &tserver.PanelWire{
		Tenant:   r.tenantID,
		Universe: r.universeID,
		Publish: func(ctx context.Context, subject string, payload []byte) error {
			return bus.Publish(ctx, subject, payload)
		},
		Subscribe: func(ctx context.Context, subject string, deliver func([]byte)) (func(), error) {
			sub, err := bus.Subscribe(ctx, subject, func(_ context.Context, msg mbapi.Message) { deliver(msg.Data()) })
			if err != nil {
				return nil, err
			}
			return func() { _ = sub.Unsubscribe() }, nil
		},
	}
}

// doneNotification is a run's Done: its error, and the error's message key
// when it knows one, so that a client says the failure in the person's
// language even when the run's own error message did not reach it (a run
// that fails at once has its message published a hair after its Done).
func doneNotification(runID string, err error) api.DoneNotification {
	done := api.DoneNotification{RunID: runID, Error: errorString(err)}
	if m, ok := usermsg.Of(err); ok {
		done.Msg = &m
	}
	return done
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// newRunID returns 16 hex chars (8 random bytes) — short enough to log,
// long enough to avoid collision within a session.
func newRunID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// Compile-time interface assertion.
var _ api.TerminalService = (*service)(nil)
