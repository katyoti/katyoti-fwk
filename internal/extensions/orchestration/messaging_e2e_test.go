// SPDX-License-Identifier: Apache-2.0

package orchestration

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
)

// T7 — end-to-end cross-agent mailbox delivery via the EventBus bridge.
//
// Acks (covered by existing tests, not duplicated here):
//   - Cross-team direct drop:          messaging_test.go::TestReceive_DropsCrossTeamFromTeammate
//   - Same-team direct delivery:       messaging_test.go::TestReceive_AcceptsSameTeam
//   - Coordinator accepts cross-team:  messaging_test.go::TestReceive_CoordinatorAcceptsCrossTeam
//   - No-team bypass:                  messaging_test.go::TestReceive_NoTeamSetBypassesCheck
//   - Cross-team drop emits audit event: mailbox_audit_test.go::TestMailbox_CrossTeamDropEmitsAuditEvent
//
// Fresh tests in THIS file fill the remaining T7 gaps:
//   - Full message round-trip (From/Text/Summary preserved across the bus)
//   - Group broadcast (`SendMessage(from, "*", ...)` reaches every team member's mailbox)
//   - Sender excluded from broadcast (no self-echo)

// TestE2E_DirectDelivery_RoundTrip — Alice (alpha) → Bob (alpha) over the
// shared bus. Verify EVERY field round-trips through the topic bridge,
// not just len(msgs)>0 as the existing TestReceive_AcceptsSameTeam does.
func TestE2E_DirectDelivery_RoundTrip(t *testing.T) {
	bus := eventbus.NewMemoryEventBus()

	bob := NewMailboxManager()
	bob.SetEventBus(bus)
	bob.SetTeam("alpha")
	bob.EnsureMailbox("bob")
	bob.Subscribe("bob", "alpha")

	alice := NewMailboxManager()
	alice.SetEventBus(bus)
	alice.SetTeam("alpha")

	if _, err := alice.SendMessage("alice", "bob", "the body text", "the summary", "blue"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	msgs := bob.ReadMailbox("bob")
	if len(msgs) != 1 {
		t.Fatalf("bob's mailbox = %d, want 1", len(msgs))
	}
	got := msgs[0]
	if got.From != "alice" {
		t.Errorf("From = %q, want alice", got.From)
	}
	if got.To != "bob" {
		t.Errorf("To = %q, want bob", got.To)
	}
	if got.Text != "the body text" {
		t.Errorf("Text = %q, want 'the body text'", got.Text)
	}
	if got.Summary != "the summary" {
		t.Errorf("Summary = %q, want 'the summary'", got.Summary)
	}
}

// TestE2E_GroupBroadcast_AllTeammatesReceive — Alice in alpha sends to "*"
// with team=alpha; bob + carol (both alpha) receive via the
// group.alpha.message topic. Verifies the GroupMessageTopic bridge that
// SendMessage publishes when to=="*" and SetTeam is configured.
func TestE2E_GroupBroadcast_AllTeammatesReceive(t *testing.T) {
	bus := eventbus.NewMemoryEventBus()

	bob := NewMailboxManager()
	bob.SetEventBus(bus)
	bob.SetTeam("alpha")
	bob.EnsureMailbox("bob")
	bob.Subscribe("bob", "alpha")

	carol := NewMailboxManager()
	carol.SetEventBus(bus)
	carol.SetTeam("alpha")
	carol.EnsureMailbox("carol")
	carol.Subscribe("carol", "alpha")

	alice := NewMailboxManager()
	alice.SetEventBus(bus)
	alice.SetTeam("alpha")

	if _, err := alice.SendMessage("alice", "*", "team-wide announcement", "summary", ""); err != nil {
		t.Fatalf("SendMessage broadcast: %v", err)
	}

	for name, mgr := range map[string]*MailboxManager{"bob": bob, "carol": carol} {
		msgs := mgr.ReadMailbox(name)
		if len(msgs) != 1 {
			t.Errorf("%s's mailbox = %d after broadcast, want 1", name, len(msgs))
			continue
		}
		if msgs[0].Text != "team-wide announcement" {
			t.Errorf("%s message text = %q, want 'team-wide announcement'", name, msgs[0].Text)
		}
	}
}

// TestE2E_GroupBroadcast_SenderExcluded — when Alice broadcasts to "*" in
// her team, her OWN mailbox stays empty (no self-echo). Guards the
// loopback-prevention `if evt.From == agentName { return }` in receiveEvent.
func TestE2E_GroupBroadcast_SenderExcluded(t *testing.T) {
	bus := eventbus.NewMemoryEventBus()

	alice := NewMailboxManager()
	alice.SetEventBus(bus)
	alice.SetTeam("alpha")
	alice.EnsureMailbox("alice")
	alice.Subscribe("alice", "alpha")

	bob := NewMailboxManager()
	bob.SetEventBus(bus)
	bob.SetTeam("alpha")
	bob.EnsureMailbox("bob")
	bob.Subscribe("bob", "alpha")

	if _, err := alice.SendMessage("alice", "*", "self test", "", ""); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	if got := len(alice.ReadMailbox("alice")); got != 0 {
		t.Errorf("alice's mailbox = %d after own broadcast, want 0 (no self-echo)", got)
	}
	if got := len(bob.ReadMailbox("bob")); got != 1 {
		t.Errorf("bob's mailbox = %d, want 1 (broadcast reaches teammate)", got)
	}
}

// TestE2E_GroupBroadcast_CrossTeamMemberDoesNotReceive — Alice (alpha)
// broadcasts to her team; Dave (beta) is subscribed to group.beta.message
// only, so Alice's group.alpha.message event never reaches him. Pin the
// per-team broadcast scoping behavior.
func TestE2E_GroupBroadcast_CrossTeamMemberDoesNotReceive(t *testing.T) {
	bus := eventbus.NewMemoryEventBus()

	bob := NewMailboxManager()
	bob.SetEventBus(bus)
	bob.SetTeam("alpha")
	bob.EnsureMailbox("bob")
	bob.Subscribe("bob", "alpha")

	dave := NewMailboxManager()
	dave.SetEventBus(bus)
	dave.SetTeam("beta")
	dave.EnsureMailbox("dave")
	dave.Subscribe("dave", "beta")

	alice := NewMailboxManager()
	alice.SetEventBus(bus)
	alice.SetTeam("alpha")

	if _, err := alice.SendMessage("alice", "*", "alpha-team only", "", ""); err != nil {
		t.Fatalf("SendMessage broadcast: %v", err)
	}

	if got := len(bob.ReadMailbox("bob")); got != 1 {
		t.Errorf("bob (alpha) mailbox = %d, want 1", got)
	}
	if got := len(dave.ReadMailbox("dave")); got != 0 {
		t.Errorf("dave (beta) mailbox = %d, want 0 (cross-team broadcast must not arrive)", got)
	}
}
