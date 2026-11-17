// SPDX-License-Identifier: Apache-2.0

package adapter_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal"
	termadapter "github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal/adapter"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal/room"
)

func startTerminalWSServer(t *testing.T, r *room.Room) string {
	t.Helper()
	srv := termadapter.NewWSServer(r)

	mux := http.NewServeMux()
	mux.HandleFunc("/terminal", srv.Handler())

	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	httpSrv := &http.Server{Handler: mux}
	go func() { _ = httpSrv.Serve(ln) }()
	t.Cleanup(func() { httpSrv.Shutdown(context.Background()) })

	return "ws://" + ln.Addr().String() + "/terminal"
}

func TestWSServer_ConnectReceivesSync(t *testing.T) {
	r := room.NewRoom(t.TempDir(), nil)
	defer r.Close()

	if _, err := r.CreateSession("s1", "test-task", "t1"); err != nil {
		t.Fatal(err)
	}

	wsURL := startTerminalWSServer(t, r)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// First message should be state.sync.
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var probe struct {
		Type     string             `json:"type"`
		Sessions []terminal.Session `json:"sessions"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if probe.Type != "state.sync" {
		t.Errorf("expected state.sync, got %q", probe.Type)
	}
	if len(probe.Sessions) != 1 || probe.Sessions[0].ID != "s1" {
		t.Errorf("unexpected sessions: %+v", probe.Sessions)
	}
}

func TestWSServer_LivePush(t *testing.T) {
	r := room.NewRoom(t.TempDir(), nil)
	defer r.Close()

	if _, err := r.CreateSession("s1", "test", "t1"); err != nil {
		t.Fatal(err)
	}

	wsURL := startTerminalWSServer(t, r)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Drain sync.
	conn.ReadMessage()

	// Append a message to the room — client should receive it.
	_ = r.AppendEvent("s1", terminal.MsgNewEvent{
		Type:    "msg.new",
		Session: "s1",
		SeqNo:   1,
		Msg: terminal.Message{
			ID:        "m1",
			SessionID: "s1",
			SeqNo:     1,
			Role:      "assistant",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Content:   terminal.Text("hello from ws"),
		},
	})

	// Read the pushed message.
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var evt struct {
		Type string `json:"type"`
	}
	json.Unmarshal(data, &evt)
	if evt.Type != "msg.new" {
		t.Errorf("expected msg.new, got %q", evt.Type)
	}
}

func TestWSServer_MultiClient(t *testing.T) {
	r := room.NewRoom(t.TempDir(), nil)
	defer r.Close()

	if _, err := r.CreateSession("s1", "test", "t1"); err != nil {
		t.Fatal(err)
	}

	wsURL := startTerminalWSServer(t, r)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Connect two clients.
	conn1, _, _ := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	defer conn1.Close()
	conn2, _, _ := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	defer conn2.Close()

	// Drain sync from both.
	conn1.ReadMessage()
	conn2.ReadMessage()

	// Push a message.
	_ = r.AppendEvent("s1", terminal.MsgNewEvent{
		Type:    "msg.new",
		Session: "s1",
		SeqNo:   1,
		Msg: terminal.Message{
			ID:        "m1",
			SessionID: "s1",
			SeqNo:     1,
			Role:      "system",
			Content:   terminal.System("info", "broadcast"),
		},
	})

	// Both clients should receive it.
	for i, conn := range []*websocket.Conn{conn1, conn2} {
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("client %d read: %v", i+1, err)
		}
		var evt struct{ Type string }
		json.Unmarshal(data, &evt)
		if evt.Type != "msg.new" {
			t.Errorf("client %d: expected msg.new, got %q", i+1, evt.Type)
		}
	}
}
