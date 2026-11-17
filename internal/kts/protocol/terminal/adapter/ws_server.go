// SPDX-License-Identifier: Apache-2.0

package adapter

import (
	"context"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal/room"
)

// WSServer serves the terminal protocol over WebSocket.
// Multiple clients connect to the same Room and receive live terminal messages.
// Coexists with the KTSP WS server on different HTTP paths (e.g. /terminal vs /ktsp).
type WSServer struct {
	room     *room.Room
	upgrader websocket.Upgrader
}

// NewWSServer creates a terminal WebSocket server for the given Room.
func NewWSServer(room *room.Room) *WSServer {
	return &WSServer{
		room: room,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

// Handler returns an http.HandlerFunc for use with an HTTP mux.
// Register it on any path, e.g.: mux.HandleFunc("/terminal", srv.Handler())
func (s *WSServer) Handler() http.HandlerFunc {
	return s.handleConn
}

func (s *WSServer) handleConn(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	adapter := NewWS(conn)
	defer adapter.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Connect to the Room as an OBSERVE-ALL client (sessionID "") — the
	// websocket surface watches every session.
	if err := s.room.Connect(ctx, adapter, ""); err != nil {
		return
	}

	// Block until the WebSocket closes.
	// The Room's command dispatch goroutine handles incoming messages.
	// We just need to keep the connection alive.
	ch, err := adapter.Receive(ctx)
	if err != nil {
		return
	}
	for range ch {
		// Messages consumed by Room.Connect's dispatch goroutine.
		// This loop keeps the HTTP handler alive until the client disconnects.
	}
}
