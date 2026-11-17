// SPDX-License-Identifier: Apache-2.0

// Package adapter provides terminal protocol adapters over the generic
// transport layer (transport.Transport[json.RawMessage]).
//
// The terminal protocol is a push/pull event stream — not JSON-RPC 2.0 — so
// it uses the raw framing layer directly rather than transport.Session:
//
//	Server → Client: typed events marshaled as JSON (MsgNewEvent, StateSyncEvent, …)
//	Client → Server: commands unmarshaled from JSON (ConnectCmd, AnswerCmd, …)
//
// Usage (WebSocket server-side):
//
//	adapter := transport.NewWS(conn)
//	defer adapter.Close()
//	adapter.Send(ctx, terminal.StateSyncEvent{…})
//	ch, _ := adapter.Receive(ctx)
//	for raw := range ch {
//	    // dispatch raw command
//	}
package adapter

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gorilla/websocket"
	genws "github.com/katyoti/katyoti-fwk/internal/kts/transport/ws"
)

// WS implements terminal.TerminalTransport over a WebSocket connection.
// Used for GUI clients (browser, desktop app) connecting to the terminal server.
//
// Backed by ws.Stream — the read goroutine is internal; just Send/Receive/Close.
type WS struct {
	stream *genws.Stream
}

// NewWS wraps a WebSocket connection as a terminal transport.
func NewWS(conn *websocket.Conn) *WS {
	return &WS{stream: genws.New(conn)}
}

// Send marshals event as JSON and writes it to the WebSocket connection.
// Thread-safe — concurrent sends are serialized by the underlying stream.
func (a *WS) Send(ctx context.Context, event any) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("terminal/ws: marshal event: %w", err)
	}
	return a.stream.Send(ctx, data)
}

// Receive returns a channel of raw JSON messages from the WebSocket connection.
// The channel closes when the connection closes or the context is cancelled.
func (a *WS) Receive(ctx context.Context) (<-chan json.RawMessage, error) {
	return a.stream.Receive(ctx)
}

// Close shuts down the adapter and closes the underlying WebSocket connection.
func (a *WS) Close() error {
	return a.stream.Close()
}
