// SPDX-License-Identifier: Apache-2.0

package supervisor

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/usermsg"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	terminalapi "github.com/katyoti/katyoti-fwk/internal/extensions/terminal/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal"
)

// sessionID is the single session identity ktw-gui v1 uses per universe.
// All terminal runs in a given ActiveCore publish + subscribe under this
// session token. Multi-session per universe (per-chat-panel sessions) is
// a v2 enhancement; this is here so the migration is one constant change.
const guiSessionID = "default"

// terminalRouter demultiplexes events arriving on
// `ktw.<tenant>.universe.<u>.session.<sid>.task.<runID>.event.<type>`
// subjects into per-runID channels for downstream consumers.
//
// Each ActiveCore owns one router. On creation it installs a single
// wildcard subscription (`…session.<sid>.>`) — every run's events arrive
// on the same goroutine and are demuxed by parsing the subject's runID
// token. Cowork happens for free: plain `bus.Subscribe` (NOT
// QueueSubscribe) means every GUI instance subscribed to the same
// pattern receives every event independently.
//
// Thread-safe.
type terminalRouter struct {
	sender     ktsp.Sender
	bus        mbapi.MessageBus
	tenantID   string
	universeID string
	sessionID  string

	sub mbapi.Subscription // single per-session wildcard subscription

	mu   sync.Mutex
	runs map[string]*ktspRun
}

// ktspRun is one in-flight run on the client side: the channel events
// flow into and the done signal closes them.
type ktspRun struct {
	runID   string
	events  chan json.RawMessage
	done    chan struct{}
	doneErr string
	doneMsg *usermsg.Msg
	closed  bool
}

// newTerminalRouter wires a router onto the given ActiveCore's bus.
// Subscribes once to `…session.<sid>.>`, which covers every run's
// event + done subjects via a single goroutine — runID-demux happens by
// parsing the subject string.
func newTerminalRouter(core *ActiveCore) *terminalRouter {
	bus := core.MessageBus()
	r := &terminalRouter{
		sender:     core.Sender(),
		bus:        bus,
		tenantID:   activeTenantID,
		universeID: core.Name,
		sessionID:  guiSessionID,
		runs:       map[string]*ktspRun{},
	}
	pattern := fmt.Sprintf("ktw.%s.universe.%s.session.%s.>",
		r.tenantID, r.universeID, r.sessionID)
	sub, err := bus.Subscribe(context.Background(), pattern, r.onMessage)
	if err != nil {
		// Subscription failure is unusual — but non-fatal: StartTaskRun
		// will still queue, just no events flow. Log loudly so the user
		// notices in dev console.
		log.Printf("[terminalRouter] subscribe failed for %s: %v", pattern, err)
	}
	r.sub = sub
	return r
}

// Close unsubscribes the router and closes every outstanding run's
// channels with EOF semantics. Called by ActiveCore.Close. Idempotent.
func (r *terminalRouter) Close() {
	if r.sub != nil {
		_ = r.sub.Unsubscribe()
		r.sub = nil
	}
	r.mu.Lock()
	runs := r.runs
	r.runs = map[string]*ktspRun{}
	r.mu.Unlock()
	for _, run := range runs {
		if run.closed {
			continue
		}
		run.closed = true
		close(run.events)
		close(run.done)
	}
}

// StartTaskRun calls Terminal.StartTask on the core, registers the
// resulting runID, and returns a TerminalTransport adapter scoped to
// that run plus the journal session id the server pre-computed
// (UUIDv7 — matches the on-disk session directory under
// `<DataDir>/sessions/`).
//
// We pre-generate the runID and register the run BEFORE calling
// Terminal/start-task so any event published on the run's subject
// during the request round-trip slots cleanly into the run's channel
// rather than being dropped as "unknown runID".
func (r *terminalRouter) StartTaskRun(ctx context.Context, in terminalapi.StartTaskInput) (terminal.TerminalTransport, string, string, error) {
	runID, err := newClientRunID()
	if err != nil {
		return nil, "", "", fmt.Errorf("Terminal.StartTask: generate runID: %w", err)
	}
	in.RunID = runID
	if in.SessionID == "" {
		in.SessionID = r.sessionID
	}

	run := &ktspRun{
		runID:  runID,
		events: make(chan json.RawMessage, 256),
		done:   make(chan struct{}),
	}
	r.mu.Lock()
	r.runs[runID] = run
	r.mu.Unlock()

	respRaw, err := r.sender.Call(ctx, "ktw-agents:terminal/start-task", in)
	if err != nil {
		r.unregister(runID)
		return nil, "", "", fmt.Errorf("Terminal.StartTask: %w", err)
	}
	var resp terminalapi.StartTaskOutput
	if err := json.Unmarshal(respRaw, &resp); err != nil {
		r.unregister(runID)
		return nil, "", "", fmt.Errorf("Terminal.StartTask decode: %w", err)
	}
	if resp.RunID != runID {
		r.unregister(runID)
		return nil, "", "", fmt.Errorf("Terminal.StartTask: server returned runID %q, expected %q", resp.RunID, runID)
	}

	adapter := &ktspTerminalAdapter{
		router: r,
		run:    run,
	}
	return adapter, runID, resp.SessionID, nil
}

// ListSessions queries the core's live session registry (the Room, via
// the Terminal slot) — the running + recently-finished sessions across
// ALL runs on this core. A pure request/reply, independent of any run;
// scope (tenant/universe) rides the sender's wire headers.
func (r *terminalRouter) ListSessions(ctx context.Context) ([]terminalapi.SessionInfo, error) {
	respRaw, err := r.sender.Call(ctx, "ktw-agents:terminal/list-sessions", terminalapi.ListSessionsInput{})
	if err != nil {
		return nil, fmt.Errorf("Terminal.ListSessions: %w", err)
	}
	var resp terminalapi.ListSessionsOutput
	if err := json.Unmarshal(respRaw, &resp); err != nil {
		return nil, fmt.Errorf("Terminal.ListSessions decode: %w", err)
	}
	return resp.Sessions, nil
}

// newClientRunID mirrors the server format (16 hex chars / 8 random bytes).
// The charset overlaps with the NATS subject-token charset ([a-z0-9-])
// so we can use the runID directly as a subject token without escaping.
func newClientRunID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// onMessage is the bus handler installed by newTerminalRouter. The full
// subject layout is:
//
//	ktw.<tenant>.universe.<u>.session.<sid>.task.<runID>.event.<type>
//
// `<type>` ∈ {"event", "done"}. We split the subject by '.', then locate
// the `task` and `event` tokens to pull runID + type out unambiguously.
//
// Unknown / out-of-shape subjects are silently dropped — the wildcard
// pattern may pick up unrelated session events (mailbox, ledger, …) once
// other subsystems start publishing on the same session prefix; this
// router only cares about task-event subjects.
func (r *terminalRouter) onMessage(_ context.Context, msg mbapi.Message) {
	runID, kind, ok := parseTaskEventSubject(msg.Subject())
	if !ok {
		return
	}
	r.mu.Lock()
	run := r.runs[runID]
	r.mu.Unlock()
	if run == nil || run.closed {
		return
	}
	switch kind {
	case "event":
		data := msg.Data()
		// Two reliability classes (mirrors the terminal Room's
		// Broadcast-vs-AppendEvent split, which this NATS hop sits
		// downstream of):
		//   - msg.delta: ephemeral typing chunks — droppable on a full
		//     buffer (the authoritative full text follows on msg.new).
		//   - everything else (msg.new / msg.done / msg.update / stats /
		//     ui.*): AUTHORITATIVE — must NOT be dropped, or the chat
		//     permanently truncates (the trailing msg.new carrying the
		//     full assistant text was being dropped here during fast
		//     streams). Block briefly for the fast consumer to drain.
		if isDroppableEvent(data) {
			select {
			case run.events <- json.RawMessage(data):
			default: // buffer full — drop the ephemeral delta
			}
		} else {
			select {
			case run.events <- json.RawMessage(data):
			case <-run.done:
			default:
				timer := time.NewTimer(5 * time.Second)
				select {
				case run.events <- json.RawMessage(data):
				case <-run.done:
				case <-timer.C: // genuinely stuck consumer — last-resort drop
				}
				timer.Stop()
			}
		}
	case "done":
		r.completeRun(runID, msg.Data())
	}
}

// isDroppableEvent reports whether a terminal event may be dropped when
// the consumer buffer is full. Only ephemeral streaming deltas qualify;
// every persisted/authoritative event must be delivered.
func isDroppableEvent(data []byte) bool {
	var probe struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(data, &probe) != nil {
		return false // can't tell → treat as authoritative
	}
	return probe.Type == "msg.delta"
}

// parseTaskEventSubject extracts (runID, kind) from a subject of shape
// `ktw.<t>.universe.<u>.session.<s>.task.<runID>.event.<kind>`.
// Returns ok=false if the subject doesn't match that shape.
//
// Subject has 10 tokens (indices 0..9) — the fixed words `ktw`,
// `universe`, `session`, `task`, `event` plus their values. Per
// `subjapi.TaskEventSubject` which formats as
// "ktw.%s.universe.%s.session.%s.task.%s.event.%s" (5 args + 5 literal
// words = 10 dot-separated tokens).
func parseTaskEventSubject(subject string) (runID, kind string, ok bool) {
	parts := strings.Split(subject, ".")
	if len(parts) != 10 {
		return "", "", false
	}
	if parts[0] != "ktw" || parts[2] != "universe" || parts[4] != "session" ||
		parts[6] != "task" || parts[8] != "event" {
		return "", "", false
	}
	return parts[7], parts[9], true
}

// completeRun handles a Done notification: decodes the body for error
// context, drains the run from the map, and closes the run's channels.
func (r *terminalRouter) completeRun(runID string, body []byte) {
	r.mu.Lock()
	run, ok := r.runs[runID]
	if ok {
		delete(r.runs, runID)
	}
	r.mu.Unlock()
	if !ok || run == nil || run.closed {
		return
	}
	var done terminalapi.DoneNotification
	_ = json.Unmarshal(body, &done) // body is best-effort diagnostic; runID is authoritative from subject
	run.doneErr = done.Error
	run.doneMsg = done.Msg
	run.closed = true
	close(run.events)
	close(run.done)
}

// unregister removes a run from the router (called when the adapter
// closes locally — covers the "client cancelled before server done"
// case).
func (r *terminalRouter) unregister(runID string) {
	r.mu.Lock()
	run, ok := r.runs[runID]
	if ok {
		delete(r.runs, runID)
	}
	r.mu.Unlock()
	if ok && run != nil && !run.closed {
		run.closed = true
		close(run.events)
		close(run.done)
	}
}

// ── KTSP-backed TerminalTransport ────────────────────────────────────

// ktspTerminalAdapter implements terminal.TerminalTransport over a
// terminalRouter + ktsp.Sender pair, scoped to a single runID.
//
// From the consumer's perspective it's identical to adapter.Stdio /
// adapter.Channel — Send/Receive/Close — only the wire is different
// (bus Request + Publish/Subscribe instead of NDJSON over a pipe).
type ktspTerminalAdapter struct {
	router *terminalRouter
	run    *ktspRun
}

// Send forwards an event as Terminal.Cmd on the router's slot sender.
// The event is opaque to the bus — wire-compatible with the existing
// terminal protocol.
func (a *ktspTerminalAdapter) Send(ctx context.Context, event any) error {
	raw, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("ktsp adapter: marshal event: %w", err)
	}
	_, err = a.router.sender.Call(ctx, "ktw-agents:terminal/cmd", terminalapi.CmdInput{
		RunID:   a.run.runID,
		Command: raw,
	})
	if err != nil {
		return fmt.Errorf("Terminal.Cmd: %w", err)
	}
	return nil
}

// Receive returns the channel of incoming events. Closes when the run
// terminates (Done event received) or when Close is called locally.
func (a *ktspTerminalAdapter) Receive(_ context.Context) (<-chan json.RawMessage, error) {
	return a.run.events, nil
}

// Close unregisters the run from the router and best-effort cancels the
// server-side run via Terminal.Cancel. Idempotent.
// DoneError is why the run ended in error, as its Done notification said
// ("" for a run that ended well, or before its end). It is read once Receive's
// channel is closed: completeRun sets it before closing it.
func (a *ktspTerminalAdapter) DoneError() string { return a.run.doneErr }

// DoneMsg is the message key of the error the Done notification said (nil
// when it knew none): the chat says the failure in the person's language.
func (a *ktspTerminalAdapter) DoneMsg() *usermsg.Msg { return a.run.doneMsg }

// The GUI reads a run's end through them (sessionEnd, runEndEvents).
var _ interface {
	DoneError() string
	DoneMsg() *usermsg.Msg
} = (*ktspTerminalAdapter)(nil)

func (a *ktspTerminalAdapter) Close() error {
	// Best-effort cancel the server-side run; ignore errors since the
	// run may have already completed.
	_, _ = a.router.sender.Call(context.Background(), "ktw-agents:terminal/cancel", terminalapi.CancelInput{
		RunID:  a.run.runID,
		Reason: "client-close",
	})
	a.router.unregister(a.run.runID)
	return nil
}
