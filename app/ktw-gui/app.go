// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/app/ktw-gui/internal/supervisor"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/core/usermsg"
	terminalapi "github.com/katyoti/katyoti-fwk/internal/extensions/terminal/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the Wails-bound Go backend. All exported methods become JS bindings.
//
// Task execution is universe-aware: tasks run inside the displayed universe's
// long-lived ktw-agents core via the KTSP Terminal service. There is no
// legacy stdio/connect/mock dichotomy at the GUI process boundary — transport
// is decided by each sidecar's `transport:` field in Univers.yaml.
type App struct {
	ctx context.Context

	mu sync.Mutex
	// runs holds every in-flight run, keyed by its journal session id —
	// the same id the frontend binds each chat tab to. Replaces the old
	// single-task `transport`/`cancel`/`running` triple so multiple chats
	// run concurrently (gui-parallel-chat-sessions P1).
	runs map[string]*guiRun
	// focusedSessionID is the chat the user is currently looking at. The
	// single-transport command bindings (AnswerPrompt / SendUIEvent /
	// SendTeamMessage / CancelChild) target this run. Set by the frontend
	// via SetFocusedSession on tab focus.
	focusedSessionID string

	// master holds the active-universes map for this ktw-gui process —
	// one ktw-agents core per active universe. Activated by the user via
	// the IconRibbon context menu; torn down on Deactivate / app shutdown.
	master *supervisor.Master

	// initialUniverse is the universe to open on launch, if any — set from
	// the `--universe` launch flag (the katyoti launcher resolves it from
	// the flag or the startup default). Empty = boot to the welcome picker.
	// The frontend consults InitialUniverse() once, after loading the list.
	initialUniverse string

	// initialOpen is the address to open on launch (`--open`, a katyoti://
	// link), its universe being initialUniverse; "" = none.
	initialOpen string
	// noWelcome is this start's --no-welcome / KTW_NO_WELCOME.
	noWelcome bool

	// person is the cached handle of the person using this GUI: the cause
	// every call to a universe core carries (app_person_cause.go).
	person personCause

	// panels are the display panels the screen follows (
	// app_display_panels.go).
	panels panelFollows
}

// guiRun is one in-flight run tracked by the GUI: its journal session id,
// cancel func, and the terminal transport used to send commands into it.
type guiRun struct {
	sessionID string
	cancel    context.CancelFunc
	transport terminal.TerminalTransport
}

func NewApp(initialUniverse string) *App {
	return &App{
		master:          supervisor.NewMaster(),
		runs:            map[string]*guiRun{},
		initialUniverse: initialUniverse,
	}
}

// InitialUniverse returns the universe the GUI was launched to open (from the
// `--universe` launch flag), or "" to boot to the welcome picker. The frontend
// calls this once at boot (after UniverseList) and, when non-empty and present
// in the registry, displays that universe instead of the welcome banner.
//
// Wails-bound: window.go.main.App.InitialUniverse().
func (a *App) InitialUniverse() string {
	return a.initialUniverse
}

// registerRun records a started run under its session id and marks it focused
// (the newest run is what the user is looking at).
func (a *App) registerRun(sessionID string, cancel context.CancelFunc, transport terminal.TerminalTransport) {
	if sessionID == "" {
		return
	}
	a.mu.Lock()
	a.runs[sessionID] = &guiRun{sessionID: sessionID, cancel: cancel, transport: transport}
	a.focusedSessionID = sessionID
	a.mu.Unlock()
}

// unregisterRun drops a finished run. If it was focused, focus falls back to
// any remaining run so the single-target command bindings still hit a live one.
func (a *App) unregisterRun(sessionID string) {
	a.mu.Lock()
	delete(a.runs, sessionID)
	if a.focusedSessionID == sessionID {
		a.focusedSessionID = ""
		for sid := range a.runs {
			a.focusedSessionID = sid
			break
		}
	}
	a.mu.Unlock()
}

// transportFor returns a specific run's transport (parallel-chat targeting).
// Empty sessionID → the focused run (activeTransport). nil when not found.
func (a *App) transportFor(sessionID string) terminal.TerminalTransport {
	if sessionID == "" {
		return a.activeTransport()
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if r := a.runs[sessionID]; r != nil {
		return r.transport
	}
	return nil
}

// activeTransport returns the focused run's transport (or any run's, as a
// fallback) for the single-target command bindings. nil when nothing runs.
func (a *App) activeTransport() terminal.TerminalTransport {
	a.mu.Lock()
	defer a.mu.Unlock()
	if r := a.runs[a.focusedSessionID]; r != nil {
		return r.transport
	}
	for _, r := range a.runs {
		return r.transport
	}
	return nil
}

// SetFocusedSession marks which chat the user is viewing so AnswerPrompt and
// the other single-target commands route to that run. Wails-bound; the
// frontend calls it on chat-tab focus.
func (a *App) SetFocusedSession(sessionID string) {
	a.mu.Lock()
	if _, ok := a.runs[sessionID]; ok {
		a.focusedSessionID = sessionID
	}
	a.mu.Unlock()
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// The admin universe bootstrap is NO LONGER done here: ktw-gui is a pure
	// KTSP client ((modules/univers/rules/R-001-the-hub-owns-management-state-clients-are-pure.cwm.md)) and writes no management
	// state itself. The katyoti launcher ensures admin exists before spawning
	// the GUI (LaunchGUI → admin.EnsureDefault), and the ktw-univers core
	// self-heals it on boot. Activating admin below therefore just connects.

	// Forward CWM object events (writes from ANY process on the universe's
	// bus — CLI, agent runs, other GUI windows, this one) to the frontend,
	// with the object they are about: the tree re-fetches, and a shown
	// declared panel re-reads when the object is in its refresh scope
	// (usePanelRefresh).
	if a.master != nil {
		a.master.SetWorkspaceEventHandler(func(ev supervisor.WorkspaceEvent) {
			wailsRuntime.EventsEmit(a.ctx, "workspace.changed", ev)
		})
		// A universe created, deleted or edited ANYWHERE -- another GUI, a
		// terminal running `katyoti universe create` -- reaches the ribbon here.
		// No payload: the frontend re-reads the registry, so it cannot render a
		// stale view carried in the event.
		a.master.SetUniversesEventHandler(func() {
			wailsRuntime.EventsEmit(a.ctx, "universes.changed")
		})
		// The profile set from `katyoti profile` (or another GUI) reaches the
		// profile menu here; the frontend re-reads it.
		a.master.SetProfileEventHandler(func() {
			a.forgetPerson() // the next call reads who the person now is
			wailsRuntime.EventsEmit(a.ctx, "profile.changed")
		})
		// A universe's catalog reloaded (a sidecar wired, rebuilt or edited):
		// the frontend re-reads what it derives from it — the contribution
		// translation bundle (i18n/contrib.ts).
		a.master.SetCatalogEventHandler(func(universe string) {
			wailsRuntime.EventsEmit(a.ctx, "catalog.changed", map[string]string{"universe": universe})
		})
		// One of a universe's runs changed state: the Runs pane refreshes.
		a.master.SetRunsEventHandler(func(universe string, event json.RawMessage) {
			wailsRuntime.EventsEmit(a.ctx, "runs.changed", map[string]any{"universe": universe, "event": event})
		})
	}
}

// LogFromFrontend writes a log line from the React/iframe layer to the
// GUI's log file. Use this for diagnostics that can't be surfaced via
// DevTools (production Wails builds detach the inspector). The frontend
// calls `window.go.main.App.LogFromFrontend('iframe-widget', 'message')`.
func (a *App) LogFromFrontend(tag, message string) {
	log.Printf("[frontend][%s] %s", tag, message)
}

// shutdown tears down all active universe cores so child ktw-agents
// processes don't outlive the GUI window. Called by Wails on app exit.
func (a *App) shutdown(_ context.Context) {
	a.panels.stopAll()
	if a.master != nil {
		a.master.Close()
	}
}

// TaskInfo is a single task entry returned by ListTasksDetail.
type TaskInfo struct {
	Name string `json:"name"`
	// Title is the human-friendly display label; the frontend falls back to
	// Name when empty. Name stays the runnable id (shown in small chars +
	// the `ktw-agents run <name>` argument).
	Title       string `json:"title,omitempty"`
	Provider    string `json:"provider,omitempty"`
	URI         string `json:"uri,omitempty"`
	Capability  string `json:"capability"`
	Description string `json:"description"`
	// Body is the task's markdown body (the rich description shown behind the
	// card's "?" affordance). Empty when the task has no body.
	Body     string   `json:"body,omitempty"`
	Type     string   `json:"type"`
	Icon     string   `json:"icon,omitempty"`
	IconName string   `json:"iconName,omitempty"`
	Hashtags []string `json:"hashtags,omitempty"`
	Category string   `json:"category,omitempty"`
	Tags     []string `json:"tags,omitempty"`

	// Targeting metadata (ms-action-system): a task self-declares which scope
	// it attaches to. The GLOBAL gallery shows only kind:global/unset — a
	// kind:object/collection task needs a target to run, so it surfaces on
	// that object or type list, not the global launcher.
	Kind  string   `json:"kind,omitempty"`
	Types []string `json:"types,omitempty"`
	// Audience says whom the task is for: end-user | technical | demo; empty
	// is undeclared. A tagged universe's first level keeps the end-user tasks.
	Audience string `json:"audience,omitempty"`

	// Canonical schema-resource URIs for this task's input + output
	// data shapes (per ms-schema-driven-ui US-S1 D1''). Empty when the
	// task has no input-schema / output-schema block. The frontend
	// passes these to `GetSchemaAST` to fetch the parsed AST and
	// render the form via the @ktw/display-lib WidgetPanel.
	InputSchemaURI  string `json:"inputSchemaURI,omitempty"`
	OutputSchemaURI string `json:"outputSchemaURI,omitempty"`
}

// ListTasks / ListTasksDetail moved to app_listings.go — they now call the
// active universe's core via KTSP instead of shelling out.

// RunTaskInUniverse runs the named task inside the named universe's long-
// lived ktw-agents core via the KTSP Terminal service. Replaces runStdio
// for the universe-aware path: tasks run with the right KTW_WORKSPACE + sidecars
// + skills, multiplexed over the core's persistent KTSP session.
//
// universeName must reference a currently-active universe. The frontend
// should pass the displayed universe; if no universe is active the call
// errors and surfaces the message in the Wails error event.
//
// Wails-bound: window.go.main.App.RunTaskInUniverse(universeName, taskName, userInput).
func (a *App) RunTaskInUniverse(universeName, taskName, userInput string) {
	// Concurrent runs (gui-parallel-chat-sessions P1): no single-task guard.
	// Each run registers under its own journal session id once started.
	log.Printf("[RunTaskInUniverse] starting universe=%q task=%q", universeName, taskName)
	go a.runViaUniverse(universeName, taskName, userInput, nil)
}

// RunTaskInUniverseWithContext is the context-envelope variant of
// RunTaskInUniverse. Same control flow + same single-task-at-a-time
// guard; additionally threads `pluginContext` through to
// `terminalapi.StartTaskInput.Context` → AgentRunRequest.Context →
// `WithPluginContext(ctx, ...)`. Plugins read this via
// `agentapi.PluginContextFrom(ctx)`.
//
// Used by useChatLauncher to inject `{_target: activeNodeId}`
// when the user opens chat with a workspace node selected — the cwm
// plugin resolves the object's agent context and prepends the
// `<cwm_object_context>` system-prompt block. Generic enough that
// future surfaces (per-item chat, per-skill agent runs) can reuse it
// without adding more bindings.
//
// Wails-bound: window.go.main.App.RunTaskInUniverseWithContext(universeName, taskName, userInput, pluginContext).
func (a *App) RunTaskInUniverseWithContext(universeName, taskName, userInput string, pluginContext map[string]any) {
	// Concurrent runs (P1): no single-task guard.
	log.Printf("[RunTaskInUniverseWithContext] starting universe=%q task=%q ctxKeys=%d", universeName, taskName, len(pluginContext))
	go a.runViaUniverse(universeName, taskName, userInput, pluginContext)
}

// StopTask signals a running session to stop gracefully. With an empty
// sessionID it stops the focused run (back-compat for callers that don't
// track ids); otherwise it stops exactly that session.
func (a *App) StopTask(sessionID string) {
	a.mu.Lock()
	if sessionID == "" {
		sessionID = a.focusedSessionID
	}
	r := a.runs[sessionID]
	a.mu.Unlock()
	log.Printf("[StopTask] sessionID=%q found=%v", sessionID, r != nil)
	if r != nil && r.cancel != nil {
		r.cancel()
	}
}

// SaveDocument prompts the user with a native Save dialog and writes the
// document content to the chosen path. Returns the saved path (empty if the
// user cancelled) or an error message string. Called from the GUI's Save
// button on document tabs. The extension and dialog filter come from the
// media type (render.ExtensionFor); base64 content is written as bytes.
func (a *App) SaveDocument(title, mediaType, encoding, content string) (string, error) {
	// Sanitize title into a default filename.
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == ' ', r == '-', r == '_':
			return '-'
		default:
			return -1
		}
	}, strings.TrimSuffix(title, render.ExtensionFor(mediaType)))
	if safe == "" {
		safe = "document"
	}

	// The extension and the dialog filter come from the one media-type table.
	ext := render.ExtensionFor(mediaType)
	defaultFilename := safe + ext
	filter := wailsRuntime.FileFilter{DisplayName: render.BaseMediaType(mediaType) + " (*" + ext + ")", Pattern: "*" + ext}

	path, err := wailsRuntime.SaveFileDialog(a.ctx, wailsRuntime.SaveDialogOptions{
		DefaultFilename: defaultFilename,
		Title:           "Save document",
		Filters:         []wailsRuntime.FileFilter{filter, {DisplayName: "All files", Pattern: "*.*"}},
	})
	if err != nil {
		return "", fmt.Errorf("save dialog: %w", err)
	}
	if path == "" {
		// User cancelled.
		return "", nil
	}

	data := []byte(content)
	if encoding == "base64" {
		if data, err = base64.StdEncoding.DecodeString(content); err != nil {
			return "", fmt.Errorf("decode base64 content: %w", err)
		}
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}
	return path, nil
}

// AnswerPrompt routes the user's answer to a SPECIFIC run's transport
// (parallel chats: the answer must reach the chat the prompt came from, not
// whatever happens to be focused). Empty sessionID falls back to the focused
// run. Wails sig changed: AnswerPrompt(sessionID, id, response).
func (a *App) AnswerPrompt(sessionID string, id string, response string) {
	t := a.transportFor(sessionID)
	if t != nil {
		_ = t.Send(context.Background(), terminal.AnswerCmd{
			Type:   "answer",
			Params: terminal.AnswerOpt{ID: id, Value: response},
		})
	}
}

// SendUIEvent forwards a user-interaction event from the GUI's artifact
// renderer back to the agent. Used by the gui-artifact-renderer (ms07b)
// for `submit` (form data), `change` (live edit), `click` (action), and
// `navigate` (deep link) events on a streamed artifact.
//
// The agent's side of the artifact (the tool that mounted it, through the
// terminal room) processes the event. `eventType` discriminates
// the payload semantics; `valueJSON` is a JSON-encoded payload that
// the agent decodes per the artifact's schema.
//
// Wired by llm-streaming-protocol (ms06, 2026-05-04).
func (a *App) SendUIEvent(artifactID string, eventType string, valueJSON string) {
	t := a.activeTransport()
	if t == nil {
		return
	}
	var raw json.RawMessage
	if valueJSON != "" {
		raw = json.RawMessage(valueJSON)
	}
	_ = t.Send(context.Background(), terminal.UIEventCmd{
		Type: "ui_event",
		Params: terminal.UIEventOpt{
			ArtifactID: artifactID,
			EventType:  eventType,
			Value:      raw,
		},
	})
}

// SendTeamMessage sends a direct message to a teammate via the terminal
// protocol's SendMessageCmd. The room dispatcher publishes an
// AgentMessageEvent on the bus with sender "gui-user". Used by the Team tab
// MailboxPanel's send button.
func (a *App) SendTeamMessage(to string, message string) {
	t := a.activeTransport()
	if t != nil {
		_ = t.Send(context.Background(), terminal.SendMessageCmd{
			Type: "send_message",
			Params: terminal.SendMessageOpt{
				To:      to,
				Message: message,
			},
		})
	}
}

// CancelChild requests cancellation of a running spawned child by its run ID.
// Used by the Team tab's per-card Cancel button. The room dispatcher publishes
// CancelRequestTopic(runID) on the bus; the Spawner that owns the child
// subscribed to that topic at spawn time and reacts by terminating the child's
// context. The GUI sees the resulting `child.<name>.failed` event via the
// existing translator subscription and updates the team card to "cancelled".
//
// childName is informational (used for confirmation modal copy + logs); only
// runID is required to actually cancel the right child.
func (a *App) CancelChild(runID string, childName string) {
	t := a.activeTransport()
	if t != nil {
		_ = t.Send(context.Background(), terminal.CancelChildCmd{
			Type: "cancel_child",
			Params: terminal.CancelChildOpt{
				RunID:     runID,
				ChildName: childName,
				Reason:    "gui-cancel",
			},
		})
	}
}

// runViaUniverse drives a task through the displayed universe's long-lived
// ktw-agents core via the KTSP Terminal service.
//
// This is the universe-aware replacement for runStdio: instead of spawning
// a fresh ktw-agents subprocess per task (legacy single-tenant), the task
// runs inside the universe's core (with KTW_WORKSPACE + sidecars + skills set
// correctly). Multiple tasks can run concurrently in the same universe;
// they multiplex over the core's KTSP session.
//
// Falls back with an error if the universe isn't active (the user must
// activate it via the IconRibbon first). The frontend should ensure this
// can't happen — chat is only enabled when a universe is displayed.
// runViaUniverse drives the StartTaskRun KTSP call against the
// universe's long-lived core and bridges the resulting event stream
// to Wails events. The optional `pluginContext` carries the plugin-
// context envelope (see terminalapi.StartTaskInput.Context godoc) —
// pass nil when no envelope is needed (matches the pre-Phase-3
// signature).
func (a *App) runViaUniverse(universeName, taskName, userInput string, pluginContext map[string]any) {
	log.Printf("[runViaUniverse] step1 universe=%q task=%q ctxKeys=%d", universeName, taskName, len(pluginContext))
	ctx, cancel := context.WithCancel(a.ctx)
	// sessionID is filled in once StartTaskRun returns it; the deferred
	// cleanup unregisters the run under that id.
	var sessionID string
	defer func() { a.cleanup(cancel, sessionID) }()

	a.mu.Lock()
	master := a.master
	a.mu.Unlock()
	log.Printf("[runViaUniverse] step3 a.mu released; master=%v", master != nil)
	if master == nil || !master.IsActive(universeName) {
		log.Printf("[runViaUniverse] universe %q is not active (master=%v)", universeName, master != nil)
		a.emitRunError(taskName, fmt.Sprintf("universe %q is not active — activate it via the IconRibbon first", universeName))
		return
	}

	log.Printf("[runViaUniverse] step4 calling master.Activate")
	core, err := master.Activate(ctx, universeName) // idempotent fast-path when already active
	if err != nil {
		log.Printf("[runViaUniverse] Activate %q failed: %v", universeName, err)
		a.emitRunError(taskName, fmt.Sprintf("activate %q: %v", universeName, err))
		return
	}
	log.Printf("[runViaUniverse] step5 Activate returned ok")

	log.Printf("[runViaUniverse] step6 getting TerminalRouter")
	router := core.TerminalRouter()
	log.Printf("[runViaUniverse] step7 TerminalRouter ok")
	input := map[string]any{}
	if userInput != "" {
		input["user_input"] = userInput
	}
	// Every GUI-initiated run is ATTENDED — the user is sitting in the
	// chat/run surface right now. Flag it so ask-user resolves to the
	// `direct` (inline) channel even for tasks that don't declare
	// mode:chat, instead of falling back to the system-spawned Inbox.
	// A task's explicit `user_comms_input:` still wins (see
	// resolveFromExecRequest). Reserved `_`-prefixed substrate key.
	if pluginContext == nil {
		pluginContext = map[string]any{}
	}
	pluginContext["_attended"] = true

	log.Printf("[runViaUniverse] step8 StartTaskRun task=%q", taskName)
	transport, runID, sid, err := router.StartTaskRun(ctx, terminalapi.StartTaskInput{
		TaskName:    taskName,
		Input:       input,
		Context:     pluginContext,
		ClientLabel: "ktw-gui",
	})
	if err != nil {
		log.Printf("[runViaUniverse] StartTaskRun failed: %v", err)
		a.emitRunError(taskName, fmt.Sprintf("start task %q in %q: %v", taskName, universeName, err))
		return
	}
	sessionID = sid
	log.Printf("[runViaUniverse] step9 StartTaskRun ok runID=%s sessionID=%s; entering bridgeEvents", runID, sessionID)

	// Surface the server-pre-computed journal session id to the frontend
	// so the Output tab in ChatPanel (and any other consumer) can hit
	// the journal/output endpoints immediately, without waiting for the
	// task to terminate and guessing by task-name + recency.
	if sessionID != "" {
		wailsRuntime.EventsEmit(a.ctx, "session.created", map[string]any{
			"type": "session.created",
			"session": map[string]any{
				"id":     sessionID,
				"name":   taskName,
				"status": "running",
				"taskId": runID,
			},
		})
	}

	a.registerRun(sessionID, cancel, transport)
	defer transport.Close()

	a.bridgeEvents(ctx, transport, sessionID)
	log.Printf("[runViaUniverse] bridgeEvents returned for runID=%s", runID)
}

// runViaUniverseWithMessages is the session-resume variant of runViaUniverse.
// Identical control flow, but passes `initialMessages` through to the agent
// loop via `StartTaskInput.InitialMessages` — the agent runs in fork mode
// and replays the prior conversation before responding to the new input.
//
// `messages` is a slice of raw JSON entries (each a serialized
// `models.MessageContent`), typically loaded from the original session's
// `llm.jsonl`. Pre-condition: caller already set `a.running = true`.
func (a *App) runViaUniverseWithMessages(universeName, taskName, userInput string, messages []json.RawMessage) {
	log.Printf("[runViaUniverseWithMessages] universe=%q task=%q messages=%d", universeName, taskName, len(messages))
	ctx, cancel := context.WithCancel(a.ctx)
	var sessionID string
	defer func() { a.cleanup(cancel, sessionID) }()

	a.mu.Lock()
	master := a.master
	a.mu.Unlock()
	if master == nil || !master.IsActive(universeName) {
		a.emitRunError(taskName, fmt.Sprintf("universe %q is not active — activate it via the IconRibbon first", universeName))
		return
	}

	core, err := master.Activate(ctx, universeName)
	if err != nil {
		a.emitRunError(taskName, fmt.Sprintf("activate %q: %v", universeName, err))
		return
	}

	router := core.TerminalRouter()
	input := map[string]any{}
	if userInput != "" {
		input["user_input"] = userInput
	}
	transport, runID, sid, err := router.StartTaskRun(ctx, terminalapi.StartTaskInput{
		TaskName:        taskName,
		Input:           input,
		ClientLabel:     "ktw-gui-resume",
		InitialMessages: messages,
	})
	if err != nil {
		a.emitRunError(taskName, fmt.Sprintf("resume %q in %q: %v", taskName, universeName, err))
		return
	}
	sessionID = sid
	log.Printf("[runViaUniverseWithMessages] StartTaskRun ok runID=%s sessionID=%s", runID, sessionID)

	// Emit session.created so the frontend can lock onto the new
	// session journal (resumes get a FRESH session id — they fork the
	// conversation forward, not append to the original).
	if sessionID != "" {
		wailsRuntime.EventsEmit(a.ctx, "session.created", map[string]any{
			"type": "session.created",
			"session": map[string]any{
				"id":     sessionID,
				"name":   taskName,
				"status": "running",
				"taskId": runID,
			},
		})
	}

	a.registerRun(sessionID, cancel, transport)
	defer transport.Close()

	a.bridgeEvents(ctx, transport, sessionID)
}

// ── Shared infrastructure ─────────────────────────────────────────────────

// bridgeEvents reads terminal protocol events from the transport and forwards
// them as Wails runtime events. Returns when the transport channel closes
// (server-side Terminal.done) OR when ctx is cancelled (StopTask via the
// "Done" button). The ctx-cancel path is essential: the KTSP transport
// adapter's Receive() ignores ctx, so without watching ctx.Done() here we
// would wedge on the for-range loop forever after a user-initiated dismiss
// — leaving `a.running=true` and silently no-op'ing the next call (root
// cause of task-rerun-hang, 2026-05-06).
// The session's own workflow run is the first one started with no parent
// (rootRunOf): its answer is said in the chat (workflowAnswerEvents).
func (a *App) bridgeEvents(ctx context.Context, transport terminal.TerminalTransport, sessionID string) {
	rootRunID := ""
	// Send connect command to trigger state sync.
	if err := transport.Send(ctx, terminal.ConnectCmd{
		Type:   "connect",
		Params: terminal.ConnectOpt{},
	}); err != nil {
		log.Printf("[bridge] connect Send failed: %v", err)
	}

	ch, err := transport.Receive(ctx)
	if err != nil {
		log.Printf("[bridge] Receive failed: %v", err)
		return
	}

	// session.ended carries the sessionID so the frontend marks the RIGHT
	// session terminal (parallel chats); empty would default to the focused
	// one and could close the wrong tab.
	count := 0
	saidError := false // the chat already shows an error message of this run
	for {
		select {
		case <-ctx.Done():
			log.Printf("[bridge] ctx cancelled after %d events: %v", count, ctx.Err())
			wailsRuntime.EventsEmit(a.ctx, "session.ended", map[string]string{"status": "cancelled", "session": sessionID})
			return
		case raw, ok := <-ch:
			if !ok {
				log.Printf("[bridge] channel closed after %d events", count)
				for _, e := range runEndEvents(transport, sessionID, saidError) {
					wailsRuntime.EventsEmit(a.ctx, e.name, e.payload)
				}
				return
			}
			count++
			var probe struct {
				Type    string `json:"type"`
				Session string `json:"session"`
			}
			if json.Unmarshal(raw, &probe) != nil {
				continue
			}
			// DIAGNOSTIC (parallel-chat): the run is BOUND to `sessionID`;
			// each event carries its own `session`. If they differ for the
			// msg.* events, the frontend can't separate the two chats.
			if probe.Type == "msg.new" || probe.Type == "msg.delta" || probe.Type == "stats" {
				match := "OK"
				if probe.Session != sessionID {
					match = "MISMATCH"
				}
				log.Printf("[bridge run=%s] %s type=%q evt.session=%q", sessionID, match, probe.Type, probe.Session)
			}
			var obj any
			_ = json.Unmarshal(raw, &obj)
			wailsRuntime.EventsEmit(a.ctx, probe.Type, obj)
			saidError = saidError || isErrorMessage(probe.Type, obj)
			rootRunID = rootRunOf(rootRunID, probe.Type, obj)
			for _, e := range workflowAnswerEvents(sessionID, rootRunID, probe.Type, obj) {
				saidError = saidError || isErrorMessage(e.name, e.payload)
				wailsRuntime.EventsEmit(a.ctx, e.name, e.payload)
			}
		}
	}
}

// sessionEnd is the session.ended of a run whose events have all come: a run
// whose Done said an error (a model call that failed, a run that could not
// start) ends "error" with it - the chat's banner says « Tâche en échec »,
// never « Tâche terminée »; else "done".
func sessionEnd(transport any, sessionID string) map[string]string {
	if d, ok := transport.(interface{ DoneError() string }); ok {
		if why := d.DoneError(); why != "" {
			return map[string]string{"status": "error", "error": why, "session": sessionID}
		}
	}
	return map[string]string{"status": "done", "session": sessionID}
}

// runEndEvents is what the GUI emits once a run's events have all come: its
// session.ended (sessionEnd), preceded - for a run whose Done said an error
// while no error message reached its chat - by that error as a system
// message. A run that fails at once (a model whose key is not set) has its
// error message published by the core a hair after its Done, so it never
// reaches the chat: the reason must still be said there, never an empty chat
// under « Tâche en échec ».
func runEndEvents(transport any, sessionID string, saidError bool) []guiEvent {
	end := sessionEnd(transport, sessionID)
	if end["status"] != "error" || saidError {
		return []guiEvent{{"session.ended", end}}
	}
	content := map[string]any{"type": "system", "level": "error", "message": end["error"]}
	if d, ok := transport.(interface{ DoneMsg() *usermsg.Msg }); ok && d.DoneMsg() != nil {
		content["msg"] = d.DoneMsg() // said in the person's language (agent.modelKeyMissing, ...)
	}
	return []guiEvent{
		{"msg.new", map[string]any{
			"type": "msg.new", "session": sessionID, "seqNo": 0,
			"msg": map[string]any{"id": "run-error-" + sessionID, "sessionId": sessionID, "seqNo": 0, "role": "system",
				"timestamp": time.Now().Format(time.RFC3339), "content": content},
		}},
		{"session.ended", end},
	}
}

// isErrorMessage reports whether a GUI event is a chat message saying an
// error (a system message of level error).
func isErrorMessage(typ string, obj any) bool {
	if typ != "msg.new" {
		return false
	}
	ev, _ := obj.(map[string]any)
	msg, _ := ev["msg"].(map[string]any)
	content, _ := msg["content"].(map[string]any)
	return content["type"] == "system" && content["level"] == "error"
}

func (a *App) cleanup(cancel context.CancelFunc, sessionID string) {
	log.Printf("[cleanup] entered session=%q", sessionID)
	cancel()
	a.unregisterRun(sessionID)
	log.Printf("[cleanup] done session=%q", sessionID)
}

// rootRunOf is the session's own workflow run: the first team.workflow_started
// with no parent run. Never StartTaskRun's id - that is the terminal's client
// run id, not the engine's run id the workflow events carry.
func rootRunOf(root, typ string, obj any) string {
	if root != "" || typ != "team.workflow_started" {
		return root
	}
	ev, _ := obj.(map[string]any)
	if ev == nil {
		return root
	}
	if parent, _ := ev["parentRunId"].(string); parent != "" {
		return root
	}
	id, _ := ev["runId"].(string)
	return id
}

// agentResultOpen is the object an agent's final result names to open: the
// result is an {ok, open} answer (alone, or in a ```json fence); "" when it
// is prose, names nothing, or says it failed.
func agentResultOpen(summary string) string {
	s := strings.TrimSpace(summary)
	if strings.HasPrefix(s, "```") && strings.HasSuffix(s, "```") {
		s = strings.TrimSuffix(s, "```")
		if nl := strings.IndexByte(s, '\n'); nl >= 0 {
			s = s[nl+1:]
		} else {
			s = ""
		}
		s = strings.TrimSpace(s)
	}
	var ans struct {
		Ok   *bool  `json:"ok"`
		Open string `json:"open"`
	}
	if !strings.HasPrefix(s, "{") || json.Unmarshal([]byte(s), &ans) != nil {
		return ""
	}
	if ans.Ok != nil && !*ans.Ok {
		return ""
	}
	return strings.TrimSpace(ans.Open)
}

// workflowAnswerEvents says a workflow's answer in its chat: a workflow
// writes no message of its own, so the session's own run (rootRunID) ending
// done with an {ok, message, open} answer - a creation made with no model,
// as « Nouvelle étude de l'existant » - shows its message (an error when ok
// is false) and opens what it names (run.open, read by the GUI as a tool's
// {ok, open}); ending failed shows why. Any other event: nothing.
func workflowAnswerEvents(sessionID, rootRunID, typ string, obj any) []guiEvent {
	ev, _ := obj.(map[string]any)
	if ev == nil || rootRunID == "" || ev["runId"] != rootRunID {
		return nil
	}
	say := func(role, kind, level, text string) guiEvent {
		content := map[string]any{"type": kind}
		if kind == "text" {
			content["text"] = text
		} else {
			content["level"], content["message"] = level, text
		}
		return guiEvent{"msg.new", map[string]any{
			"type": "msg.new", "session": sessionID, "seqNo": 0,
			"msg": map[string]any{"id": "answer-" + rootRunID, "sessionId": sessionID, "seqNo": 0, "role": role,
				"timestamp": time.Now().Format(time.RFC3339), "content": content},
		}}
	}
	switch typ {
	case "team.workflow_done":
		out, _ := ev["output"].(map[string]any)
		// An agent task's run ends with its final result as the summary,
		// already said in its chat: a result that is an {ok, open} answer
		// opens what it names, and says nothing more.
		if summary, isText := out["summary"].(string); isText && len(out) == 1 {
			if open := agentResultOpen(summary); open != "" {
				return []guiEvent{{"run.open", map[string]any{"session": sessionID, "open": open}}}
			}
			return nil
		}
		msg, _ := out["message"].(string)
		refused := false
		if ok, isBool := out["ok"].(bool); isBool && !ok {
			refused = true
		}
		if refused {
			if msg == "" {
				return nil
			}
			return []guiEvent{say("system", "system", "error", msg)}
		}
		// An answer naming what it made opens it, with or without a message
		// : {ok, open} alone is a created object to show.
		var evs []guiEvent
		if msg != "" {
			evs = append(evs, say("assistant", "text", "", msg))
		}
		if open, _ := out["open"].(string); open != "" {
			evs = append(evs, guiEvent{"run.open", map[string]any{"session": sessionID, "open": open}})
		}
		return evs
	case "team.workflow_failed":
		why, _ := ev["error"].(string)
		if why == "" {
			why = "la tâche a échoué"
		}
		return []guiEvent{say("system", "system", "error", why)}
	}
	return nil
}

// emitRunError surfaces a run that could not start - the universe not
// active, a model whose key is not set - in the chat the person is looking
// at: the run gets a session of its own (session.created opens its chat
// tab), the message lands in it as an error and the session ends failed.
// emitError's pseudo-session "error" is shown by no chat, so a run refused
// at its start used to end on an empty chat with no reason.
func (a *App) emitRunError(taskName, msg string) {
	for _, e := range runStartFailureEvents(taskName, msg, time.Now()) {
		wailsRuntime.EventsEmit(a.ctx, e.name, e.payload)
	}
}

// guiEvent is one Wails event: its name and its payload.
type guiEvent struct {
	name    string
	payload any
}

// runStartFailureEvents is what emitRunError emits, in order: the failed
// run's own session created, its error message, its end (failed).
func runStartFailureEvents(taskName, msg string, now time.Time) []guiEvent {
	sid := fmt.Sprintf("start-failed-%d_%s", now.UnixNano(), taskName)
	return []guiEvent{
		{"session.created", map[string]any{
			"type":    "session.created",
			"session": map[string]any{"id": sid, "name": taskName, "status": "running"},
		}},
		{"msg.new", map[string]any{
			"type":    "msg.new",
			"session": sid,
			"seqNo":   0,
			"msg": map[string]any{
				"id":        "err-" + sid,
				"sessionId": sid,
				"seqNo":     0,
				"role":      "system",
				"timestamp": now.Format(time.RFC3339),
				"content":   map[string]any{"type": "system", "level": "error", "message": msg},
			},
		}},
		{"session.ended", map[string]string{"status": "error", "error": msg, "session": sid}},
	}
}

func (a *App) emitError(msg string) {
	// Emit as a system message so the user sees it in the chat.
	wailsRuntime.EventsEmit(a.ctx, "msg.new", map[string]any{
		"type":    "msg.new",
		"session": "error",
		"seqNo":   0,
		"msg": map[string]any{
			"id":        fmt.Sprintf("err-%d", time.Now().UnixMilli()),
			"sessionId": "error",
			"seqNo":     0,
			"role":      "system",
			"timestamp": time.Now().Format(time.RFC3339),
			"content": map[string]any{
				"type":    "system",
				"level":   "error",
				"message": msg,
			},
		},
	})
	wailsRuntime.EventsEmit(a.ctx, "session.ended", map[string]string{"status": "error", "error": msg})
}
