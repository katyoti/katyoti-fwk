// SPDX-License-Identifier: Apache-2.0

// Package translator converts system events (EventBus) into display-oriented
// terminal messages. It subscribes to agent events and pushes translated
// messages to a Room via AppendEvent (persisted + fan-out) or Broadcast
// (ephemeral streaming).
//
// Usage:
//
//	t := translator.New(bus, room, sessionID)
//	t.Start(ctx)
//	// ... agent runs, events flow through translator → room → clients
//	t.Stop()
package translator

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/core/usermsg"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	_ "github.com/katyoti/katyoti-fwk/internal/extensions/tools" // register built-in decorators
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal/chatpanel"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal/room"
)

// Translator converts agent EventBus events into terminal protocol messages.
type Translator struct {
	bus       eventbus.EventBus
	room      *room.Room
	sessionID string

	// strictSession, when true, owns ONLY runs that carry sessionID directly —
	// NOT their delegated descendants. The default (false) follows the whole run
	// tree, which is right for the single fullscreen TUI; the multi-pane console
	// sets it true so each run gets its OWN pane (a parent pane must not also
	// render its children, which have their own panes).
	strictSession bool

	mu     sync.Mutex
	seqNo  int
	unsubs []func()

	// Streaming text state — accumulates deltas until LLMResponse.
	currentTextID      string
	currentTextBuf     string
	currentTextTurn    int
	currentTextEmitter string // run id producing the in-flight text; tags the finalized msg.new with its source delegate

	// Tool call → message ID correlation for msg.update.
	toolMsgIDs map[string]string // tool call ID → terminal message ID

	// Prompt → message ID correlation for answer updates.
	promptMsgID string

	// Document ID → terminal message ID correlation.
	// The stream plugin uses the LLM tool_id as the document ID;
	// the translator maps it to a terminal message ID for msg.update.
	docMsgIDs     map[string]string // doc/tool ID → terminal message ID
	docTitles     map[string]string // doc ID → title (for fallback when DocumentEvent is missing)
	docMediaTypes map[string]string // doc ID → media type

	// chat is the session's conversation as its display panel of type chat
	// : the agent's UI artifact events and the text messages
	// become its panel events, sent as `panel.event` - the
	// one feed every chat draws a run's artifacts from -
	// and handed to panelSink when one is set.
	chat      *chatpanel.Bridge
	panelSink func(panel.Event)

	// agentNames maps a run ID (from agent.session_start.Emitter) to the
	// agent's display name, so we can recover the name on complete/error and
	// emit matching team.* done/failed events. topLevelRunID identifies the
	// root run for this translator (the one with no ParentRunID); used to
	// decide whether to also emit team.workflow_done / team.workflow_failed.
	agentNames    map[string]string
	topLevelRunID string

	// ownedRuns is the set of run ids whose events belong to THIS session — the
	// top-level run + its delegated descendants. The translator subscribes to
	// the GLOBAL EventBus, so without this gate a second parallel session's
	// events (prompts, stats, text) would leak into this one. Built from
	// task.started events (they carry correlation.sessionId + parentRunId).
	// `bootstrapped` flips true on the first task.started seen; until then
	// events pass (the brief window before any run has started has no display
	// events of its own). Guarded by t.mu.
	ownedRuns    map[string]bool
	bootstrapped bool

	// parentByRun maps a run id → its parent run id, learned from task.started
	// (which carries correlation.parentRunId). Used to stamp team.workflow_started
	// with a ParentRunID so the GUI can nest a sub-workflow instead of forking a
	// new root scope. task.started fires before the run's workflow.started, so the
	// parent is known in time. Guarded by t.mu.
	parentByRun map[string]string

	// Session telemetry for the GUI status bar (StatsEvent). Model +
	// provider captured on the top-level session_start; usage
	// accumulated across LLM responses. Guarded by t.mu.
	statModel         string
	statProvider      string
	statModels        []string // distinct models seen this session (multi-model = delegation across models)
	statUsage         agentapi.Usage
	statCostUSD       float64 // summed at-source cost (Usage.Cost) across all runs
	statCostKnown     bool    // any priced call seen
	statContextWindow int     // model context window (tokens)
	statCurrentInput  int     // latest request's input size (incl. cache)
	statTurns         int     // highest turn number seen
	statLastDuration  int64   // last response duration (ms)
	statLastTTFT      int64   // last response time-to-first-token (ms)
}

// New creates a Translator that will push messages to the given Room session.
func New(bus eventbus.EventBus, room *room.Room, sessionID string) *Translator {
	return &Translator{
		bus:           bus,
		room:          room,
		sessionID:     sessionID,
		toolMsgIDs:    make(map[string]string),
		docMsgIDs:     make(map[string]string),
		docTitles:     make(map[string]string),
		docMediaTypes: make(map[string]string),
	}
}

// NewSessionScoped is New but STRICT: the translator owns only the run(s)
// carrying sessionID directly, never their delegated children. The multi-pane
// console uses it so every run renders in its own pane without a parent pane
// duplicating its descendants' output.
func NewSessionScoped(bus eventbus.EventBus, room *room.Room, sessionID string) *Translator {
	t := New(bus, room, sessionID)
	t.strictSession = true
	return t
}

// Own marks runID as belonging to this translator and flips `bootstrapped`. A
// translator created REACTIVELY — after its run's task.started already fired
// (the multi-pane console makes a strict translator the moment it SEES a run
// start) — would otherwise never record its own run: pre-bootstrap it passes
// EVERY event (so every pane shows everything), and once another run's
// task.started flips bootstrapped it would drop its own. Seeding the owned run
// up front makes the scope correct from the first event. Caller must NOT hold
// t.mu.
func (t *Translator) Own(runID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.bootstrapped = true
	if t.ownedRuns == nil {
		t.ownedRuns = make(map[string]bool)
	}
	if runID != "" {
		t.ownedRuns[runID] = true
	}
}

// owns reports whether emitter's run belongs to this session. Empty emitter
// (pre-stamp / global audit events) passes. Before the first task.started is
// seen (`bootstrapped`), everything passes — the window before any run starts
// carries no display events. Caller must NOT hold t.mu.
func (t *Translator) owns(emitter string) bool {
	if emitter == "" {
		return true
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.bootstrapped {
		return true
	}
	return t.ownedRuns[emitter]
}

// scoped wraps a display-event handler so it runs ONLY for events whose run
// belongs to this session — the per-session isolation gate that stops a
// parallel session's events leaking into this translator.
func (t *Translator) scoped(h func(string, any)) func(string, any) {
	return func(topic string, payload any) {
		if ev, ok := payload.(agentapi.AgentEvent); ok && !t.owns(ev.GetEmitter()) {
			return
		}
		h(topic, payload)
	}
}

// onTaskStarted records run ids that belong to this session. A run is ours if
// it carries our session id, or its parent run already does (a delegated child
// inherits the parent's session). Framework-emitted before any of the run's
// display events, so the owned set is populated in time.
func (t *Translator) onTaskStarted(_ string, payload any) {
	e, ok := payload.(*engine.TaskStartedEvent)
	if !ok {
		if v, ok2 := payload.(engine.TaskStartedEvent); ok2 {
			e = &v
		} else {
			return
		}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.bootstrapped = true
	if t.ownedRuns == nil {
		t.ownedRuns = make(map[string]bool)
	}
	if t.parentByRun == nil {
		t.parentByRun = make(map[string]string)
	}
	t.parentByRun[e.RunID] = e.Correlation.ParentRunID
	owned := e.Correlation.SessionID == t.sessionID
	if !t.strictSession {
		// Default: a delegated child inherits the parent's session-ownership so
		// the single TUI shows the whole tree. Strict mode skips this so each run
		// stays isolated to its own pane.
		owned = owned || t.ownedRuns[e.Correlation.ParentRunID]
	}
	if owned {
		t.ownedRuns[e.RunID] = true
	}
}

// Start subscribes to all agent events and begins translation.
func (t *Translator) Start() {
	t.unsubs = append(t.unsubs,
		// task.started (unscoped) bootstraps the owned-run set; everything else
		// is scoped to this session's runs.
		t.bus.Subscribe(engine.EventTaskStarted, t.onTaskStarted),
		t.bus.Subscribe(agentapi.EventTextDelta, t.scoped(t.onTextDelta)),
		t.bus.Subscribe(agentapi.EventLLMResponse, t.scoped(t.onLLMResponse)),
		t.bus.Subscribe(agentapi.EventToolCall, t.scoped(t.onToolCall)),
		t.bus.Subscribe(agentapi.EventToolResult, t.scoped(t.onToolResult)),
		t.bus.Subscribe(agentapi.EventToolError, t.scoped(t.onToolError)),
		t.bus.Subscribe(agentapi.EventPromptShown, t.scoped(t.onPromptShown)),
		t.bus.Subscribe(agentapi.EventUserInput, t.scoped(t.onUserInput)),
		t.bus.Subscribe(agentapi.EventComplete, t.scoped(t.onComplete)),
		t.bus.Subscribe(agentapi.EventError, t.scoped(t.onError)),
		t.bus.Subscribe(agentapi.EventSessionStart, t.scoped(t.onSessionStart)),
		t.bus.Subscribe(agentapi.EventDocument, t.scoped(t.onDocument)),
		t.bus.Subscribe(agentapi.EventDocumentDelta, t.scoped(t.onDocumentDelta)),
		t.bus.Subscribe(agentapi.EventUIArtifactMount, t.scoped(t.onUIArtifactMount)),
		t.bus.Subscribe(agentapi.EventUIArtifactPatch, t.scoped(t.onUIArtifactPatch)),
		t.bus.Subscribe(agentapi.EventUIArtifactComplete, t.scoped(t.onUIArtifactComplete)),
		t.bus.Subscribe(agentapi.EventPanelShown, t.scoped(t.onPanelShown)),
		// Bridge ms08 task-engine topics → terminal protocol team.* events.
		// Uses SubscribeAny + topic-prefix filter so we don't have to enumerate
		// dynamic child names upfront. Cost per non-team Publish is one prefix
		// check; not a hot path in practice.
		t.bus.SubscribeAny(t.onMS08Event),
	)
}

// Stop unsubscribes from all events.
func (t *Translator) Stop() {
	for _, unsub := range t.unsubs {
		unsub()
	}
	t.unsubs = nil
}

// ── Text streaming ────────────────────────────────────────────────────────

func (t *Translator) onTextDelta(_ string, payload any) {
	e, ok := payload.(agentapi.TextDeltaEvent)
	if !ok {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	// Start new text message on first delta of a turn.
	if t.currentTextID == "" || t.currentTextTurn != e.Turn {
		t.currentTextID = t.newMsgID()
		t.currentTextBuf = ""
		t.currentTextTurn = e.Turn
		t.currentTextEmitter = e.GetEmitter()
	}
	t.currentTextBuf += e.Text

	// Broadcast ephemeral delta to connected clients (not stored).
	t.room.Broadcast(t.sessionID, terminal.MsgDeltaEvent{
		Type:    "msg.delta",
		Session: t.sessionID,
		ID:      t.currentTextID,
		Delta:   e.Text,
	})
}

func (t *Translator) onLLMResponse(_ string, payload any) {
	e, ok := payload.(agentapi.LLMResponseEvent)
	if !ok {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	// Accumulate token usage for the status bar regardless of whether
	// this turn produced text (a tool-only turn still consumes tokens).
	// Also track context fill (latest request input incl. cache),
	// turn count, and last-response latency.
	t.statCurrentInput = e.Usage.InputTokens + e.Usage.CacheReadInputTokens
	if e.Turn > t.statTurns {
		t.statTurns = e.Turn
	}
	t.statLastDuration = e.DurationMs
	t.statLastTTFT = e.TTFTMs
	t.accumulateUsage(e.Usage)

	if t.currentTextID == "" || t.currentTextBuf == "" {
		return // no text was streamed this turn
	}

	// Persist the final assembled text message.
	t.appendMsg(t.currentTextEmitter, terminal.Message{
		ID:        t.currentTextID,
		SessionID: t.sessionID,
		Role:      "assistant",
		Content:   terminal.Text(t.currentTextBuf),
	})

	// Signal streaming done.
	t.room.Broadcast(t.sessionID, terminal.MsgDoneEvent{
		Type:    "msg.done",
		Session: t.sessionID,
		ID:      t.currentTextID,
	})
	t.toChatPanel(t.chatPanel(t.currentTextEmitter).Message(chatpanel.RoleAssistant, t.currentTextID, t.currentTextBuf))

	t.currentTextID = ""
	t.currentTextBuf = ""
	t.currentTextEmitter = ""
}

// accumulateUsage adds one turn's usage to the running session total
// and broadcasts the cumulative StatsEvent for the GUI status bar.
// Called from onLLMResponse (already holds t.mu).
func (t *Translator) accumulateUsage(u agentapi.Usage) {
	t.statUsage.InputTokens += u.InputTokens
	t.statUsage.OutputTokens += u.OutputTokens
	t.statUsage.CacheReadInputTokens += u.CacheReadInputTokens
	t.statUsage.CacheCreationInputTokens += u.CacheCreationInputTokens
	// Cost AT SOURCE: sum the per-call cost the agent loop already priced from
	// each model's own rates. This is correct for multi-model sessions (a
	// delegating parent + a child on a different model) — the old recompute
	// applied ONE model's rate to the summed tokens. Track the distinct models
	// too so the status line can show "…" + the list when several were used.
	if u.Cost != nil {
		t.statCostUSD += u.Cost.TotalUSD
		t.statCostKnown = true
	}
	if u.Model != "" {
		t.statModel = u.Model
		seen := false
		for _, m := range t.statModels {
			if m == u.Model {
				seen = true
				break
			}
		}
		if !seen {
			t.statModels = append(t.statModels, u.Model)
		}
	}
	if u.Provider != "" {
		t.statProvider = u.Provider
	}
	t.broadcastStats()
}

// broadcastStats emits the current cumulative StatsEvent. Caller holds t.mu.
func (t *Translator) broadcastStats() {
	// Cost is the at-source sum (Usage.Cost across all runs) — priced per-model
	// with provider-specific cache rates, so it's correct for multi-model
	// sessions. session_start no longer carries any rate to fall back to.
	costUSD := t.statCostUSD
	costKnown := t.statCostKnown
	t.room.Broadcast(t.sessionID, terminal.StatsEvent{
		Type:                "stats",
		Session:             t.sessionID,
		Model:               t.statModel,
		Models:              t.statModels,
		Provider:            t.statProvider,
		InputTokens:         t.statUsage.InputTokens,
		OutputTokens:        t.statUsage.OutputTokens,
		CacheReadTokens:     t.statUsage.CacheReadInputTokens,
		CacheCreationTokens: t.statUsage.CacheCreationInputTokens,
		CostUSD:             costUSD,
		CostKnown:           costKnown,
		CurrentInputTokens:  t.statCurrentInput,
		ContextWindow:       t.statContextWindow,
		Turns:               t.statTurns,
		LastDurationMs:      t.statLastDuration,
		LastTTFTMs:          t.statLastTTFT,
	})
}

// ── Tool call lifecycle ───────────────────────────────────────────────────

func (t *Translator) onToolCall(_ string, payload any) {
	e, ok := payload.(agentapi.ToolCallEvent)
	if !ok {
		return
	}
	// Skip ToolContent for ask_user / ask-user-data — both surface via
	// PromptContent (the prompt banner / open-form button), so rendering
	// the tool call too would duplicate the display.
	if e.Tool == "ask-user" || e.Tool == "ask-user-data" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	// Flush any pending text before tool display.
	t.flushText()

	// Special case: delegate tool → DelegationContent.
	if e.Tool == "delegate" {
		t.handleDelegation(e)
		return
	}

	msgID := t.newMsgID()
	t.toolMsgIDs[e.ID] = msgID

	decorator := toolsapi.GetDecorator(e.Tool)
	view := decorator.OnCall(e.Input)
	display := &terminal.DisplayData{
		Icon:  view.Icon,
		Label: view.Label,
		Color: view.Color,
		Title: view.Title,
		Body:  view.Body,
	}
	t.appendMsg(e.GetEmitter(), terminal.Message{
		ID:        msgID,
		SessionID: t.sessionID,
		Role:      "tool",
		Content: terminal.NewContent(terminal.ToolContent{
			Type:     "tool",
			ToolName: e.Tool,
			Status:   "running",
			Input:    e.Input,
			Display:  display,
		}),
	})
}

func (t *Translator) onToolResult(_ string, payload any) {
	e, ok := payload.(agentapi.ToolResultEvent)
	if !ok {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	msgID, ok := t.toolMsgIDs[e.ID]
	if !ok {
		return // no matching tool call
	}
	delete(t.toolMsgIDs, e.ID)

	t.updateMsg(msgID, terminal.NewContent(terminal.ToolContent{
		Type:     "tool",
		ToolName: e.Tool,
		Status:   "success",
		Output:   e.Result,
		Duration: int(e.DurationMs),
	}))
}

func (t *Translator) onToolError(_ string, payload any) {
	e, ok := payload.(agentapi.ToolErrorEvent)
	if !ok {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	msgID, ok := t.toolMsgIDs[e.ID]
	if !ok {
		return
	}
	delete(t.toolMsgIDs, e.ID)

	t.updateMsg(msgID, terminal.NewContent(terminal.ToolContent{
		Type:     "tool",
		ToolName: e.Tool,
		Status:   "error",
		Error:    e.Error,
		Duration: int(e.DurationMs),
	}))
}

// ── Prompt / user input ───────────────────────────────────────────────────

func (t *Translator) onPromptShown(_ string, payload any) {
	e, ok := payload.(agentapi.PromptShownEvent)
	if !ok {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	t.flushText()

	msgID := t.newMsgID()
	t.promptMsgID = msgID

	// Empty question = "yield to user, wait for next message" (chat
	// keepalive idle signal). The
	// GUI renders a placeholder instead of an empty paragraph so the
	// banner stays informative.
	question := e.Question
	if question == "" {
		question = "Waiting for your next message…"
	}
	t.appendMsg(e.GetEmitter(), terminal.Message{
		ID:        msgID,
		SessionID: t.sessionID,
		Role:      "system",
		Content: terminal.NewContent(terminal.PromptContent{
			Type:       "prompt",
			PromptType: promptType(e.PromptType),
			Question:   question,
			Options:    e.Options,
			FormSchema: e.FormSchema,
			FormData:   e.FormData,
			Answered:   false,
			Channel:    e.Channel,
			ArtifactID: e.ArtifactID,
			Title:      e.Title,
		}),
	})
}

func (t *Translator) onUserInput(_ string, payload any) {
	e, ok := payload.(agentapi.UserInputEvent)
	if !ok {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.promptMsgID != "" {
		t.updateMsg(t.promptMsgID, terminal.NewContent(terminal.PromptContent{
			Type:     "prompt",
			Answered: true,
			Answer:   e.Answer,
		}))
		t.promptMsgID = ""
	}

	// The person's answer is their turn of the conversation: a message of
	// the chat panel.
	if e.Answer != "" {
		t.toChatPanel(t.chatPanel(e.GetEmitter()).Message(chatpanel.RoleUser, "user-"+t.newMsgID(), e.Answer))
	}
}

// ── Completion / errors ───────────────────────────────────────────────────

func (t *Translator) onComplete(_ string, payload any) {
	e, ok := payload.(agentapi.CompleteEvent)
	if !ok {
		return
	}
	t.mu.Lock()
	t.flushText()

	level, status, text := completeLine(e)
	t.appendMsg(e.GetEmitter(), terminal.Message{
		ID:        t.newMsgID(),
		SessionID: t.sessionID,
		Role:      "system",
		Content: terminal.NewContent(terminal.SystemContent{
			Type:    "system",
			Level:   level,
			Message: text,
		}),
	})
	t.mu.Unlock()

	_ = t.room.UpdateSessionStatus(t.sessionID, status)
	// A run's end leaves its journal with each chat panel node's last
	// replace once - what a reopened session replays.
	_ = t.room.CompactJournal(t.sessionID)

	// Bridge to GUI Team tab — close out this agent's card.
	t.bridgeAgentComplete(e.Emitter, e.Summary)
}

// completeLine is how a run's end reads in its chat: its level, the
// session's status and its line. A run ended in error says so - its error
// was said by its error event - and its session ends "error", never "done"
// ; a run stopped short of its work (its turn limit, its
// budget) is a warning.
func completeLine(e agentapi.CompleteEvent) (level, status, text string) {
	text = fmt.Sprintf("%s (turns: %d, duration: %s)", e.Summary, e.Turns, e.Duration)
	switch {
	case e.Status == "completed":
		return "success", "done", text
	case e.Status == "error" || e.Error != "":
		if e.Summary == "" {
			text = fmt.Sprintf("The run failed (turns: %d, duration: %s)", e.Turns, e.Duration)
		}
		return "error", "error", text
	}
	return "warning", "done", text
}

func (t *Translator) onError(_ string, payload any) {
	e, ok := payload.(agentapi.ErrorEvent)
	if !ok {
		return
	}
	t.mu.Lock()
	t.appendMsg(e.GetEmitter(), terminal.Message{
		ID:        t.newMsgID(),
		SessionID: t.sessionID,
		Role:      "system",
		Content: terminal.NewContent(terminal.SystemContent{
			Type:    "system",
			Level:   "error",
			Message: e.Error,
			Msg:     e.Msg,
		}),
	})
	t.mu.Unlock()

	// Bridge to GUI Team tab — mark this agent's card as failed.
	t.bridgeAgentError(e.Emitter, e.Error)
}

// ReportRunError shows why a run failed outside any agent — a task that could
// not start (an unknown config key, a missing tool), or that ended in error
// before its agent could say so — as a system error message in the session,
// so the client sees the reason instead of a run that just ends.
func (t *Translator) ReportRunError(msg string) {
	t.reportRunError(msg, nil)
}

// ReportRunFailure is ReportRunError for an error: its message key, when it
// carries one (a model whose key is not set), rides on the line for the GUI
// to say it in the person's language.
func (t *Translator) ReportRunFailure(err error) {
	if err == nil {
		return
	}
	var key *usermsg.Msg
	if m, ok := usermsg.Of(err); ok {
		key = &m
	}
	t.reportRunError(err.Error(), key)
}

func (t *Translator) reportRunError(msg string, key *usermsg.Msg) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.appendMsg("", terminal.Message{
		ID:        t.newMsgID(),
		SessionID: t.sessionID,
		Role:      "system",
		Content: terminal.NewContent(terminal.SystemContent{
			Type:    "system",
			Level:   "error",
			Message: msg,
			Msg:     key,
		}),
	})
}

// ── Document events ───────────────────────────────────────────────────────

// onDocumentDelta handles partial content arriving from the LLM tool-input
// stream. First delta per document creates the msg.new (with DocumentContent
// status="streaming"). Subsequent deltas emit msg.delta for progressive
// rendering.
func (t *Translator) onDocumentDelta(_ string, payload any) {
	e, ok := payload.(agentapi.DocumentDeltaEvent)
	if !ok {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	t.flushText()

	msgID, exists := t.docMsgIDs[e.ID]
	if !exists {
		// First delta — create the streaming message.
		msgID = t.newMsgID()
		t.docMsgIDs[e.ID] = msgID
		t.docTitles[e.ID] = e.Title
		t.docMediaTypes[e.ID] = e.MediaType

		t.appendMsg(e.GetEmitter(), terminal.Message{
			ID:        msgID,
			SessionID: t.sessionID,
			Role:      "assistant",
			Content: terminal.NewContent(terminal.DocumentContent{
				Type:     "document",
				Document: render.Document{MediaType: documentMediaType(e.MediaType), Title: e.Title, Content: ""}, // content builds up via deltas
				Status:   "streaming",
			}),
		})
	}

	// A document redrawn as it streams replaces its content: msg.update
	// with the whole document, still streaming.
	if e.Content != "" {
		t.room.Broadcast(t.sessionID, terminal.MsgUpdateEvent{
			Type:    "msg.update",
			Session: t.sessionID,
			ID:      msgID,
			Content: terminal.NewContent(terminal.DocumentContent{
				Type:     "document",
				Document: render.Document{MediaType: documentMediaType(t.docMediaTypes[e.ID]), Title: t.docTitles[e.ID], Content: e.Content},
				Status:   "streaming",
			}),
		})
	}

	// Emit msg.delta for the new chars. The frontend's appendDocumentDelta
	// action merges them into the document's content buffer.
	if e.Delta != "" {
		t.room.Broadcast(t.sessionID, terminal.MsgDeltaEvent{
			Type:    "msg.delta",
			Session: t.sessionID,
			ID:      msgID,
			Delta:   e.Delta,
		})
	}
}

// onDocument finalizes a document — fires on tool_use_done with the full
// content. If a streaming message already exists, update it to status="done".
// Otherwise (non-streaming path), create a fresh msg.new.
func (t *Translator) onDocument(_ string, payload any) {
	e, ok := payload.(agentapi.DocumentEvent)
	if !ok {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	t.flushText()

	msgID, streamed := t.docMsgIDs[e.ID]
	title := e.Title
	if title == "" {
		title = t.docTitles[e.ID]
	}
	mediaType := e.MediaType
	if mediaType == "" {
		mediaType = t.docMediaTypes[e.ID]
	}
	doc := render.Document{MediaType: documentMediaType(mediaType), Title: title, Content: e.Content}

	if streamed {
		// Streaming path: finalize with msg.update.
		delete(t.docMsgIDs, e.ID)
		delete(t.docTitles, e.ID)
		delete(t.docMediaTypes, e.ID)
		_ = t.room.AppendEvent(t.sessionID, terminal.MsgUpdateEvent{
			Type:    "msg.update",
			Session: t.sessionID,
			ID:      msgID,
			Content: terminal.NewContent(terminal.DocumentContent{
				Type:     "document",
				Document: doc,
				Status:   "done",
			}),
		})
		return
	}

	// Non-streaming path: create the complete message in one go.
	t.appendMsg(e.GetEmitter(), terminal.Message{
		ID:        t.newMsgID(),
		SessionID: t.sessionID,
		Role:      "assistant",
		Content: terminal.NewContent(terminal.DocumentContent{
			Type:     "document",
			Document: doc,
			Status:   "done",
		}),
	})
}

// documentMediaType is a streamed document's media type: markdown when the
// producer has not said (the partial tool input may not have reached it yet).
func documentMediaType(mt string) string {
	if mt == "" {
		return render.MediaTypeMarkdown
	}
	return mt
}

// ── UI artifacts (display-ui-component / patch-ui-component / ─────────────
//      complete-ui-component): the chat panel's input, its only feed.

// onUIArtifactMount shows the artifact the event mounts on the session's
// chat panel; the bridge answers nothing for an artifact already shown (the
// stream plugin may fire the mount again).
func (t *Translator) onUIArtifactMount(_ string, payload any) {
	e, ok := payload.(agentapi.UIArtifactMountEvent)
	if !ok {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	t.flushText()
	t.toChatPanel(t.chatPanel(e.GetEmitter()).Mount(e))
}

// onUIArtifactPatch applies the agent's ops to the artifact on the chat
// panel: one replace of the artifact's node.
func (t *Translator) onUIArtifactPatch(_ string, payload any) {
	e, ok := payload.(agentapi.UIArtifactPatchEvent)
	if !ok {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.chat != nil {
		t.toChatPanel(t.chat.Patch(e))
	}
}

// onUIArtifactComplete marks the artifact complete on the chat panel.
func (t *Translator) onUIArtifactComplete(_ string, payload any) {
	e, ok := payload.(agentapi.UIArtifactCompleteEvent)
	if !ok {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.chat != nil {
		t.toChatPanel(t.chat.Complete(e))
	}
}

// onPanelShown places a display panel in the conversation where it came
// : a question asked from the chat shows there as its card.
func (t *Translator) onPanelShown(_ string, payload any) {
	e, ok := payload.(agentapi.PanelShownEvent)
	if !ok {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	t.flushText()
	t.toChatPanel(t.chatPanel(e.GetEmitter()).Panel(e))
}

// chatPanel is the session's chat panel, made on its first artifact, owned
// by the run that mounted it. Caller holds t.mu.
func (t *Translator) chatPanel(runID string) *chatpanel.Bridge {
	if t.chat == nil {
		t.chat = chatpanel.New("", t.sessionID, runID)
	}
	t.chat.Own(runID)
	return t.chat
}

// UseChatPanel makes the session's chat panel now, with the id given (a
// fresh one when empty), before its first message or artifact - so its
// input can be listened to from the start - and hands each of its events
// to sink (nil: the room alone), beside the room's `panel.event`. It
// answers the panel's id. Caller must NOT hold t.mu.
func (t *Translator) UseChatPanel(id string, sink func(panel.Event)) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.chat == nil {
		t.chat = chatpanel.New(id, t.sessionID, "")
	}
	t.panelSink = sink
	return t.chat.ID()
}

// ChatMessage is the text of a message the person sent on the chat
// panel's input - the run's next user turn -; ok false for any other
// input. Caller must NOT hold t.mu.
func (t *Translator) ChatMessage(in panel.Input) (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.chat == nil {
		return "", false
	}
	return t.chat.MessageOf(in)
}

// toChatPanel sends the chat panel's events on the terminal protocol, in
// their order: journaled in the session's journal - so a reopened session
// still shows the artifacts - but for the replaces of an artifact still
// streaming, only sent (the completion carries where it ended). What the
// bridge refused goes no further. Caller holds t.mu.
func (t *Translator) toChatPanel(events []panel.Event, _ error) {
	for _, ev := range events {
		pe := terminal.PanelEvent{
			Type:      "panel.event",
			Session:   t.sessionID,
			Event:     ev,
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		}
		if chatpanel.Transient(ev) {
			t.room.Broadcast(t.sessionID, pe)
		} else {
			_ = t.room.AppendEvent(t.sessionID, pe)
		}
		if t.panelSink != nil {
			t.panelSink(ev)
		}
	}
}

// ChatPanelID is the id of the session's chat panel, "" before its first
// UI artifact.
func (t *Translator) ChatPanelID() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.chat == nil {
		return ""
	}
	return t.chat.ID()
}

// ── Session lifecycle ─────────────────────────────────────────────────────

func (t *Translator) onSessionStart(_ string, payload any) {
	e, ok := payload.(agentapi.SessionStartEvent)
	if !ok {
		return
	}
	// Capture the active model + provider for the status bar from the
	// top-level run (the one with no parent). Emit an initial
	// zero-token StatsEvent so the bar shows the real model
	// immediately, not the static default.
	if e.ParentRunID == "" {
		t.mu.Lock()
		t.statModel = e.Model
		t.statProvider = e.Provider
		t.statContextWindow = e.ContextWindow
		t.broadcastStats()
		t.mu.Unlock()
	}

	// Sub-agent sessions (background delegation) get a separate session in the Room.
	if e.ParentRunID != "" && e.Mode == "background" {
		subSess, err := t.room.CreateSession("", e.Name, "")
		if err == nil {
			_ = t.room.AppendEvent(t.sessionID, terminal.SessionCreatedEvent{
				Type:    "session.created",
				Session: *subSess,
			})
		}
	}

	// Bridge to GUI Team tab. Every agent run becomes a card. The translator
	// runs once per top-level run; background sub-agents publish their own
	// session_start on the same shared bus so they show up here too with the
	// parent's runID set.
	t.bridgeAgentSessionStart(e)
}

// bridgeAgentSessionStart emits the team.* events the GUI Team tab consumes.
// Without this, single-agent tasks (type: agent / type: task) never populate
// the team tab — only workflow-spawned children do, because the spawner is
// the only thing emitting child.<name>.* topics.
func (t *Translator) bridgeAgentSessionStart(e agentapi.SessionStartEvent) {
	runID := e.Emitter
	if runID == "" {
		return // no run identity → can't bridge
	}
	t.mu.Lock()
	if t.agentNames == nil {
		t.agentNames = make(map[string]string)
	}
	t.agentNames[runID] = e.Name
	if e.ParentRunID == "" {
		t.topLevelRunID = runID
	}
	t.mu.Unlock()

	ts := time.Now().UTC().Format(time.RFC3339Nano)

	// If this is the top-level run, also emit team.workflow_started so the
	// status bar at the top of the Team tab can show the active workflow.
	if e.ParentRunID == "" {
		t.room.Broadcast(t.sessionID, terminal.TeamWorkflowStartedEvent{
			Type:      "team.workflow_started",
			RunID:     runID,
			Name:      e.Name,
			Timestamp: ts,
		})
	}

	// Emit team.child_started for the agent itself. Top-level agents become
	// the root card; sub-agents (delegated background tasks) appear as
	// children of their ParentRunID.
	t.room.Broadcast(t.sessionID, terminal.TeamChildStartedEvent{
		Type:        "team.child_started",
		RunID:       runID,
		ParentRunID: e.ParentRunID,
		Name:        e.Name,
		Kind:        "agent",
		Timestamp:   ts,
	})
}

// bridgeAgentComplete fires team.child_done (and team.workflow_done if
// top-level) when an agent run finishes successfully. Uses the agentNames
// map to recover the name we recorded on session_start.
func (t *Translator) bridgeAgentComplete(runID string, summary string) {
	if runID == "" {
		return
	}
	t.mu.Lock()
	name := t.agentNames[runID]
	wasTopLevel := runID == t.topLevelRunID
	delete(t.agentNames, runID)
	t.mu.Unlock()

	ts := time.Now().UTC().Format(time.RFC3339Nano)
	t.room.Broadcast(t.sessionID, terminal.TeamChildDoneEvent{
		Type:      "team.child_done",
		RunID:     runID,
		Name:      name,
		Output:    map[string]any{"summary": summary},
		Timestamp: ts,
	})
	if wasTopLevel {
		t.room.Broadcast(t.sessionID, terminal.TeamWorkflowDoneEvent{
			Type:      "team.workflow_done",
			RunID:     runID,
			Output:    map[string]any{"summary": summary},
			Timestamp: ts,
		})
	}
}

// bridgeAgentError fires team.child_failed (and team.workflow_failed if
// top-level) when an agent run errors out.
func (t *Translator) bridgeAgentError(runID string, errMsg string) {
	if runID == "" {
		return
	}
	t.mu.Lock()
	name := t.agentNames[runID]
	wasTopLevel := runID == t.topLevelRunID
	delete(t.agentNames, runID)
	t.mu.Unlock()

	ts := time.Now().UTC().Format(time.RFC3339Nano)
	t.room.Broadcast(t.sessionID, terminal.TeamChildFailedEvent{
		Type:      "team.child_failed",
		RunID:     runID,
		Name:      name,
		Error:     errMsg,
		Timestamp: ts,
	})
	if wasTopLevel {
		t.room.Broadcast(t.sessionID, terminal.TeamWorkflowFailedEvent{
			Type:      "team.workflow_failed",
			RunID:     runID,
			Error:     errMsg,
			Timestamp: ts,
		})
	}
}

// ── Delegation ────────────────────────────────────────────────────────────

func (t *Translator) handleDelegation(e agentapi.ToolCallEvent) {
	var input struct {
		TaskName string `json:"task_name"`
		Mode     string `json:"mode"`
	}
	_ = json.Unmarshal(e.Input, &input)
	if input.Mode == "" {
		input.Mode = "worker"
	}

	msgID := t.newMsgID()
	t.toolMsgIDs[e.ID] = msgID

	t.appendMsg(e.GetEmitter(), terminal.Message{
		ID:        msgID,
		SessionID: t.sessionID,
		Role:      "tool",
		Content: terminal.NewContent(terminal.DelegationContent{
			Type:     "delegation",
			TaskName: input.TaskName,
			Mode:     input.Mode,
			Status:   "running",
		}),
	})
}

// ── ms08 task-engine topics → team.* events ───────────────────────────────

// onMS08Event filters bus events by topic prefix and translates the four
// ms08 topic families (workflow.*, child.*, to.*, group.*) into the team.*
// terminal-protocol events the GUI Team tab consumes. Non-matching events are
// ignored — the cost on the hot path is one prefix check.
func (t *Translator) onMS08Event(topic string, payload any) {
	switch {
	case strings.HasPrefix(topic, "workflow."):
		t.onWorkflowTopic(topic, payload)
	case strings.HasPrefix(topic, "child."):
		t.onChildTopic(topic, payload)
	case strings.HasPrefix(topic, "to."):
		t.onAgentMessageTopic(topic, payload)
	case strings.HasPrefix(topic, "group."):
		t.onGroupMessageTopic(topic, payload)
	case topic == engine.EventTeamEvent:
		t.onTeamEventTopic(payload)
	case strings.HasPrefix(topic, "call."):
		t.onCallTopic(topic, payload)
	case strings.HasPrefix(topic, "ledger."):
		t.onLedgerTopic(payload)
	}
}

// onLedgerTopic bridges ledger.<id>.{snapshot,complete} (the Magentic
// shared-state machine) to the GUI Team tab's Ledger lens. Each snapshot is the
// full state at a version; complete marks the terminal state.
func (t *Translator) onLedgerTopic(payload any) {
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	switch e := payload.(type) {
	case *engine.LedgerSnapshotEvent:
		t.room.Broadcast(t.sessionID, terminal.TeamLedgerEvent{
			Type: "team.ledger", RunID: e.Emitter, State: e.State, Version: e.Version, Timestamp: ts,
		})
	case *engine.LedgerCompleteEvent:
		t.room.Broadcast(t.sessionID, terminal.TeamLedgerEvent{
			Type: "team.ledger", RunID: e.Emitter, State: e.FinalState, Complete: true, Reason: e.Reason, Timestamp: ts,
		})
	}
}

// onTeamEventTopic bridges the flat team-lens narrative stream (the typed
// engine.TeamEvent payloads emitted by orchestration.TeamManager) to the
// GUI's team view.
func (t *Translator) onTeamEventTopic(payload any) {
	e, ok := payload.(*engine.TeamEvent)
	if !ok || e == nil {
		return
	}
	t.room.Broadcast(t.sessionID, terminal.TeamTeamEvent{
		Type:      "team.team_event",
		Action:    e.Action,
		TeamID:    e.TeamID,
		TeamName:  e.TeamName,
		State:     e.State,
		MemberID:  e.MemberID,
		RunID:     e.Correlation.RunID,
		ParentRun: e.Correlation.ParentRunID,
		Detail:    e.Detail,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	})
}

// onCallTopic bridges call.<name>.completed (the workflow `call`
// primitive's per-attempt observability row — NOT a child / team member).
func (t *Translator) onCallTopic(topic string, payload any) {
	if !strings.HasSuffix(topic, ".completed") {
		return
	}
	e, ok := payload.(*engine.CallCompletedEvent)
	if !ok || e == nil {
		return
	}
	t.room.Broadcast(t.sessionID, terminal.TeamCallCompletedEvent{
		Type:       "team.call_completed",
		Node:       e.Node,
		URI:        e.URI,
		RunID:      e.Correlation.RunID,
		DurationMs: e.DurationMs,
		Attempt:    e.Attempt,
		Outcome:    e.Outcome,
		Error:      e.Error,
		Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (t *Translator) onWorkflowTopic(topic string, payload any) {
	// topic: workflow.<runID>.{started,done,failed}
	parts := strings.SplitN(topic, ".", 3)
	if len(parts) != 3 {
		return
	}
	runID, suffix := parts[1], parts[2]
	ts := time.Now().UTC().Format(time.RFC3339Nano)

	switch suffix {
	case "started":
		var name string
		var inputs map[string]any
		if e, ok := payload.(*engine.WorkflowStartedEvent); ok && e != nil {
			name = e.Name
			inputs = e.Inputs
		}
		t.mu.Lock()
		parentRunID := t.parentByRun[runID]
		t.mu.Unlock()
		t.room.Broadcast(t.sessionID, terminal.TeamWorkflowStartedEvent{
			Type:        "team.workflow_started",
			RunID:       runID,
			ParentRunID: parentRunID,
			Name:        name,
			Inputs:      inputs,
			Timestamp:   ts,
		})
	case "done":
		var output any
		if e, ok := payload.(*engine.WorkflowDoneEvent); ok && e != nil {
			output = e.Output
		}
		t.room.Broadcast(t.sessionID, terminal.TeamWorkflowDoneEvent{
			Type:      "team.workflow_done",
			RunID:     runID,
			Output:    output,
			Timestamp: ts,
		})
	case "failed":
		var errMsg string
		if e, ok := payload.(*engine.WorkflowFailedEvent); ok && e != nil {
			errMsg = e.Error
		}
		t.room.Broadcast(t.sessionID, terminal.TeamWorkflowFailedEvent{
			Type:      "team.workflow_failed",
			RunID:     runID,
			Error:     errMsg,
			Timestamp: ts,
		})
	}
}

func (t *Translator) onChildTopic(topic string, payload any) {
	// topic: child.<name>.{started,done,failed,suspended,progress}
	// Name may contain dots if the workflow author chose so; we conservatively
	// take the LAST segment as the suffix and everything between as the name.
	idx := strings.LastIndex(topic, ".")
	if idx < 0 || idx <= len("child.") {
		return
	}
	suffix := topic[idx+1:]
	name := topic[len("child."):idx]
	ts := time.Now().UTC().Format(time.RFC3339Nano)

	switch suffix {
	case "started":
		var runID, parentRunID, kind, mode string
		if e, ok := payload.(*engine.ChildStartedEvent); ok && e != nil {
			runID = e.RunID
			parentRunID = e.ParentRunID
			kind = e.ChildKind
			mode = e.Mode
		}
		t.room.Broadcast(t.sessionID, terminal.TeamChildStartedEvent{
			Type:        "team.child_started",
			RunID:       runID,
			ParentRunID: parentRunID,
			Name:        name,
			Kind:        kind,
			Mode:        mode,
			Timestamp:   ts,
		})
	case "done":
		var runID string
		var output any
		if e, ok := payload.(*engine.ChildDoneEvent); ok && e != nil {
			runID = e.RunID
			output = e.Output
		}
		t.room.Broadcast(t.sessionID, terminal.TeamChildDoneEvent{
			Type:      "team.child_done",
			RunID:     runID,
			Name:      name,
			Output:    output,
			Timestamp: ts,
		})
	case "failed":
		var runID, errMsg string
		if e, ok := payload.(*engine.ChildFailedEvent); ok && e != nil {
			runID = e.RunID
			errMsg = e.Error
		}
		t.room.Broadcast(t.sessionID, terminal.TeamChildFailedEvent{
			Type:      "team.child_failed",
			RunID:     runID,
			Name:      name,
			Error:     errMsg,
			Timestamp: ts,
		})
	case "suspended":
		// A child asked its parent a question (comms channel). Render it in the
		// PARENT's pane — the parent is the one being asked. The child's own pane
		// shows its native stream (an agent's prompt; a simulator's says).
		if e, ok := payload.(*engine.ChildSuspendedEvent); ok && e != nil && e.ParentRunID != "" && t.owns(e.ParentRunID) {
			who := e.ChildName
			if who == "" {
				who = name
			}
			t.appendConvoLine("assistant", fmt.Sprintf("❓ %s asks: %s", who, e.Question), e.ParentRunID)
		}
	case "progress":
		// One-way conversation line (a say / a delivered answer). Render it in the
		// emitting run's OWN pane.
		if e, ok := payload.(*engine.ChildProgressEvent); ok && e != nil && e.Message != "" && e.RunID != "" && t.owns(e.RunID) {
			t.appendConvoLine("assistant", e.Message, e.RunID)
		}
	}
}

// appendConvoLine renders a comms-channel line (say / question / answer) as a
// chat message in this translator's session. Reuses msg.new so both the console
// chatModel and the GUI chat view display it with no new event type — the
// generic surfacing of comm-channel activity for ANY task type (a simulator
// conversation looks just like an agent one).
func (t *Translator) appendConvoLine(role, text, sourceRun string) {
	t.appendMsg(sourceRun, terminal.Message{
		ID:        t.newMsgID(),
		SessionID: t.sessionID,
		Role:      role,
		Content:   terminal.Text(text),
	})
}

func (t *Translator) onAgentMessageTopic(topic string, payload any) {
	// topic: to.<name>.message → direct message
	parts := strings.SplitN(topic, ".", 3)
	if len(parts) != 3 || parts[2] != "message" {
		return
	}
	to := parts[1]
	t.broadcastTeamMessage(payload, to, "")
}

func (t *Translator) onGroupMessageTopic(topic string, payload any) {
	// topic: group.<team>.message or group.<team>.announcement
	parts := strings.SplitN(topic, ".", 3)
	if len(parts) != 3 {
		return
	}
	if parts[2] != "message" && parts[2] != "announcement" {
		return
	}
	team := parts[1]
	t.broadcastTeamMessage(payload, "", team)
}

func (t *Translator) broadcastTeamMessage(payload any, to, team string) {
	var from, message, summary string
	if e, ok := payload.(*engine.AgentMessageEvent); ok && e != nil {
		from = e.From
		message = e.Text
		summary = e.Summary
	}
	t.room.Broadcast(t.sessionID, terminal.TeamMessageEvent{
		Type:      "team.message",
		From:      from,
		To:        to,
		Team:      team,
		Message:   message,
		Summary:   summary,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	})
}

// ── Helpers ───────────────────────────────────────────────────────────────

// flushText persists any accumulated streaming text (called before tool/prompt events).
func (t *Translator) flushText() {
	if t.currentTextID == "" || t.currentTextBuf == "" {
		return
	}
	t.appendMsg(t.currentTextEmitter, terminal.Message{
		ID:        t.currentTextID,
		SessionID: t.sessionID,
		Role:      "assistant",
		Content:   terminal.Text(t.currentTextBuf),
	})
	t.room.Broadcast(t.sessionID, terminal.MsgDoneEvent{
		Type:    "msg.done",
		Session: t.sessionID,
		ID:      t.currentTextID,
	})
	t.toChatPanel(t.chatPanel(t.currentTextEmitter).Message(chatpanel.RoleAssistant, t.currentTextID, t.currentTextBuf))
	t.currentTextID = ""
	t.currentTextBuf = ""
	t.currentTextEmitter = ""
}

func (t *Translator) appendMsg(sourceRun string, msg terminal.Message) {
	t.seqNo++
	msg.SeqNo = t.seqNo
	msg.SourceRun = sourceRun
	if msg.Timestamp == "" {
		msg.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	_ = t.room.AppendEvent(t.sessionID, terminal.MsgNewEvent{
		Type:    "msg.new",
		Session: t.sessionID,
		SeqNo:   msg.SeqNo,
		Msg:     msg,
	})
}

func (t *Translator) updateMsg(msgID string, content terminal.MessageContent) {
	_ = t.room.AppendEvent(t.sessionID, terminal.MsgUpdateEvent{
		Type:    "msg.update",
		Session: t.sessionID,
		ID:      msgID,
		Content: content,
	})
}

func (t *Translator) newMsgID() string {
	return "msg-" + uuid.New().String()[:8]
}

func promptType(pt string) string {
	if pt == "" {
		return "text"
	}
	return pt
}
