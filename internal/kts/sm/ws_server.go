// SPDX-License-Identifier: Apache-2.0

package sm

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	ktransport "github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/jsonrpc"
)

// wsCallerTransportKey is the context key for the per-connection transport.
// Used by journal/subscribe to push notifications back to the caller.
type wsCallerTransportKey struct{}

// setCallerTransport injects a caller transport into the context.
func setCallerTransport(ctx context.Context, t ktsp.Transport) context.Context {
	return context.WithValue(ctx, wsCallerTransportKey{}, t)
}

// CallerTransport retrieves the per-connection transport from context.
// Returns false if no transport is set (i.e. caller is not a WS connection).
func CallerTransport(ctx context.Context) (ktsp.Transport, bool) {
	t, ok := ctx.Value(wsCallerTransportKey{}).(ktsp.Transport)
	return t, ok
}

// WsServer exposes the Session Manager over WebSocket (KTSP/WS).
// External containers connect, call SM slots, and receive pushed notifications.
type WsServer struct {
	sm       *Manager
	port     int
	upgrader websocket.Upgrader
	server   *http.Server
}

// NewWsServer creates a new WebSocket server for the given SM.
func NewWsServer(sm *Manager, port int) *WsServer {
	return &WsServer{
		sm:   sm,
		port: port,
		upgrader: websocket.Upgrader{
			// Phase 1: localhost only, no origin check needed.
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

// Addr returns the listening address string (e.g., "localhost:7765").
func (s *WsServer) Addr() string {
	return fmt.Sprintf("localhost:%d", s.port)
}

// WsURL returns the WebSocket endpoint URL.
func (s *WsServer) WsURL() string {
	return fmt.Sprintf("ws://localhost:%d/ktsp", s.port)
}

// Start begins listening on localhost:{port} and serves until ctx is cancelled.
// Returns http.ErrServerClosed on clean shutdown.
func (s *WsServer) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.Addr())
	if err != nil {
		return fmt.Errorf("ktsp ws server listen %s: %w", s.Addr(), err)
	}
	return s.ServeOn(ctx, ln)
}

// ServeOn serves WebSocket connections on a pre-created listener.
// Useful for testing with OS-assigned ports (net.Listen("tcp", "localhost:0")).
func (s *WsServer) ServeOn(ctx context.Context, ln net.Listener) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ktsp", s.handleConn)
	s.server = &http.Server{
		Addr:    ln.Addr().String(),
		Handler: mux,
	}

	// Shutdown when context is cancelled.
	go func() {
		<-ctx.Done()
		_ = s.server.Shutdown(context.Background())
	}()

	return s.server.Serve(ln)
}

// handleConn upgrades an HTTP connection to WebSocket, registers SM handlers,
// and blocks until the connection closes.
func (s *WsServer) handleConn(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	adapter := ktransport.NewWSConn(conn)
	defer adapter.Close()

	// Inject the per-connection transport into the base context so that
	// SM handlers (e.g. journal/subscribe) can push notifications back.
	ctx := setCallerTransport(r.Context(), adapter)

	// Route incoming requests to the SM. The WS framing doesn't carry
	// an instance token yet — every dispatch hits the singleton
	// DefaultInstance. When the WS adapter learns the
	// `slot(instance)/method` grammar, thread the parsed instance here.
	adapter.OnRequest(func(_ context.Context, slot, method string, params json.RawMessage) (json.RawMessage, error) {
		return s.sm.Handle(ctx, slot, ktsp.DefaultInstance, method, params)
	})

	// Route incoming notifications (fire-and-forget) to the SM.
	adapter.OnAnyNotify(func(slot, method string, params json.RawMessage) {
		_, _ = s.sm.Handle(ctx, slot, ktsp.DefaultInstance, method, params)
	})

	// Block until the connection closes.
	adapter.Run()
}
