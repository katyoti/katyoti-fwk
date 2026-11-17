// SPDX-License-Identifier: Apache-2.0

// Package room implements the Room service — the L3 bridge between the KTSP
// system world and terminal protocol users.
//
// A Room manages terminal sessions (chat threads), fans out messages to all
// connected TerminalTransport clients, handles the connect/sync handshake
// (history replay + live subscription), and routes bridge calls through KTSP.
//
// Usage:
//
//	room := room.NewRoom(dataDir, sender)
//
//	// Producer side: terminal translator calls AppendEvent
//	room.AppendEvent("sess-1", terminal.MsgNewEvent{...})
//
//	// Consumer side: transport layer calls Connect
//	room.Connect(ctx, transport)
package room

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal"
)

// Room bridges the KTSP system world and terminal protocol users.
// It is a singleton per ktw-agents instance (ApplicationScope).
type Room struct {
	dataDir string
	sender  ktsp.Sender       // nil = bridge calls return an error
	bus     eventbus.EventBus // nil = SendMessageCmd returns an error

	mu       sync.RWMutex
	sessions map[string]*sessionState
	clients  map[string]*clientConn

	closeOnce sync.Once
	done      chan struct{}
}

// sessionState holds a terminal session, its message log, and its HumanGate.
type sessionState struct {
	session  terminal.Session
	journal  *terminal.Journal
	messages []terminal.Message // in-memory log (replayed to new clients on connect)
	// panels are the chat panel events journaled, the stale replaces left
	// out (terminal.KeepPanelEvent): replayed with the messages on connect,
	// so a reopened session still shows its artifacts.
	panels []terminal.PanelEvent
	// panelBytes is each kept panel event's line size in the journal (by
	// terminal.PanelEventKey), keptBytes their sum; stale holds the events
	// still in the journal file that a later replace made stale, staleBytes
	// their size: compaction drops them from the file (compactLocked).
	panelBytes map[string]int
	keptBytes  int
	stale      map[string]int
	staleBytes int
	// gate is THIS session's HumanGate (a per-session attribute). Answer /
	// permission / ui_event commands from a client of this session route to
	// it — isolating parallel chats. nil = no prompt routing for the session.
	gate *HumanGate
}

// clientConn represents a connected TerminalTransport with a buffered fan-out channel.
type clientConn struct {
	id        string
	sessionID string // the session this client follows; "" = all (observe)
	transport terminal.TerminalTransport
	ch        chan any      // fan-out channel: events written by AppendEvent
	done      chan struct{} // closed when this client disconnects
}

// NewRoom creates a Room. dataDir is the base path for journal files
// (journals are written to dataDir/terminal/<sessionID>/terminal.jsonl).
// sender is optional — if nil, CallCmd bridge requests return an error response.
func NewRoom(dataDir string, sender ktsp.Sender) *Room {
	return &Room{
		dataDir:  dataDir,
		sender:   sender,
		sessions: make(map[string]*sessionState),
		clients:  make(map[string]*clientConn),
		done:     make(chan struct{}),
	}
}

// SetHumanGate installs a terminal-protocol-backed HumanGate FOR A SESSION.
// "answer"/"permission"/"ui_event" commands from a client are delivered to
// the gate of that client's session (parallel-chat isolation).
func (r *Room) SetHumanGate(sessionID string, gate *HumanGate) {
	r.mu.Lock()
	if s, ok := r.sessions[sessionID]; ok {
		s.gate = gate
	}
	r.mu.Unlock()
}

// SetBus installs the event bus the room uses to publish team-related events
// (currently: agent-to-agent messages routed via SendMessageCmd). Set once at
// ServeTask boot from the application's shared bus service.
func (r *Room) SetBus(bus eventbus.EventBus) {
	r.bus = bus
}

// HumanGateInstance returns a session's HumanGate (nil if not set).
func (r *Room) HumanGateInstance(sessionID string) *HumanGate {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if s, ok := r.sessions[sessionID]; ok {
		return s.gate
	}
	return nil
}

// ── Session management ────────────────────────────────────────────────────

// CreateSession creates a new terminal session and opens its journal.
func (r *Room) CreateSession(id, name, taskID string) (*terminal.Session, error) {
	if id == "" {
		id = fmt.Sprintf("sess-%s", uuid.New().String()[:8])
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.sessions[id]; exists {
		return nil, fmt.Errorf("room: session %q already exists", id)
	}

	j, err := terminal.NewJournal(filepath.Join(r.dataDir, "terminal"), id)
	if err != nil {
		return nil, fmt.Errorf("room: create journal for %q: %w", id, err)
	}

	sess := terminal.Session{
		ID:        id,
		Name:      name,
		TaskID:    taskID,
		Status:    "running",
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}

	// Replay existing messages from journal (catch-up after restart).
	journalDir := filepath.Join(r.dataDir, "terminal")
	existing, _ := terminal.ReplayMessages(journalDir, id)
	panels, _ := terminal.ReplayPanelEvents(journalDir, id)

	state := &sessionState{
		session:    sess,
		journal:    j,
		messages:   existing,
		panels:     panels,
		panelBytes: map[string]int{},
		stale:      map[string]int{},
	}
	state.session.MessageCount = len(existing)
	// A journal written before compaction (or by a run cut short) may hold
	// stale panel events: the replay kept the live ones, the file drops the
	// others now.
	live := map[string]bool{}
	for _, pe := range panels {
		key := terminal.PanelEventKey(pe)
		live[key] = true
		if raw, err := json.Marshal(pe); err == nil {
			state.panelBytes[key] = len(raw) + 1
			state.keptBytes += len(raw) + 1
		}
	}
	_, _ = j.Compact(func(line []byte) bool {
		key := terminal.PanelLineKey(line)
		return key != "" && !live[key]
	})
	r.sessions[id] = state
	return &sess, nil
}

// GetSession returns a snapshot of a session by ID.
func (r *Room) GetSession(id string) (terminal.Session, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if state, ok := r.sessions[id]; ok {
		return state.session, true
	}
	return terminal.Session{}, false
}

// ListSessions returns a snapshot of all terminal sessions.
func (r *Room) ListSessions() []terminal.Session {
	r.mu.RLock()
	defer r.mu.RUnlock()
	sessions := make([]terminal.Session, 0, len(r.sessions))
	for _, state := range r.sessions {
		sessions = append(sessions, state.session)
	}
	return sessions
}

// UpdateSessionStatus changes a session's status field.
func (r *Room) UpdateSessionStatus(id, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, ok := r.sessions[id]
	if !ok {
		return fmt.Errorf("room: session %q not found", id)
	}
	state.session.Status = status
	return nil
}

// ── Message log ───────────────────────────────────────────────────────────

// AppendEvent persists an event to the session journal, updates the in-memory
// message log (for MsgNewEvent), and fans the event out to all connected clients.
func (r *Room) AppendEvent(sessionID string, event any) error {
	r.mu.RLock()
	state, ok := r.sessions[sessionID]
	r.mu.RUnlock()
	if !ok {
		return fmt.Errorf("room: session %q not found", sessionID)
	}

	_, size, err := state.journal.Append(event)
	if err != nil {
		return fmt.Errorf("room: journal append: %w", err)
	}

	// Update in-memory message log for msg.new events (used for history replay).
	if msg, isMsgNew := asMsgNew(event); isMsgNew {
		r.mu.Lock()
		state.messages = append(state.messages, msg)
		state.session.MessageCount = len(state.messages)
		r.mu.Unlock()
	}
	if pe, isPanel := asPanelEvent(event); isPanel {
		r.mu.Lock()
		state.keepPanel(pe, size)
		bound := state.staleBytes > max(state.keptBytes, compactBound)
		r.mu.Unlock()
		if bound {
			r.compact(state)
		}
	}

	// Reliable fan-out to all connected clients. Unlike Broadcast
	// (ephemeral msg.delta — droppable), AppendEvent carries PERSISTED
	// events (msg.new with the authoritative full text, msg.update,
	// lifecycle). These must NOT be dropped: a fast delta stream can
	// transiently fill a client's 256-buffer, and silently dropping the
	// trailing msg.new there left the chat permanently showing the
	// lossy delta-accumulated text (the journal — this same Append —
	// was always complete). We snapshot the client channels, release
	// the lock, then send with a short per-client timeout so a
	// momentarily-full buffer drains instead of losing the event. A
	// genuinely stuck client times out (it gets torn down elsewhere on
	// disconnect) rather than stalling the room.
	// Fan out ONLY to clients following this session (or observe-all
	// clients, sessionID==""). Without this scoping, a parallel chat's
	// client receives this session's events (cross-run leak / dup).
	r.mu.RLock()
	chans := make([]chan any, 0, len(r.clients))
	for _, client := range r.clients {
		if client.sessionID == sessionID || client.sessionID == "" {
			chans = append(chans, client.ch)
		}
	}
	r.mu.RUnlock()
	for _, ch := range chans {
		select {
		case ch <- event:
		default:
			// Buffer full — wait briefly for the drain goroutine instead
			// of dropping a persisted event.
			timer := time.NewTimer(5 * time.Second)
			select {
			case ch <- event:
				timer.Stop()
			case <-timer.C:
				// Genuinely stuck client — drop as last resort.
			}
		}
	}

	return nil
}

// compactBound is the stale panel bytes a running session's journal may
// hold before it is compacted - beyond the live ones: a run rewrites its
// journal only past both, so a short run is never rewritten and a long one
// keeps its file within twice its live panels.
const compactBound = 1 << 20

// keepPanel keeps a journaled panel event for the replay (KeepPanelEvent)
// and counts the events it makes stale. Caller holds r.mu.
func (s *sessionState) keepPanel(pe terminal.PanelEvent, size int) {
	before := make([]string, len(s.panels))
	for i, k := range s.panels {
		before[i] = terminal.PanelEventKey(k)
	}
	s.panels = terminal.KeepPanelEvent(s.panels, pe)
	after := make(map[string]bool, len(s.panels))
	for _, k := range s.panels {
		after[terminal.PanelEventKey(k)] = true
	}
	for _, key := range before {
		if after[key] {
			continue
		}
		n := s.panelBytes[key]
		delete(s.panelBytes, key)
		s.keptBytes -= n
		s.stale[key] = n
		s.staleBytes += n
	}
	key := terminal.PanelEventKey(pe)
	s.panelBytes[key] = size
	s.keptBytes += size
}

// CompactJournal drops from a session's journal file the chat panel events
// a later replace made stale, so the file holds each node's last replace
// once - what a reopen replays. The translator calls it
// when a run ends; a running session compacts past compactBound.
func (r *Room) CompactJournal(sessionID string) error {
	r.mu.RLock()
	state, ok := r.sessions[sessionID]
	r.mu.RUnlock()
	if !ok {
		return fmt.Errorf("room: session %q not found", sessionID)
	}
	return r.compact(state)
}

// compact rewrites the session's journal without its stale panel events.
// Events made stale while it runs stay counted for the next compaction.
func (r *Room) compact(state *sessionState) error {
	r.mu.RLock()
	drop := make(map[string]bool, len(state.stale))
	for key := range state.stale {
		drop[key] = true
	}
	r.mu.RUnlock()
	if len(drop) == 0 {
		return nil
	}
	_, err := state.journal.Compact(func(line []byte) bool {
		return drop[terminal.PanelLineKey(line)]
	})
	if err != nil {
		return fmt.Errorf("room: compact journal: %w", err)
	}
	r.mu.Lock()
	for key := range drop {
		delete(state.stale, key)
	}
	state.staleBytes = 0
	for _, n := range state.stale {
		state.staleBytes += n
	}
	r.mu.Unlock()
	return nil
}

// Broadcast sends an ephemeral event (msg.delta, stats, team events) to the
// clients following `sessionID` (or observe-all), WITHOUT persisting it.
// Session-scoped like AppendEvent so parallel chats don't cross-leak.
func (r *Room) Broadcast(sessionID string, event any) {
	r.mu.RLock()
	for _, client := range r.clients {
		if client.sessionID != sessionID && client.sessionID != "" {
			continue
		}
		select {
		case client.ch <- event:
		default: // drop silently — slow client
		}
	}
	r.mu.RUnlock()
}

// GetHistory returns the in-memory message log for a session.
func (r *Room) GetHistory(sessionID string) ([]terminal.Message, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	state, ok := r.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("room: session %q not found", sessionID)
	}
	snapshot := make([]terminal.Message, len(state.messages))
	copy(snapshot, state.messages)
	return snapshot, nil
}

// ── Client connect/sync ───────────────────────────────────────────────────

// Connect registers a TerminalTransport as a connected client, sends the
// current state (StateSyncEvent with sessions + history), then subscribes
// the client to live events. The client is unregistered when ctx is cancelled
// or the transport closes.
func (r *Room) Connect(ctx context.Context, t terminal.TerminalTransport, sessionID string) error {
	if err := r.sendSync(ctx, t); err != nil {
		return fmt.Errorf("room: connect sync: %w", err)
	}

	connID := uuid.New().String()
	conn := &clientConn{
		id:        connID,
		sessionID: sessionID,
		transport: t,
		ch:        make(chan any, 1024),
		done:      make(chan struct{}),
	}

	r.mu.Lock()
	r.clients[connID] = conn
	gate := r.gateLocked(sessionID)
	n := r.countSessionClientsLocked(sessionID)
	r.mu.Unlock()
	if gate != nil {
		gate.SetAttachedClients(n)
	}

	// Fan-out goroutine: drain conn.ch → t.Send.
	go func() {
		defer func() {
			close(conn.done)
			r.mu.Lock()
			delete(r.clients, connID)
			remaining := r.countSessionClientsLocked(sessionID)
			g := r.gateLocked(sessionID)
			r.mu.Unlock()
			if g != nil {
				// Last client of THIS session gone → its gate arms the
				// detach grace timer; pending prompts fall over to the
				// inbox when it expires. Reconnects disarm it.
				g.SetAttachedClients(remaining)
			}
		}()
		for {
			select {
			case event, ok := <-conn.ch:
				if !ok {
					return
				}
				if err := t.Send(context.Background(), event); err != nil {
					return // transport closed
				}
			case <-ctx.Done():
				return
			case <-r.done:
				return
			}
		}
	}()

	// Command dispatch goroutine: t.Receive → handleCommand.
	go func() {
		ch, err := t.Receive(ctx)
		if err != nil {
			return
		}
		for {
			select {
			case raw, ok := <-ch:
				if !ok {
					return
				}
				r.handleCommand(ctx, t, raw, sessionID)
			case <-ctx.Done():
				return
			case <-r.done:
				return
			case <-conn.done:
				return
			}
		}
	}()

	return nil
}

// gateLocked returns a session's HumanGate (nil if absent). Caller holds r.mu.
func (r *Room) gateLocked(sessionID string) *HumanGate {
	if s, ok := r.sessions[sessionID]; ok {
		return s.gate
	}
	return nil
}

// countSessionClientsLocked returns how many connected clients follow
// sessionID. Caller holds r.mu.
func (r *Room) countSessionClientsLocked(sessionID string) int {
	n := 0
	for _, c := range r.clients {
		if c.sessionID == sessionID {
			n++
		}
	}
	return n
}

// ── Bridge / command dispatch ─────────────────────────────────────────────

// handleCommand decodes a raw client command and dispatches it. sessionID is
// the command's source session (the client's session) — gate ops route to
// THAT session's gate so two live chats don't cross-answer.
func (r *Room) handleCommand(ctx context.Context, t terminal.TerminalTransport, raw json.RawMessage, sessionID string) {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return
	}
	gate := r.HumanGateInstance(sessionID)
	switch probe.Type {
	case "connect":
		// Late / reconnect: resend current state, optionally from a seqNo.
		var cmd terminal.ConnectCmd
		_ = json.Unmarshal(raw, &cmd)
		_ = r.sendSyncFrom(ctx, t, cmd.Params.LastSeqNo)
	case "answer":
		if gate != nil {
			var cmd terminal.AnswerCmd
			if json.Unmarshal(raw, &cmd) == nil {
				gate.DeliverAnswer(cmd.Params.Value)
			}
		}
	case "permission":
		if gate != nil {
			var cmd terminal.PermissionCmd
			if json.Unmarshal(raw, &cmd) == nil {
				gate.DeliverAnswer(cmd.Params.Decision)
			}
		}
	case "ui_event":
		// User interaction on a mounted UI artifact (row click, form
		// submit, …) flowing back to the agent — Mode A of the CRUD
		// action wiring. The chat agent's rhythm is ask-user → wait, so
		// the event is delivered as the pending prompt's answer, in a
		// JSON envelope the agent's prompt (book-crud-demo etc.)
		// documents. With no prompt pending the gate buffers one event;
		// further events are dropped until the agent asks again.
		if gate != nil {
			var cmd terminal.UIEventCmd
			if json.Unmarshal(raw, &cmd) == nil {
				// A submit/cancel for an artifact that is a display panel
				// (the ask-user-data question) goes to that panel's input,
				// where the run waiting on it takes the structured value.
				// Any other event falls back to the legacy Mode-A path
				// (deliver as the pending prompt's answer envelope).
				if !gate.DeliverPanelInput(ctx, cmd.Params.ArtifactID, cmd.Params.EventType, cmd.Params.Value) {
					envelope, err := json.Marshal(map[string]any{
						"ui_event": map[string]any{
							"artifact_id": cmd.Params.ArtifactID,
							"event":       cmd.Params.EventType,
							"value":       cmd.Params.Value,
						},
					})
					if err == nil {
						gate.DeliverAnswer(string(envelope))
					}
				}
			}
		}
	case "call":
		r.handleCall(ctx, t, raw)
	case "send_message":
		r.handleSendMessage(ctx, t, raw)
	case "cancel_child":
		r.handleCancelChild(ctx, t, raw)
	}
}

// handleSendMessage publishes an AgentMessageEvent on the engine's bus on
// behalf of the GUI. The recipient agent's mailbox-EventBus bridge (when
// active) routes the message to their check-messages tool.
//
// Sender identity is hardcoded to "gui-user" — see gui-team-tab-functional
// design decision (Option 2 v1). Direct messages only; group send is out of
// scope for v1.
func (r *Room) handleSendMessage(ctx context.Context, t terminal.TerminalTransport, raw json.RawMessage) {
	var cmd terminal.SendMessageCmd
	if err := json.Unmarshal(raw, &cmd); err != nil {
		_ = t.Send(ctx, terminal.SystemContent{Type: "system", Level: "error",
			Message: fmt.Sprintf("room: invalid send_message: %v", err)})
		return
	}
	if r.bus == nil {
		_ = t.Send(ctx, terminal.SystemContent{Type: "system", Level: "error",
			Message: "room: send_message unavailable (no bus configured)"})
		return
	}
	if cmd.Params.To == "" || cmd.Params.Message == "" {
		_ = t.Send(ctx, terminal.SystemContent{Type: "system", Level: "error",
			Message: "room: send_message requires 'to' and 'message'"})
		return
	}

	evt := &engine.AgentMessageEvent{
		From:    "gui-user",
		To:      cmd.Params.To,
		Text:    cmd.Params.Message,
		Summary: cmd.Params.Summary,
	}
	evt.StampEvent(engine.AgentMessageTopic(cmd.Params.To), "gui-user")
	_ = r.bus.Publish(engine.AgentMessageTopic(cmd.Params.To), evt)
}

// handleCancelChild publishes a CancelRequestTopic event on the engine's bus
// on behalf of the GUI. The Spawner that owns the child subscribed to this
// topic at spawn time and reacts by calling its internal Cancel — which
// emits `to.<name>.cancel` and tears down the child's context. The GUI sees
// the resulting `child.<name>.failed` event via the existing translator
// subscription and updates the team card to "cancelled" state.
func (r *Room) handleCancelChild(ctx context.Context, t terminal.TerminalTransport, raw json.RawMessage) {
	var cmd terminal.CancelChildCmd
	if err := json.Unmarshal(raw, &cmd); err != nil {
		_ = t.Send(ctx, terminal.SystemContent{Type: "system", Level: "error",
			Message: fmt.Sprintf("room: invalid cancel_child: %v", err)})
		return
	}
	if r.bus == nil {
		_ = t.Send(ctx, terminal.SystemContent{Type: "system", Level: "error",
			Message: "room: cancel_child unavailable (no bus configured)"})
		return
	}
	if cmd.Params.RunID == "" {
		_ = t.Send(ctx, terminal.SystemContent{Type: "system", Level: "error",
			Message: "room: cancel_child requires 'runId'"})
		return
	}

	reason := cmd.Params.Reason
	if reason == "" {
		reason = "gui-cancel"
	}
	_ = r.bus.Publish(engine.CancelRequestTopic(cmd.Params.RunID), reason)
}

// handleCall executes a bridge call: routes a CallCmd through the KTSP sender.
func (r *Room) handleCall(ctx context.Context, t terminal.TerminalTransport, raw json.RawMessage) {
	var cmd terminal.CallCmd
	if err := json.Unmarshal(raw, &cmd); err != nil {
		_ = t.Send(ctx, terminal.SystemContent{Type: "system", Level: "error",
			Message: fmt.Sprintf("room: invalid call command: %v", err)})
		return
	}

	if r.sender == nil {
		_ = t.Send(ctx, terminal.SystemContent{Type: "system", Level: "error",
			Message: "room: bridge not configured"})
		return
	}

	args, _ := json.Marshal(cmd.Params.Args)
	result, err := r.sender.Call(ctx, cmd.Params.Path, json.RawMessage(args))
	if err != nil {
		_ = t.Send(ctx, terminal.SystemContent{Type: "system", Level: "error",
			Message: fmt.Sprintf("room: call %s: %v", cmd.Params.Path, err)})
		return
	}

	// Return raw result as a system message for now.
	// The transport layer will define a proper call-result event format.
	_ = t.Send(ctx, map[string]any{
		"type":   "call.result",
		"path":   cmd.Params.Path,
		"result": result,
	})
}

// ── Close ─────────────────────────────────────────────────────────────────

// Close shuts down the Room, stops all client goroutines, and closes all journals.
func (r *Room) Close() error {
	r.closeOnce.Do(func() {
		close(r.done)
		r.mu.Lock()
		defer r.mu.Unlock()
		for _, state := range r.sessions {
			_ = state.journal.Close()
		}
	})
	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────

func (r *Room) sendSync(ctx context.Context, t terminal.TerminalTransport) error {
	return r.sendSyncFrom(ctx, t, 0)
}

// sendSyncFrom sends a StateSyncEvent with messages starting from lastSeqNo.
// If lastSeqNo == 0, all messages are included (full sync).
func (r *Room) sendSyncFrom(ctx context.Context, t terminal.TerminalTransport, lastSeqNo int) error {
	r.mu.RLock()
	sessions := make([]terminal.Session, 0, len(r.sessions))
	var history []terminal.Message
	var panels []terminal.PanelEvent
	for _, state := range r.sessions {
		sessions = append(sessions, state.session)
		for _, msg := range state.messages {
			if msg.SeqNo > lastSeqNo {
				history = append(history, msg)
			}
		}
		if lastSeqNo == 0 {
			panels = append(panels, state.panels...)
		}
	}
	r.mu.RUnlock()

	return t.Send(ctx, terminal.StateSyncEvent{
		Type:     "state.sync",
		Sessions: sessions,
		History:  history,
		Panels:   panels,
	})
}

// asPanelEvent returns the chat panel event an event is, or false.
func asPanelEvent(event any) (terminal.PanelEvent, bool) {
	switch e := event.(type) {
	case terminal.PanelEvent:
		return e, true
	case *terminal.PanelEvent:
		return *e, true
	}
	return terminal.PanelEvent{}, false
}

// asMsgNew returns the Message from a MsgNewEvent, or false.
func asMsgNew(event any) (terminal.Message, bool) {
	switch e := event.(type) {
	case terminal.MsgNewEvent:
		return e.Msg, true
	case *terminal.MsgNewEvent:
		return e.Msg, true
	}
	return terminal.Message{}, false
}
