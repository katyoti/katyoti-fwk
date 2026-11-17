// SPDX-License-Identifier: Apache-2.0

// Package server provides shared terminal protocol wiring for both
// embedded (in-process) and external (stdio/ws) modes.
//
// ServeTask is the server side: Room + Translator + HumanGate + task execution.
// RunTUI is the client side: Bubbletea + bridge goroutine.
//
// Usage (embedded TUI):
//
//	serverT, clientT := adapter.NewChannelPair(256)
//	go client.ServeTask(ctx, serverT, opts)
//	client.RunTUI(ctx, clientT)
//
// Usage (external stdio):
//
//	stdioT := adapter.NewStdio(os.Stdin, os.Stdout)
//	client.ServeTask(ctx, stdioT, opts)   // blocks
//
// Usage (external TUI connecting via spawn):
//
//	stdioT := adapter.NewStdio(subprocess.Stdout, subprocess.Stdin)
//	client.RunTUI(ctx, stdioT)
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	"github.com/katyoti/katyoti-fwk/internal/core/services"
	hgapi "github.com/katyoti/katyoti-fwk/internal/extensions/humangate/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal/room"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal/translator"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
)

// TaskOpts configures a ServeTask call.
type TaskOpts struct {
	TaskName      string
	WorkspaceRoot string
	Input         map[string]any

	// SessionID is the journal session id forwarded verbatim to
	// `TaskRunRequest.SessionID`. Callers (TerminalService.StartTask)
	// pre-compute a UUIDv7 via `engine.GenerateSessionID` so the
	// resulting on-disk session directory matches what they returned
	// synchronously in `StartTaskOutput.SessionID`. Empty → let
	// TaskService.Run generate one.
	SessionID string

	// SkipHumanGate disables the terminal-backed HumanGate override.
	// Set to true when the caller provides its own HumanGate (e.g. pipe mode
	// with CLI prompter, or embedded TUI with TUIPrompter).
	SkipHumanGate bool

	// DryRun activates dry-run mode for this run: every spawned LLM/program
	// child is replaced with a deterministic simulator. Propagated into
	// taskapi.TaskRunRequest.DryRunMode.
	DryRun bool

	// ConfigOverride carries per-spawn dimension overrides forwarded
	// verbatim to `TaskRunRequest.ConfigOverride`. Used by session-resume
	// flows to inject `initialMessages` + `historyMode: "full_fork"` so
	// the agent loop replays the prior conversation instead of starting
	// fresh. Nil for default behavior.
	ConfigOverride map[string]any

	// Panels, when set, puts the session's chat panel on the universe's
	// panel subjects: its events go out to the panel
	// server and every screen following it, and a message the person sends
	// on its input is the run's next user turn. Nil: the chat panel shows
	// only on this session's transport.
	Panels *PanelWire
}

// PanelWire is how a session reaches the universe's panel subjects: the
// bus's publish and subscribe as functions (rule), and the tenant
// and universe the subjects live in.
type PanelWire struct {
	Tenant    string
	Universe  string
	Publish   panel.Publish
	Subscribe func(ctx context.Context, subject string, deliver func(payload []byte)) (stop func(), err error)
}

// ServeTask wires Room + Translator + HumanGate, connects the transport,
// starts the task, and blocks until it completes.
//
// This is the single implementation for all modes:
//   - Embedded TUI: transport = Channel (server side of pair)
//   - Terminal stdio: transport = StdioAdapter
//   - Terminal WS: transport = WSAdapter
func ServeTask(ctx context.Context, transport terminal.TerminalTransport, opts TaskOpts) error {
	// Resolve Room. It's an application-scoped singleton service (see
	// internal/kts/protocol/terminal/room/service.go) — multiple tasks
	// share the SAME instance over the lifetime of the universe core, so
	// we must NOT Close() it after one task. Closing the singleton broke
	// every subsequent task: r.done was closed, the fan-out + dispatch
	// goroutines spawned by Connect exited immediately on `<-r.done`,
	// state.sync still arrived (it's sent synchronously inside Connect
	// before the goroutines start) but no further events ever flowed.
	// (root cause of task-rerun-hang, 2026-05-06.)
	//
	// For tests / fallback paths where no service is registered we own
	// the Room locally and DO close it.
	r, _ := application.UseService[*room.Room](ctx)
	if r == nil {
		r = room.NewRoom("", nil)
		defer r.Close()
	}

	// Create the session FIRST. Use the JOURNAL session id (opts.SessionID,
	// the UUIDv7 the caller pre-computed) as the Room session id so a single
	// id keys everything: the Room session, the id the translator stamps on
	// every event (`session` field), the on-disk journal directory, AND the
	// id StartTask returns to the client for tab binding. Empty →
	// CreateSession auto-generates a `sess-xxx` id (non-GUI / test callers).
	// Creating it up front lets the gate + the fan-out client attach to the
	// session (the gate is a per-session attribute, stored ON the session).
	sess, err := r.CreateSession(opts.SessionID, opts.TaskName, "")
	if err != nil {
		return fmt.Errorf("terminal: create session: %w", err)
	}

	// Override HumanGate: route prompts through terminal protocol.
	// Skipped when the caller provides its own (pipe mode, embedded TUI).
	var gate *room.HumanGate
	if !opts.SkipHumanGate {
		gate = room.NewHumanGate()
		// The gate is a SESSION attribute (stored on sess). The Room routes
		// THIS session's answers to it; the run ctx carries it so ask-user
		// uses it directly — isolating parallel chats (the old app-global
		// "default" was overwritten by the second run, misrouting answers).
		r.SetHumanGate(sess.ID, gate)
		ctx = hgapi.WithHumanGate(ctx, hgapi.HumanGate(gate))
		if app := application.AppFromContext(ctx); app != nil {
			// Keep a registry fallback for non-ctx consumers (permission gate).
			application.PutService[hgapi.HumanGate](app, "default", hgapi.HumanGate(gate))
		}
	}

	// Enter session scope so session-scoped services (filestate cache) are available.
	ctx, scopeContainer := services.SessionScope.EnterScope(ctx)
	defer scopeContainer.Close()

	// Connect transport to Room as a client of THIS session (scoped fan-out).
	if err := r.Connect(ctx, transport, sess.ID); err != nil {
		return fmt.Errorf("terminal: connect: %w", err)
	}

	// Wire translator: EventBus → terminal messages → Room → transport.
	bus, _ := application.UseService[engine.EventBus](ctx)
	if bus == nil {
		bus = eventbus.NewMemoryEventBus()
	}
	// Make the bus available to the Room so the SendMessageCmd dispatcher
	// can publish agent-message events back into the engine.
	r.SetBus(bus)
	trans := translator.New(bus, r, sess.ID)
	if opts.Panels != nil {
		if gate != nil {
			gate.SetPanelInput(panelInputSender(opts.Panels))
		}
		stop, err := wireChatPanel(ctx, trans, opts.Panels, gate)
		if err != nil {
			slog.Warn("terminal: the chat panel stays on the session's transport", "session", sess.ID, "err", err)
		} else {
			defer stop()
		}
	}
	trans.Start()
	defer trans.Stop()

	// Start the task. Top-level run (ParentRunID empty); the agent factory
	// recognizes "no parent + no SpawnPermissions declared" as an interactive
	// user run and applies AllowAll. Spawn paths (delegate/workflow) carry
	// ParentRunID and inherit perms from their mode preset instead.
	taskSvc, err := application.UseService[taskapi.TaskService](ctx)
	if err != nil {
		return fmt.Errorf("terminal: task service: %w", err)
	}

	runResp, err := taskSvc.Run(ctx, taskapi.TaskRunRequest{
		TaskName:       opts.TaskName,
		WorkspaceRoot:  opts.WorkspaceRoot,
		Input:          opts.Input,
		DryRunMode:     opts.DryRun,
		ConfigOverride: opts.ConfigOverride,
		SessionID:      opts.SessionID,
	})
	if err != nil {
		err = fmt.Errorf("terminal: run task: %w", err)
		trans.ReportRunFailure(err)
		_ = r.UpdateSessionStatus(sess.ID, "error")
		return err
	}

	// Block until task completes.
	result, err := taskSvc.Result(ctx, taskapi.ResultRequest{RunID: runResp.RunID})
	if err != nil {
		return err
	}

	// A run that ended in error leaves its session "error".
	if result.Error != "" {
		_ = r.UpdateSessionStatus(sess.ID, "error")
		return fmt.Errorf("%s", result.Error)
	}
	_ = r.UpdateSessionStatus(sess.ID, "done")
	return nil
}

// wireChatPanel makes the session's chat panel, sends each of its events on
// its panel's out subject and listens on its input: a message the person
// sends there is the run's next user turn, answered through the session's
// gate. It answers the listen's stop.
func wireChatPanel(ctx context.Context, trans *translator.Translator, w *PanelWire, gate *room.HumanGate) (func(), error) {
	if w.Publish == nil || w.Subscribe == nil {
		return nil, fmt.Errorf("terminal: a panel wire needs its publish and its subscribe")
	}
	id := trans.UseChatPanel("", func(e panel.Event) { publishPanelEvent(ctx, w, e) })
	var deliver func(string)
	if gate != nil {
		deliver = gate.DeliverAnswer
	}
	return w.Subscribe(ctx, subjects.PanelInSubject(w.Tenant, w.Universe, id), chatInput(trans.ChatMessage, deliver))
}

// panelInputSender sends the person's input to a panel's input subject: a
// form the chat card submitted for the question ask-user-data opened
// (room.HumanGate.DeliverPanelInput), taken there by the run waiting on it.
func panelInputSender(w *PanelWire) func(ctx context.Context, in panel.Input) error {
	return func(ctx context.Context, in panel.Input) error {
		if err := in.Validate(); err != nil {
			return err
		}
		payload, err := json.Marshal(in)
		if err != nil {
			return err
		}
		pctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		return w.Publish(pctx, subjects.PanelInSubject(w.Tenant, w.Universe, in.Panel), payload)
	}
}

// chatInput reads one input off the chat panel's input subject: a message
// the person sent is handed to deliver - the run's next user turn -; any
// other input, or one that does not decode, is left.
func chatInput(message func(panel.Input) (string, bool), deliver func(string)) func([]byte) {
	return func(payload []byte) {
		in, err := panel.DecodeInput(payload)
		if err != nil {
			slog.Warn("terminal: a chat panel input does not decode", "err", err)
			return
		}
		text, ok := message(in)
		if !ok || deliver == nil {
			return
		}
		deliver(text)
	}
}

// publishPanelEvent sends one chat panel event on its panel's out subject.
func publishPanelEvent(ctx context.Context, w *PanelWire, e panel.Event) {
	payload, err := json.Marshal(e)
	if err != nil {
		slog.Warn("terminal: a chat panel event could not be encoded", "panel", e.Panel, "seq", e.Seq, "err", err)
		return
	}
	pctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if err := w.Publish(pctx, subjects.PanelOutSubject(w.Tenant, w.Universe, e.Panel), payload); err != nil {
		slog.Warn("terminal: a chat panel event did not go out", "panel", e.Panel, "seq", e.Seq, "err", err)
	}
}
