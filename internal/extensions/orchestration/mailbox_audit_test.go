// SPDX-License-Identifier: Apache-2.0

package orchestration

import (
	"sync"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// TestMailbox_CrossTeamDropEmitsAuditEvent (R5+I1) — when a cross-team
// message is dropped at receive, MailboxManager emits a
// MailboxCrossTeamDropEvent on MailboxCrossTeamDropTopic so operators
// (and the recorder) have an audit trail. Pre-R5+I1 the drop was silent.
func TestMailbox_CrossTeamDropEmitsAuditEvent(t *testing.T) {
	bus := eventbus.NewMemoryEventBus()

	// Capture every audit-drop event on the bus.
	var (
		mu    sync.Mutex
		drops []*engine.MailboxCrossTeamDropEvent
	)
	unsub := bus.Subscribe(engine.MailboxCrossTeamDropTopic(), func(_ string, payload any) {
		mu.Lock()
		defer mu.Unlock()
		if evt, ok := payload.(*engine.MailboxCrossTeamDropEvent); ok {
			drops = append(drops, evt)
		}
	})
	defer unsub()

	// Bob is in team beta, NOT a coordinator. He receives an event tagged
	// for team alpha — should be dropped + audit event emitted.
	bob := NewMailboxManager()
	bob.SetEventBus(bus)
	bob.SetTeam("beta")
	// coordinatorMode stays false → cross-team drop fires.

	bob.receiveEvent("bob", &engine.AgentMessageEvent{
		From:    "alice",
		To:      "bob",
		Team:    "alpha", // mismatches bob's "beta"
		Text:    "secret cross-team payload",
		Summary: "summary",
	})

	// Verify the message itself was dropped.
	if msgs := bob.ReadMailbox("bob"); len(msgs) != 0 {
		t.Errorf("bob's mailbox = %d messages, want 0 (cross-team should be dropped); got: %+v", len(msgs), msgs)
	}

	// Verify the audit event was emitted with the right fields.
	mu.Lock()
	defer mu.Unlock()
	if len(drops) != 1 {
		t.Fatalf("expected 1 audit drop event, got %d: %+v", len(drops), drops)
	}
	d := drops[0]
	if d.From != "alice" {
		t.Errorf("From = %q, want alice", d.From)
	}
	if d.To != "bob" {
		t.Errorf("To = %q, want bob", d.To)
	}
	if d.FromTeam != "alpha" {
		t.Errorf("FromTeam = %q, want alpha", d.FromTeam)
	}
	if d.ToTeam != "beta" {
		t.Errorf("ToTeam = %q, want beta", d.ToTeam)
	}
	if d.Reason == "" {
		t.Error("Reason should not be empty")
	}
	// Message text must NOT appear in the audit event (privacy guarantee).
	for _, field := range []string{d.From, d.To, d.FromTeam, d.ToTeam, d.Reason} {
		if field == "secret cross-team payload" {
			t.Error("audit event leaked the message text — only metadata should be captured")
		}
	}
}

// TestMailbox_CoordinatorAcceptDoesNotEmitDrop — when a coordinator
// receives a cross-team message (allowed by coordinatorMode=true), the
// drop topic is NOT touched. Regression guard so the audit channel
// doesn't spam on the legitimate cross-team coordinator flow.
func TestMailbox_CoordinatorAcceptDoesNotEmitDrop(t *testing.T) {
	bus := eventbus.NewMemoryEventBus()
	var (
		mu    sync.Mutex
		count int
	)
	unsub := bus.Subscribe(engine.MailboxCrossTeamDropTopic(), func(_ string, _ any) {
		mu.Lock()
		count++
		mu.Unlock()
	})
	defer unsub()

	coord := NewMailboxManager()
	coord.SetEventBus(bus)
	coord.SetTeam("beta")
	coord.SetCoordinatorMode(true) // accepts cross-team

	coord.receiveEvent("coord", &engine.AgentMessageEvent{
		From: "alice",
		To:   "coord",
		Team: "alpha",
		Text: "escalation",
	})

	mu.Lock()
	defer mu.Unlock()
	if count != 0 {
		t.Errorf("coordinator accept path should NOT emit drop event; got %d", count)
	}
}
