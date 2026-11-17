// SPDX-License-Identifier: Apache-2.0

package room_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal"
	termadapter "github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal/adapter"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal/room"
)

// ── helpers ───────────────────────────────────────────────────────────────

// newTestRoom creates a Room with a temporary journal directory.
func newTestRoom(t *testing.T) *room.Room {
	t.Helper()
	return room.NewRoom(t.TempDir(), nil)
}

// recvWithTimeout reads one raw message from a TerminalTransport or fails.
func recvOne(t *testing.T, ctx context.Context, ch <-chan json.RawMessage) json.RawMessage {
	t.Helper()
	select {
	case msg, ok := <-ch:
		if !ok {
			t.Fatal("receive channel closed unexpectedly")
		}
		return msg
	case <-ctx.Done():
		t.Fatalf("recvOne timeout: %v", ctx.Err())
		return nil
	}
}

// ── Session management ────────────────────────────────────────────────────

func TestRoom_CreateSession(t *testing.T) {
	r := newTestRoom(t)
	defer r.Close()

	sess, err := r.CreateSession("s1", "spec-extract", "task-1")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if sess.ID != "s1" || sess.Name != "spec-extract" || sess.Status != "running" {
		t.Errorf("unexpected session: %+v", sess)
	}
}

func TestRoom_CreateSession_DuplicateError(t *testing.T) {
	r := newTestRoom(t)
	defer r.Close()

	if _, err := r.CreateSession("s1", "task-a", "t1"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := r.CreateSession("s1", "task-b", "t2"); err == nil {
		t.Error("expected error for duplicate session ID, got nil")
	}
}

func TestRoom_ListSessions(t *testing.T) {
	r := newTestRoom(t)
	defer r.Close()

	if _, err := r.CreateSession("s1", "a", "t1"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CreateSession("s2", "b", "t2"); err != nil {
		t.Fatal(err)
	}

	sessions := r.ListSessions()
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

// ── Message log ───────────────────────────────────────────────────────────

func TestRoom_AppendEvent_History(t *testing.T) {
	r := newTestRoom(t)
	defer r.Close()

	if _, err := r.CreateSession("s1", "test", "t1"); err != nil {
		t.Fatal(err)
	}

	evt := terminal.MsgNewEvent{
		Type:    "msg.new",
		Session: "s1",
		SeqNo:   0,
		Msg: terminal.Message{
			ID:        "m1",
			SessionID: "s1",
			SeqNo:     0,
			Role:      "assistant",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Content:   terminal.Text("Hello"),
		},
	}
	if err := r.AppendEvent("s1", evt); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	history, err := r.GetHistory("s1")
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(history) != 1 {
		t.Errorf("expected 1 message in history, got %d", len(history))
	}
}

func TestRoom_AppendEvent_UnknownSession(t *testing.T) {
	r := newTestRoom(t)
	defer r.Close()

	err := r.AppendEvent("no-such-session", "event")
	if err == nil {
		t.Error("expected error for unknown session, got nil")
	}
}

// ── Connect / sync ────────────────────────────────────────────────────────

func TestRoom_Connect_ReceivesSync(t *testing.T) {
	r := newTestRoom(t)
	defer r.Close()

	if _, err := r.CreateSession("s1", "test", "t1"); err != nil {
		t.Fatal(err)
	}

	server, client := termadapter.NewChannelPair(16)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := r.Connect(ctx, server, ""); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// First message from Room should be the StateSyncEvent.
	ch, err := client.Receive(ctx)
	if err != nil {
		t.Fatalf("client Receive: %v", err)
	}
	raw := recvOne(t, ctx, ch)

	var probe struct {
		Type     string             `json:"type"`
		Sessions []terminal.Session `json:"sessions"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		t.Fatalf("unmarshal sync: %v", err)
	}
	if probe.Type != "state.sync" {
		t.Errorf("expected state.sync, got %q", probe.Type)
	}
	if len(probe.Sessions) != 1 {
		t.Errorf("expected 1 session in sync, got %d", len(probe.Sessions))
	}
}

func TestRoom_Connect_FanOut(t *testing.T) {
	r := newTestRoom(t)
	defer r.Close()

	if _, err := r.CreateSession("s1", "test", "t1"); err != nil {
		t.Fatal(err)
	}

	// Connect two clients
	server1, client1 := termadapter.NewChannelPair(16)
	server2, client2 := termadapter.NewChannelPair(16)
	defer server1.Close()
	defer server2.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := r.Connect(ctx, server1, ""); err != nil {
		t.Fatalf("Connect 1: %v", err)
	}
	if err := r.Connect(ctx, server2, ""); err != nil {
		t.Fatalf("Connect 2: %v", err)
	}

	ch1, _ := client1.Receive(ctx)
	ch2, _ := client2.Receive(ctx)

	// Drain the StateSyncEvent from both clients first.
	recvOne(t, ctx, ch1)
	recvOne(t, ctx, ch2)

	// Append an event — both clients should receive it.
	evt := terminal.MsgNewEvent{
		Type:    "msg.new",
		Session: "s1",
		SeqNo:   1,
		Msg: terminal.Message{
			ID:        "m1",
			SessionID: "s1",
			SeqNo:     1,
			Role:      "assistant",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Content:   terminal.Text("broadcast"),
		},
	}
	if err := r.AppendEvent("s1", evt); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	raw1 := recvOne(t, ctx, ch1)
	raw2 := recvOne(t, ctx, ch2)

	for i, raw := range []json.RawMessage{raw1, raw2} {
		var probe struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &probe); err != nil {
			t.Fatalf("client %d: unmarshal: %v", i+1, err)
		}
		if probe.Type != "msg.new" {
			t.Errorf("client %d: expected msg.new, got %q", i+1, probe.Type)
		}
	}
}

// ── SendMessageCmd dispatch ──────────────────────────────────────────────

// TestRoom_SendMessageCmd_Publishes verifies that a SendMessageCmd received
// from a connected transport is dispatched into the bus as an
// AgentMessageEvent on the recipient's topic with sender "gui-user".
func TestRoom_SendMessageCmd_Publishes(t *testing.T) {
	bus := eventbus.NewMemoryEventBus()
	r := room.NewRoom(t.TempDir(), nil)
	r.SetBus(bus)
	t.Cleanup(func() { r.Close() })

	if _, err := r.CreateSession("s1", "test", ""); err != nil {
		t.Fatal(err)
	}

	// Subscribe to the recipient's mailbox topic before sending.
	gotCh := make(chan *engine.AgentMessageEvent, 1)
	bus.Subscribe(engine.AgentMessageTopic("alice"), func(_ string, payload any) {
		if e, ok := payload.(*engine.AgentMessageEvent); ok {
			select {
			case gotCh <- e:
			default:
			}
		}
	})

	// Wire a transport pair and connect it to the room. The room's command
	// dispatch goroutine reads from the server side, so we send our cmd via
	// the client side.
	server, client := termadapter.NewChannelPair(64)
	t.Cleanup(func() { server.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := r.Connect(ctx, server, ""); err != nil {
		t.Fatal(err)
	}

	// Drain the initial state.sync.
	rxCh, _ := client.Receive(ctx)
	_ = recvOne(t, ctx, rxCh)

	if err := client.Send(ctx, terminal.SendMessageCmd{
		Type: "send_message",
		Params: terminal.SendMessageOpt{
			To:      "alice",
			Message: "hello from gui",
		},
	}); err != nil {
		t.Fatalf("send: %v", err)
	}

	select {
	case got := <-gotCh:
		if got.From != "gui-user" {
			t.Errorf("From = %q, want 'gui-user'", got.From)
		}
		if got.To != "alice" {
			t.Errorf("To = %q, want 'alice'", got.To)
		}
		if got.Text != "hello from gui" {
			t.Errorf("Text = %q", got.Text)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for AgentMessageEvent on bus")
	}
}
