// SPDX-License-Identifier: Apache-2.0

// Package driver drives one Claude Code worker over its stdio: the initialize
// handshake, the subscription guard, the lifecycle state machine, user
// messages, permissions (can_use_tool) and their withdrawal, interrupts and
// the clean end. It works on a Conn, so tests drive it against a scripted
// fake Claude without starting a process.
package driver

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/claudecode/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/claudecode/internal/protocol"
)

// Conn is a started Claude Code: its pipes and its end.
type Conn interface {
	Stdin() io.WriteCloser
	Stdout() io.Reader
	Stderr() io.Reader // nil: none
	Done() <-chan struct{}
	ExitCode() int
	Shutdown(grace time.Duration)
}

// ErrNotSubscription refuses a worker that would bill an API key.
var ErrNotSubscription = errors.New("Claude Code is not signed in with a subscription: an API key or another provider would be billed")

// Timeouts, overridable by tests.
var (
	InitializeTimeout = 60 * time.Second
	ControlTimeout    = 60 * time.Second
	ShutdownGrace     = 5 * time.Second
)

const historyMax = 1000

// Worker is one driven Claude Code process.
type Worker struct {
	conn Conn
	w    *protocol.Writer

	mu      sync.Mutex
	info    api.Worker
	perm    *pendingPerm
	waiters map[string]chan protocol.ControlResponseBody
	history []api.Event
	subs    map[int]chan api.Event
	nextSub int
	seq     int64
	stderr  []string
	failure string // why the worker is ending abnormally; the first reason wins

	reqSeq   atomic.Int64
	readDone chan struct{}
	ended    chan struct{}
	endOnce  sync.Once
}

type pendingPerm struct {
	req         api.PermissionRequest
	input       json.RawMessage
	suggestions []json.RawMessage
}

// New wraps a started conn. Start must follow.
func New(conn Conn, info api.Worker) *Worker {
	if info.ID == "" {
		info.ID = NewID()
	}
	info.State = api.StateStarting
	if info.StartedAt.IsZero() {
		info.StartedAt = time.Now().UTC()
	}
	return &Worker{
		conn:     conn,
		w:        protocol.NewWriter(conn.Stdin()),
		info:     info,
		waiters:  map[string]chan protocol.ControlResponseBody{},
		subs:     map[int]chan api.Event{},
		readDone: make(chan struct{}),
		ended:    make(chan struct{}),
	}
}

// NewID is a worker id.
func NewID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return "w-" + hex.EncodeToString(b)
}

// Start runs the handshake, applies the subscription guard and, when prompt
// is not empty, sends it. A refused start ends the worker.
func (w *Worker) Start(ctx context.Context, prompt string) error {
	go w.readLoop()
	if se := w.conn.Stderr(); se != nil {
		go w.stderrLoop(se)
	}
	body, err := w.request(ctx, InitializeTimeout, map[string]any{"subtype": protocol.SubtypeInitialize, "hooks": nil})
	if err != nil {
		w.fail(fmt.Errorf("initialize: %w", err))
		return w.endErr()
	}
	var init protocol.InitializeResponse
	_ = json.Unmarshal(body, &init)
	if init.Account != nil {
		a := init.Account
		w.mu.Lock()
		w.info.Account = &api.Account{Email: a.Email, Organization: a.Organization, SubscriptionType: a.SubscriptionType,
			TokenSource: a.TokenSource, APIKeySource: a.APIKeySource, APIProvider: a.APIProvider}
		w.mu.Unlock()
		if !subscription(a.SubscriptionType, a.APIProvider, a.APIKeySource) {
			w.fail(ErrNotSubscription)
			return w.endErr()
		}
	}
	if init.ClaudeCodeVersion != "" {
		w.mu.Lock()
		w.info.ClaudeVersion = init.ClaudeCodeVersion
		w.mu.Unlock()
	}
	w.setState(api.StateIdle)
	if prompt != "" {
		return w.Send(prompt)
	}
	return nil
}

// OpenRemote opens the worker to claude.ai/code and the Claude app (Remote
// Control): the person watches and drives it there too. It returns the
// session's URL. A refusal leaves the worker running without it.
func (w *Worker) OpenRemote(ctx context.Context, name string) (string, error) {
	req := map[string]any{"subtype": protocol.SubtypeRemoteControl, "enabled": true}
	if name != "" {
		req["name"] = name
	}
	body, err := w.request(ctx, ControlTimeout, req)
	if err != nil {
		w.mu.Lock()
		w.info.RemoteState = "refused: " + err.Error()
		w.mu.Unlock()
		return "", fmt.Errorf("remote control: %w", err)
	}
	var rc protocol.RemoteControlResponse
	_ = json.Unmarshal(body, &rc)
	w.mu.Lock()
	w.info.RemoteURL = rc.SessionURL
	if w.info.RemoteState == "" {
		w.info.RemoteState = "opened"
	}
	w.mu.Unlock()
	w.emit(api.Event{Kind: api.EventRemote, Text: rc.SessionURL})
	return rc.SessionURL, nil
}

// subscription: a claude.ai subscription sign-in, first-party, no API key.
func subscription(subType, provider, keySource string) bool {
	if subType == "" {
		return false
	}
	if provider != "" && provider != "firstParty" {
		return false
	}
	return keySource == "" || keySource == "none"
}

// Send writes the next user message.
func (w *Worker) Send(text string) error {
	// The state moves before the write: Claude Code may answer — a result, a
	// crash — before the write returns, and that answer must win.
	w.mu.Lock()
	if w.info.State.Ended() {
		id := w.info.ID
		w.mu.Unlock()
		return fmt.Errorf("worker %s has ended", id)
	}
	moved := w.info.State != api.StateAwaitingPermission && w.info.State != api.StateRunning
	if moved {
		w.info.State = api.StateRunning
	}
	w.mu.Unlock()
	if moved {
		w.emit(api.Event{Kind: api.EventState, State: api.StateRunning})
	}
	if err := w.w.Write(protocol.NewUserMessage(text)); err != nil {
		w.fail(fmt.Errorf("write: %w", err))
		return w.endErr()
	}
	return nil
}

// Answer answers the pending permission.
func (w *Worker) Answer(in api.AnswerInput) error {
	w.mu.Lock()
	p := w.perm
	if p == nil || p.req.RequestID != in.RequestID {
		w.mu.Unlock()
		return fmt.Errorf("worker %s waits on no permission %q", w.info.ID, in.RequestID)
	}
	w.mu.Unlock()
	var resp any
	switch in.Behavior {
	case api.BehaviorAllow:
		allow := protocol.PermissionAllow{Behavior: api.BehaviorAllow, UpdatedInput: p.input}
		if len(allow.UpdatedInput) == 0 {
			allow.UpdatedInput = json.RawMessage(`{}`)
		}
		switch in.Scope {
		case "", api.ScopeOnce:
		case api.ScopeSession:
			allow.UpdatedPermissions = withDestination(p.suggestions, "session")
		case api.ScopeProject:
			allow.UpdatedPermissions = withDestination(p.suggestions, "localSettings")
		default:
			return fmt.Errorf("scope %q: once, session or project", in.Scope)
		}
		resp = allow
	case api.BehaviorDeny:
		msg := in.Message
		if msg == "" {
			msg = "Denied by the person."
		}
		resp = protocol.PermissionDeny{Behavior: api.BehaviorDeny, Message: msg, Interrupt: in.Interrupt}
	default:
		return fmt.Errorf("behavior %q: allow or deny", in.Behavior)
	}
	b, _ := json.Marshal(resp)
	// The pending permission clears before the write: Claude Code goes on at
	// once and what it answers next must win.
	w.mu.Lock()
	if w.perm != p {
		w.mu.Unlock()
		return fmt.Errorf("worker %s: permission %q was withdrawn", w.info.ID, in.RequestID)
	}
	w.perm = nil
	w.info.Pending = nil
	if w.info.State == api.StateAwaitingPermission {
		w.info.State = api.StateRunning
	}
	w.mu.Unlock()
	w.emit(api.Event{Kind: api.EventState, State: api.StateRunning})
	if err := w.w.Write(protocol.ControlResponse{Type: protocol.TypeControlResponse,
		Response: protocol.ControlResponseBody{Subtype: "success", RequestID: in.RequestID, Response: b}}); err != nil {
		w.fail(fmt.Errorf("write: %w", err))
		return w.endErr()
	}
	return nil
}

// withDestination copies the suggested permission updates, each sent to dest.
func withDestination(sugg []json.RawMessage, dest string) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(sugg))
	for _, s := range sugg {
		var m map[string]any
		if json.Unmarshal(s, &m) != nil {
			continue
		}
		m["destination"] = dest
		b, _ := json.Marshal(m)
		out = append(out, b)
	}
	return out
}

// Interrupt ends the current turn.
func (w *Worker) Interrupt(ctx context.Context) error {
	w.setState(api.StateInterrupting)
	_, err := w.request(ctx, ControlTimeout, map[string]any{"subtype": protocol.SubtypeInterrupt})
	return err
}

// Stop ends the worker cleanly: end of input, then terminate, then kill.
func (w *Worker) Stop() {
	w.conn.Shutdown(ShutdownGrace)
	select {
	case <-w.ended:
	case <-time.After(ShutdownGrace):
	}
}

// Ended is closed when the worker has ended.
func (w *Worker) Ended() <-chan struct{} { return w.ended }

// Info is the worker's state now.
func (w *Worker) Info() api.Worker {
	w.mu.Lock()
	defer w.mu.Unlock()
	i := w.info
	if i.Pending != nil {
		p := *i.Pending
		i.Pending = &p
	}
	return i
}

// History is every event so far (the latest historyMax).
func (w *Worker) History() []api.Event {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]api.Event(nil), w.history...)
}

// Subscribe receives the events from now on; cancel stops it. A slow
// subscriber loses events rather than holding the worker.
func (w *Worker) Subscribe() (<-chan api.Event, func()) {
	ch := make(chan api.Event, 256)
	w.mu.Lock()
	id := w.nextSub
	w.nextSub++
	w.subs[id] = ch
	w.mu.Unlock()
	return ch, func() {
		w.mu.Lock()
		if c, ok := w.subs[id]; ok {
			delete(w.subs, id)
			close(c)
		}
		w.mu.Unlock()
	}
}

// Follow is the history so far and the events from then on, taken together so
// no event falls between them; cancel stops it.
func (w *Worker) Follow() ([]api.Event, <-chan api.Event, func()) {
	ch := make(chan api.Event, 256)
	w.mu.Lock()
	past := append([]api.Event(nil), w.history...)
	ended := w.info.State.Ended()
	id := w.nextSub
	w.nextSub++
	if ended {
		close(ch)
	} else {
		w.subs[id] = ch
	}
	w.mu.Unlock()
	return past, ch, func() {
		w.mu.Lock()
		if c, ok := w.subs[id]; ok {
			delete(w.subs, id)
			close(c)
		}
		w.mu.Unlock()
	}
}

// request sends a host control request and waits for its answer.
func (w *Worker) request(ctx context.Context, timeout time.Duration, req map[string]any) (json.RawMessage, error) {
	id := fmt.Sprintf("req_%d_%s", w.reqSeq.Add(1), NewID()[2:])
	ch := make(chan protocol.ControlResponseBody, 1)
	w.mu.Lock()
	w.waiters[id] = ch
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		delete(w.waiters, id)
		w.mu.Unlock()
	}()
	b, _ := json.Marshal(req)
	if err := w.w.Write(protocol.ControlRequest{Type: protocol.TypeControlRequest, RequestID: id, Request: b}); err != nil {
		return nil, err
	}
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case r := <-ch:
		if r.Subtype == "error" {
			return nil, errors.New(r.Error)
		}
		return r.Response, nil
	case <-w.ended:
		return nil, w.endErr()
	case <-t.C:
		return nil, fmt.Errorf("no answer in %s", timeout)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (w *Worker) readLoop() {
	defer close(w.readDone)
	r := protocol.NewReader(w.conn.Stdout())
	for {
		frame, env, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			w.fail(err)
			break
		}
		w.dispatch(frame, env)
	}
	select {
	case <-w.conn.Done():
	case <-time.After(ShutdownGrace):
		w.conn.Shutdown(ShutdownGrace)
	}
	code := w.conn.ExitCode()
	if code == 0 {
		w.end(api.StateFinished, "")
		return
	}
	why := fmt.Sprintf("claude exited with code %d", code)
	if tail := w.stderrTail(); tail != "" {
		why += ": " + tail
	}
	w.end(api.StateCrashed, why)
}

func (w *Worker) dispatch(frame json.RawMessage, env protocol.Envelope) {
	switch env.Type {
	case protocol.TypeSystem:
		switch env.Subtype {
		case protocol.SubtypeInit:
			var si protocol.SystemInit
			_ = json.Unmarshal(frame, &si)
			w.mu.Lock()
			w.info.ClaudeSessionID = si.SessionID
			if si.Model != "" {
				w.info.Model = si.Model
			}
			if si.ClaudeCodeVersion != "" {
				w.info.ClaudeVersion = si.ClaudeCodeVersion
			}
			w.mu.Unlock()
			if si.APIKeySource != "" && si.APIKeySource != "none" {
				w.fail(ErrNotSubscription)
				w.conn.Shutdown(ShutdownGrace)
			}
		case protocol.SubtypeBridgeState:
			var bs protocol.BridgeState
			_ = json.Unmarshal(frame, &bs)
			w.mu.Lock()
			w.info.RemoteState = bs.State
			w.mu.Unlock()
			w.emit(api.Event{Kind: api.EventRemote, Text: bs.State})
		case protocol.SubtypeSessionStateChanged:
			var sc protocol.SessionStateChanged
			_ = json.Unmarshal(frame, &sc)
			switch sc.State {
			case "idle":
				w.mu.Lock()
				waiting := w.perm != nil
				w.mu.Unlock()
				if !waiting {
					w.setState(api.StateIdle)
				}
			case "running":
				w.setState(api.StateRunning)
			}
		default:
			w.emit(api.Event{Kind: api.EventFrame, Frame: frame})
		}
	case protocol.TypeAssistant:
		var m protocol.ChatMessage
		_ = json.Unmarshal(frame, &m)
		for _, b := range m.Blocks() {
			switch b.Type {
			case "text":
				w.emit(api.Event{Kind: api.EventText, Text: b.Text})
			case "tool_use":
				w.emit(api.Event{Kind: api.EventToolUse, Tool: b.Name, Input: b.Input})
			}
		}
	case protocol.TypeUser:
		var m protocol.ChatMessage
		_ = json.Unmarshal(frame, &m)
		for _, b := range m.Blocks() {
			if b.Type == "tool_result" {
				w.emit(api.Event{Kind: api.EventToolResult, Text: resultText(b.Content), IsError: b.IsError})
			}
		}
	case protocol.TypeResult:
		var r protocol.Result
		_ = json.Unmarshal(frame, &r)
		w.mu.Lock()
		w.info.Turns++
		if r.TotalCostUSD > 0 {
			w.info.CostUSD = r.TotalCostUSD
		}
		w.info.LastResult = r.Result
		if r.IsError {
			w.info.LastError = strings.TrimSpace(r.Result + " " + strings.Join(r.Errors, "; "))
		}
		if r.SessionID != "" {
			w.info.ClaudeSessionID = r.SessionID
		}
		if w.info.State == api.StateRunning || w.info.State == api.StateInterrupting {
			w.info.State = api.StateIdle
		}
		w.mu.Unlock()
		w.emit(api.Event{Kind: api.EventResult, Text: r.Result, IsError: r.IsError, CostUSD: r.TotalCostUSD})
	case protocol.TypeRateLimit:
		var rl protocol.RateLimitEvent
		_ = json.Unmarshal(frame, &rl)
		w.mu.Lock()
		w.info.RateLimit = rl.RateLimitInfo.Status
		w.mu.Unlock()
		w.emit(api.Event{Kind: api.EventRateLimit, Text: rl.RateLimitInfo.Status, Frame: frame})
	case protocol.TypeControlRequest:
		w.onControlRequest(frame)
	case protocol.TypeControlCancelRequest:
		var c protocol.ControlCancel
		_ = json.Unmarshal(frame, &c)
		w.mu.Lock()
		withdrawn := w.perm != nil && w.perm.req.RequestID == c.RequestID
		if withdrawn {
			w.perm = nil
			w.info.Pending = nil
			if w.info.State == api.StateAwaitingPermission {
				w.info.State = api.StateRunning
			}
		}
		w.mu.Unlock()
		if withdrawn {
			w.emit(api.Event{Kind: api.EventState, State: api.StateRunning, Text: "permission withdrawn"})
		}
	case protocol.TypeControlResponse:
		var cr protocol.ControlResponse
		_ = json.Unmarshal(frame, &cr)
		w.mu.Lock()
		ch := w.waiters[cr.Response.RequestID]
		w.mu.Unlock()
		if ch != nil {
			ch <- cr.Response
		}
	case protocol.TypeKeepAlive, protocol.TypeStreamEvent:
	default:
		w.emit(api.Event{Kind: api.EventFrame, Frame: frame})
	}
}

func (w *Worker) onControlRequest(frame json.RawMessage) {
	var cr protocol.ControlRequest
	_ = json.Unmarshal(frame, &cr)
	var head struct {
		Subtype string `json:"subtype"`
	}
	_ = json.Unmarshal(cr.Request, &head)
	if head.Subtype != protocol.SubtypeCanUseTool {
		_ = w.w.Write(protocol.ControlResponse{Type: protocol.TypeControlResponse, Response: protocol.ControlResponseBody{
			Subtype: "error", RequestID: cr.RequestID, Error: "not supported by this host: " + head.Subtype}})
		return
	}
	var cu protocol.CanUseTool
	_ = json.Unmarshal(cr.Request, &cu)
	req := api.PermissionRequest{RequestID: cr.RequestID, ToolName: cu.ToolName, ToolUseID: cu.ToolUseID, Input: cu.Input,
		Title: cu.Title, Description: cu.Description, Reason: cu.DecisionReason, BlockedPath: cu.BlockedPath,
		Suggestions: cu.PermissionSuggestions, At: time.Now().UTC()}
	w.mu.Lock()
	w.perm = &pendingPerm{req: req, input: cu.Input, suggestions: cu.PermissionSuggestions}
	p := req
	w.info.Pending = &p
	w.info.State = api.StateAwaitingPermission
	w.mu.Unlock()
	w.emit(api.Event{Kind: api.EventPermission, State: api.StateAwaitingPermission, Tool: cu.ToolName, Input: cu.Input, Permission: &req})
}

func resultText(c json.RawMessage) string {
	if len(c) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(c, &s) == nil {
		return s
	}
	var bs []protocol.ContentBlock
	if json.Unmarshal(c, &bs) == nil {
		var parts []string
		for _, b := range bs {
			if b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return string(c)
}

func (w *Worker) stderrLoop(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), protocol.MaxLine)
	for sc.Scan() {
		line := sc.Text()
		w.mu.Lock()
		w.stderr = append(w.stderr, line)
		if len(w.stderr) > 50 {
			w.stderr = w.stderr[len(w.stderr)-50:]
		}
		w.mu.Unlock()
		w.emit(api.Event{Kind: api.EventStderr, Text: line})
	}
}

func (w *Worker) stderrTail() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := len(w.stderr)
	if n > 5 {
		return strings.Join(w.stderr[n-5:], " | ")
	}
	return strings.Join(w.stderr, " | ")
}

// fail records why the worker is ending; the first reason wins.
func (w *Worker) fail(err error) {
	w.mu.Lock()
	if w.failure == "" || errors.Is(err, ErrNotSubscription) {
		w.failure = err.Error()
		w.info.LastError = err.Error()
	}
	w.mu.Unlock()
	if errors.Is(err, ErrNotSubscription) {
		go w.conn.Shutdown(ShutdownGrace)
	}
}

func (w *Worker) endErr() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.failure != "" {
		return errors.New(w.failure)
	}
	return fmt.Errorf("worker %s has ended", w.info.ID)
}

func (w *Worker) end(state api.State, why string) {
	w.endOnce.Do(func() {
		now := time.Now().UTC()
		w.mu.Lock()
		if w.failure != "" {
			state = api.StateCrashed
			w.info.LastError = w.failure
		} else if why != "" {
			w.info.LastError = why
		}
		w.info.State = state
		w.info.EndedAt = &now
		w.info.Pending = nil
		w.perm = nil
		w.mu.Unlock()
		w.emit(api.Event{Kind: api.EventState, State: state, Text: w.Info().LastError})
		close(w.ended)
		w.mu.Lock()
		for id, c := range w.subs {
			delete(w.subs, id)
			close(c)
		}
		w.mu.Unlock()
	})
}

func (w *Worker) setState(s api.State) {
	w.mu.Lock()
	if w.info.State.Ended() || w.info.State == s {
		w.mu.Unlock()
		return
	}
	w.info.State = s
	w.mu.Unlock()
	w.emit(api.Event{Kind: api.EventState, State: s})
}

func (w *Worker) emit(e api.Event) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.seq++
	e.Seq = w.seq
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	w.history = append(w.history, e)
	if len(w.history) > historyMax {
		w.history = w.history[len(w.history)-historyMax:]
	}
	for _, c := range w.subs {
		select {
		case c <- e:
		default:
		}
	}
}
