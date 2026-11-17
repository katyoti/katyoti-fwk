// SPDX-License-Identifier: Apache-2.0

// Package jsonrpc provides the JSON-RPC 2.0 wire codec for KTSP.
//
// Session wraps a raw Transport (json.RawMessage stream) and speaks KTSP types
// directly: Send accepts *ktsp.Request and returns *ktsp.Response.
// The JSON-RPC 2.0 envelope is an internal wire detail.
//
// Usage:
//
//	stream, _ := ws.Dial(ctx, url)
//	session := jsonrpc.NewSession(stream, "ws")
//	session.OnRequest(handler)
//	go session.Run()
//	resp, err := session.Send(ctx, req)
package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
	"github.com/katyoti/katyoti-fwk/internal/core/usermsg"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	"github.com/katyoti/katyoti-fwk/internal/kts/transport/api"
	genstdio "github.com/katyoti/katyoti-fwk/internal/kts/transport/stdio"
	genws "github.com/katyoti/katyoti-fwk/internal/kts/transport/ws"
)

// Session is a bidirectional KTSP session over any raw Transport.
// It handles JSON-RPC 2.0 request/response correlation, outgoing notifications,
// and incoming request/notification dispatch — all using KTSP typed messages.
type Session struct {
	stream   api.Transport
	idPrefix string

	pendingMu sync.Mutex
	pending   map[string]chan *ktsp.Response
	nextID    atomic.Uint64

	handlerMu        sync.RWMutex
	notifyHandlers   map[string]ktsp.NotificationHandler
	anyNotifyHandler func(slot, method string, params json.RawMessage)
	requestHandler   ktsp.RequestHandler

	closeOnce sync.Once
	done      chan struct{}
}

// NewSession creates a KTSP session over a raw Transport.
// idPrefix is prepended to auto-generated request IDs (e.g. "ws", "stdio").
func NewSession(stream api.Transport, idPrefix string) *Session {
	return &Session{
		stream:         stream,
		idPrefix:       idPrefix,
		pending:        make(map[string]chan *ktsp.Response),
		notifyHandlers: make(map[string]ktsp.NotificationHandler),
		done:           make(chan struct{}),
	}
}

// ── Factory functions ─────────────────────────────────────────────────────

// Dial connects to a WebSocket server and returns a KTSP session.
// The read loop starts automatically.
func Dial(ctx context.Context, url string) (*Session, error) {
	stream, err := genws.Dial(ctx, url)
	if err != nil {
		return nil, err
	}
	s := NewSession(stream, "ws")
	go s.Run()
	return s, nil
}

// StartStdio spawns a subprocess and returns a KTSP session over its stdio.
// The read loop starts automatically.
func StartStdio(command string, args ...string) (*Session, error) {
	t, err := genstdio.Start(command, args...)
	if err != nil {
		return nil, err
	}
	s := NewSession(t, "stdio")
	go s.Run()
	return s, nil
}

// StartStdioCmd starts a pre-configured exec.Cmd and returns a KTSP session.
func StartStdioCmd(cmd *exec.Cmd) (*Session, error) {
	t, err := genstdio.StartCmd(cmd)
	if err != nil {
		return nil, err
	}
	s := NewSession(t, "stdio")
	go s.Run()
	return s, nil
}

// Cmd returns the underlying subprocess command when this session is
// backed by a stdio subprocess (i.e., started via StartStdio or
// StartStdioCmd). Returns nil for ws-backed sessions or sessions
// created via NewFromPipes. Lets callers (e.g., connmgr's lifecycle
// path) send signals to the child before invoking the cooperative
// Close — supports SIGTERM-with-grace shutdown for stdio sidecars.
func (s *Session) Cmd() *exec.Cmd {
	if sub, ok := s.stream.(interface{ Cmd() *exec.Cmd }); ok {
		return sub.Cmd()
	}
	return nil
}

// NewFromPipes creates a KTSP session from raw pipes (for tests via io.Pipe()).
func NewFromPipes(in io.ReadCloser, out io.WriteCloser) *Session {
	t := genstdio.NewFromPipes(in, out)
	s := NewSession(t, "stdio")
	go s.Run()
	return s
}

// NewWSConn wraps a server-side WebSocket connection as a KTSP session.
// Register OnRequest/OnNotify handlers, then call Run() to start the read loop.
func NewWSConn(conn *websocket.Conn) *Session {
	stream := genws.New(conn)
	return NewSession(stream, "wsc")
}

// StdioServe runs a KTSP server on os.Stdin/os.Stdout and blocks until stdin closes.
// Convenience entry point for sidecar subprocess main() functions.
//
// baseCtx is the parent context every per-request ctx is derived from —
// pass `cmd.Context()` from the cobra command so service handlers can
// resolve the in-process Application (and through it the
// ServiceManager / EM points / configuration) via
// `application.UseService[T](ctx)` and
// `application.UseExtensions[T](ctx)`. A `Background()` baseCtx
// produces a server whose handlers can't see any registered service or
// extension — useful only for transport-level smoke tests.
//
// Each incoming request's ctx is wrapped with the session-bound Sender
// via `ktsp.WithSender` so service handlers can emit fire-and-forget
// notifications back over the same wire (e.g., the Terminal service
// streams task events back as Terminal.Event notifications).
//
// Phase E §5b fix (2026-06-05): previously this dropped baseCtx and
// dispatched with the JSON-RPC session's request ctx — a fresh
// `context.Background()` derivative with no Application attached.
// `contributions/list` then walked an empty `ContributionProvidersPoint`
// view and returned `{"contributions":[]}` even though the in-process
// CLI surface (which uses `cmd.Context()`) returned the full set.
func StdioServe(baseCtx context.Context, ctr ktsp.Container) error {
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	stream := genstdio.New(os.Stdin, os.Stdout)
	s := NewSession(stream, "srvstdio")
	sender := ktsp.NewSender(s)
	s.OnRequest(func(_ context.Context, slot, method string, params json.RawMessage) (json.RawMessage, error) {
		// Derive every per-request ctx from baseCtx so the in-process
		// Application + extensions remain visible to dispatched
		// service handlers. The session's own request ctx is dropped
		// intentionally — it carries no Application and its lifecycle
		// is tied to the whole stdio session (stdin EOF), which baseCtx
		// already inherits from the cobra root.
		ctx := ktsp.WithSender(baseCtx, sender)
		// JSON-RPC stdio doesn't encode an instance token yet — local
		// dispatch defaults to the singleton DefaultInstance. Update
		// when the stdio framing learns the `slot(instance)/method`
		// grammar.
		return ctr.Handle(ctx, slot, ktsp.DefaultInstance, method, params)
	})
	s.Run() // blocks
	return s.Close()
}

// ── Handler registration ──────────────────────────────────────────────────

// OnNotify registers a handler for incoming KTSP notifications on the given slot/method.
func (s *Session) OnNotify(slot, method string, handler ktsp.NotificationHandler) {
	s.handlerMu.Lock()
	defer s.handlerMu.Unlock()
	s.notifyHandlers[slot+"/"+method] = handler
}

// OnAnyNotify registers a catch-all handler for any incoming notification not
// handled by a specific OnNotify registration.
func (s *Session) OnAnyNotify(handler func(slot, method string, params json.RawMessage)) {
	s.handlerMu.Lock()
	defer s.handlerMu.Unlock()
	s.anyNotifyHandler = handler
}

// OnRequest registers a handler for all incoming KTSP requests.
func (s *Session) OnRequest(handler ktsp.RequestHandler) {
	s.handlerMu.Lock()
	defer s.handlerMu.Unlock()
	s.requestHandler = handler
}

// ── Send / Notify ─────────────────────────────────────────────────────────

// Send sends a KTSP request and waits for the matching response.
func (s *Session) Send(ctx context.Context, req *ktsp.Request) (*ktsp.Response, error) {
	select {
	case <-s.done:
		return nil, api.ErrClosed
	default:
	}

	id := fmt.Sprintf("%s-%d", s.idPrefix, s.nextID.Add(1))

	ch := make(chan *ktsp.Response, 1)
	s.pendingMu.Lock()
	s.pending[id] = ch
	s.pendingMu.Unlock()
	defer func() {
		s.pendingMu.Lock()
		delete(s.pending, id)
		s.pendingMu.Unlock()
	}()

	data, err := encodeRequest(id, req.Path(), req.Params)
	if err != nil {
		return nil, fmt.Errorf("session: encode request: %w", err)
	}
	if err := s.stream.Send(ctx, data); err != nil {
		return nil, fmt.Errorf("session: write request: %w", err)
	}

	select {
	case resp, ok := <-ch:
		if !ok {
			return nil, api.ErrClosed
		}
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.done:
		return nil, api.ErrClosed
	}
}

// Notify sends a fire-and-forget KTSP notification (no ID, no response).
func (s *Session) Notify(ctx context.Context, notif *ktsp.Notification) error {
	select {
	case <-s.done:
		return api.ErrClosed
	default:
	}
	data, err := encodeNotification(notif.Path(), notif.Params)
	if err != nil {
		return fmt.Errorf("session: encode notification: %w", err)
	}
	return s.stream.Send(ctx, data)
}

// ── Lifecycle ─────────────────────────────────────────────────────────────

// Run starts the read loop and blocks until EOF or Close.
func (s *Session) Run() {
	defer s.closeDone()
	ch, err := s.stream.Receive(context.Background())
	if err != nil {
		return
	}
	for raw := range ch {
		s.dispatch(raw)
	}
}

// Close shuts down the session, drains pending requests, and closes the stream.
func (s *Session) Close() error {
	s.closeDone()
	return s.stream.Close()
}

func (s *Session) closeDone() {
	s.closeOnce.Do(func() {
		close(s.done)
		s.pendingMu.Lock()
		for id, ch := range s.pending {
			close(ch)
			delete(s.pending, id)
		}
		s.pendingMu.Unlock()
	})
}

// ── Dispatch ──────────────────────────────────────────────────────────────

func (s *Session) dispatch(data json.RawMessage) {
	w, kind := decode(data)
	if w == nil {
		return
	}

	switch kind {
	case responseKind:
		s.pendingMu.Lock()
		ch, ok := s.pending[w.ID]
		s.pendingMu.Unlock()
		if ok {
			resp := &ktsp.Response{ID: w.ID, Result: w.Result}
			if w.Error != nil {
				resp.Error = &ktsp.RPCError{
					Code:    w.Error.Code,
					Message: w.Error.Message,
					Data:    w.Error.Data,
				}
			}
			ch <- resp
		}

	case requestKind:
		s.handlerMu.RLock()
		handler := s.requestHandler
		s.handlerMu.RUnlock()
		if handler == nil {
			return
		}
		reqID := w.ID
		_, slot, method := ktsp.ParsePath(w.Method)
		params := w.Params
		go func() {
			result, err := handler(context.Background(), slot, method, params)
			var rpcErr *wireError
			if err != nil {
				rpcErr = toWireError(err)
			}
			respData, _ := encodeResponse(reqID, result, rpcErr)
			_ = s.stream.Send(context.Background(), respData)
		}()

	case notificationKind:
		_, slot, method := ktsp.ParsePath(w.Method)
		key := slot + "/" + method
		s.handlerMu.RLock()
		handler := s.notifyHandlers[key]
		anyHandler := s.anyNotifyHandler
		s.handlerMu.RUnlock()
		if handler != nil {
			handler(w.Params)
		} else if anyHandler != nil {
			anyHandler(slot, method, w.Params)
		}
	}
}

// toWireError converts a Go error to a JSON-RPC wireError.
func toWireError(err error) *wireError {
	if _, keyed := usermsg.Of(err); keyed {
		rpcErr := ktsp.FromError(err)
		return &wireError{Code: rpcErr.Code, Message: rpcErr.Message, Data: rpcErr.Data}
	}
	if rpcErr, ok := err.(*ktsp.RPCError); ok {
		return &wireError{Code: rpcErr.Code, Message: rpcErr.Message, Data: rpcErr.Data}
	}
	data, e := json.Marshal(err)
	if e == nil && len(data) > 2 && string(data) != "{}" {
		var we wireError
		if json.Unmarshal(data, &we) == nil && we.Code != 0 {
			return &we
		}
	}
	return &wireError{Code: -32603, Message: err.Error()}
}
