// SPDX-License-Identifier: Apache-2.0

// Package ws provides a Transport over WebSocket.
// The read loop is internal — no Run() call required; just Send/Receive/Close.
package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/katyoti/katyoti-fwk/internal/kts/transport/api"
)

// Dial connects to a WebSocket server at url and returns a Transport.
func Dial(ctx context.Context, url string) (api.Transport, error) {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, url, http.Header{})
	if err != nil {
		return nil, fmt.Errorf("ws: dial %s: %w", url, err)
	}
	return New(conn), nil
}

// Stream implements api.Transport over a WebSocket connection.
// An internal goroutine reads from the connection and feeds a buffered channel.
// Receive() returns a proxy channel scoped to the caller's context.
//
// The read goroutine starts immediately in New; call Close() when done.
type Stream struct {
	conn    *websocket.Conn
	recv    chan json.RawMessage
	writeMu sync.Mutex
	done    chan struct{}
	once    sync.Once
}

// New wraps an existing WebSocket connection as a Transport.
// The read goroutine starts immediately.
func New(conn *websocket.Conn) *Stream {
	s := &Stream{
		conn: conn,
		recv: make(chan json.RawMessage, 64),
		done: make(chan struct{}),
	}
	go s.readLoop()
	return s
}

// Send marshals and transmits msg over the WebSocket connection.
// Thread-safe — concurrent sends are serialized.
func (s *Stream) Send(_ context.Context, msg json.RawMessage) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.conn.WriteMessage(websocket.TextMessage, msg)
}

// Receive returns a channel that delivers incoming messages.
// The channel is closed when the transport closes or the context is cancelled.
func (s *Stream) Receive(ctx context.Context) (<-chan json.RawMessage, error) {
	select {
	case <-s.done:
		return nil, api.ErrClosed
	default:
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := make(chan json.RawMessage)
	go func() {
		defer close(out)
		for {
			select {
			case msg, ok := <-s.recv:
				if !ok {
					return
				}
				select {
				case out <- msg:
				case <-ctx.Done():
					return
				case <-s.done:
					return
				}
			case <-ctx.Done():
				return
			case <-s.done:
				return
			}
		}
	}()
	return out, nil
}

// Close shuts down the stream and closes the underlying WebSocket connection.
// Safe to call multiple times.
func (s *Stream) Close() error {
	s.once.Do(func() { close(s.done) })
	return s.conn.Close()
}

func (s *Stream) readLoop() {
	defer close(s.recv) // signal Receive that no more messages will arrive
	defer s.Close()
	for {
		_, data, err := s.conn.ReadMessage()
		if err != nil {
			return
		}
		msg := make(json.RawMessage, len(data))
		copy(msg, data)
		select {
		case s.recv <- msg:
		case <-s.done:
			return
		}
	}
}
