// SPDX-License-Identifier: Apache-2.0

package orchestration

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// stubLoopContext is a minimal agentapi.LoopContext for tests of the
// orchestration plugin. Only GetMeta/SetMeta exercise real state; the rest
// return zero values. Sufficient for handler-level unit tests that don't
// need a full agent runtime.
type stubLoopContext struct {
	meta map[string]any
}

func newStubLoop() *stubLoopContext {
	return &stubLoopContext{meta: map[string]any{}}
}

func (s *stubLoopContext) GetMeta(key string) any               { return s.meta[key] }
func (s *stubLoopContext) SetMeta(key string, value any)        { s.meta[key] = value }
func (s *stubLoopContext) Messages() []agentapi.Message         { return nil }
func (s *stubLoopContext) PushMessage(_ agentapi.Message)       {}
func (s *stubLoopContext) ReplaceMessages(_ []agentapi.Message) {}
func (s *stubLoopContext) AbortSignal() <-chan struct{}         { return nil }
func (s *stubLoopContext) SetModelOverride(_ string)            {}
func (s *stubLoopContext) SetMaxTokens(_ int)                   {}
func (s *stubLoopContext) SetThinkingBudget(_ int)              {}
func (s *stubLoopContext) ToolDefs() []agentapi.ToolDef         { return nil }
func (s *stubLoopContext) Turn() int                            { return 0 }
func (s *stubLoopContext) TotalUsage() agentapi.Usage           { return agentapi.Usage{} }
func (s *stubLoopContext) Mode() string                         { return "" }
func (s *stubLoopContext) IsChat() bool                         { return false }
func (s *stubLoopContext) RunID() string                        { return "" }
func (s *stubLoopContext) InheritedPlugins() []any              { return nil }
func (s *stubLoopContext) PermissionProfile() string            { return "" }

// newOrchPluginForTest constructs a minimal orchPlugin for handler tests.
// Only the fields used by the handler under test are wired; nil for the rest.
func newOrchPluginForTest(loop *stubLoopContext, mailboxMgr *MailboxManager) *orchPlugin {
	return &orchPlugin{
		loop:       loop,
		mailboxMgr: mailboxMgr,
	}
}

func TestMailbox_PullWithoutEventBus(t *testing.T) {
	m := NewMailboxManager()
	_, err := m.SendMessage("alice", "bob", "hello bob", "", "")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	msgs := m.ReadMailbox("bob")
	if len(msgs) != 1 {
		t.Fatalf("ReadMailbox(bob) len = %d, want 1", len(msgs))
	}
	if msgs[0].Text != "hello bob" {
		t.Errorf("Text = %q", msgs[0].Text)
	}
}

func TestMailbox_BridgePublishesToTopic(t *testing.T) {
	bus := eventbus.NewMemoryEventBus()

	// Subscribe to the direct topic to verify publish.
	var received []*engine.AgentMessageEvent
	unsub := bus.Subscribe(engine.AgentMessageTopic("bob"), func(_ string, payload any) {
		if evt, ok := payload.(*engine.AgentMessageEvent); ok {
			received = append(received, evt)
		}
	})
	defer unsub()

	m := NewMailboxManager()
	m.SetEventBus(bus)

	_, err := m.SendMessage("alice", "bob", "direct message", "", "")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	if len(received) != 1 {
		t.Fatalf("received %d events, want 1", len(received))
	}
	if received[0].From != "alice" || received[0].To != "bob" || received[0].Text != "direct message" {
		t.Errorf("event fields wrong: %+v", received[0])
	}
}

func TestMailbox_GroupBroadcastPublishesToGroupTopic(t *testing.T) {
	bus := eventbus.NewMemoryEventBus()

	var received []*engine.AgentMessageEvent
	unsub := bus.Subscribe(engine.GroupMessageTopic("alpha"), func(_ string, payload any) {
		if evt, ok := payload.(*engine.AgentMessageEvent); ok {
			received = append(received, evt)
		}
	})
	defer unsub()

	m := NewMailboxManager()
	m.SetEventBus(bus)
	m.SetTeam("alpha")

	// Ensure some mailboxes exist so broadcast has targets.
	m.EnsureMailbox("bob")
	m.EnsureMailbox("carol")

	_, err := m.SendMessage("alice", "*", "team broadcast", "", "")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	if len(received) != 1 {
		t.Fatalf("group events = %d, want 1", len(received))
	}
	if received[0].Team != "alpha" {
		t.Errorf("Team = %q, want alpha", received[0].Team)
	}
}

func TestMailbox_SubscribeRoutesEventsIntoLocalMailbox(t *testing.T) {
	bus := eventbus.NewMemoryEventBus()

	// Bob's mailbox — attached to bus, subscribed to to.bob.message.
	bobMailbox := NewMailboxManager()
	bobMailbox.SetEventBus(bus)
	bobMailbox.Subscribe("bob", "")
	defer bobMailbox.Unsubscribe()

	// Alice's mailbox — on the same bus (simulates cross-agent messaging).
	aliceMailbox := NewMailboxManager()
	aliceMailbox.SetEventBus(bus)

	// Alice sends to Bob.
	_, err := aliceMailbox.SendMessage("alice", "bob", "hi bob from alice", "", "")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	// Bob reads his mailbox — the message should have been routed via topic.
	msgs := bobMailbox.ReadMailbox("bob")
	if len(msgs) != 1 {
		t.Fatalf("bob mailbox len = %d, want 1 (message not routed via topic)", len(msgs))
	}
	if msgs[0].Text != "hi bob from alice" {
		t.Errorf("text = %q", msgs[0].Text)
	}
}

func TestMailbox_SubscribeReceivesGroupBroadcast(t *testing.T) {
	bus := eventbus.NewMemoryEventBus()

	bobMailbox := NewMailboxManager()
	bobMailbox.SetEventBus(bus)
	bobMailbox.Subscribe("bob", "alpha") // in team alpha
	defer bobMailbox.Unsubscribe()

	aliceMailbox := NewMailboxManager()
	aliceMailbox.SetEventBus(bus)
	aliceMailbox.SetTeam("alpha")
	aliceMailbox.EnsureMailbox("bob")

	_, err := aliceMailbox.SendMessage("alice", "*", "team ping", "", "")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	msgs := bobMailbox.ReadMailbox("bob")
	if len(msgs) != 1 {
		t.Fatalf("bob mailbox len = %d, want 1", len(msgs))
	}
	if msgs[0].Text != "team ping" {
		t.Errorf("text = %q", msgs[0].Text)
	}
}

// ────────────────────────────────────────────────────────────────────────
// SendMessage RateLimiter tests — UC-111
// ((modules/tasks/decisions/OD-036-when-does-what-a-step-does-become-visible-to.cwm.md) execution rule 12: send_message enforces per-agent rate.)
// ────────────────────────────────────────────────────────────────────────

// TestSendMessage_FromDerived — UC-063, audit findings sec-3 + threat-T3 (partial).
//
// A teammate (alice) sends a message; the receiver must see From == "alice"
// even though the LLM cannot supply From and the previous code hardcoded
// "coordinator". Verifies handleSendMessage uses senderIdentity() backed by
// _teammate_name loop meta.
func TestSendMessage_FromDerived(t *testing.T) {
	loop := newStubLoop()
	loop.SetMeta("_teammate_name", "alice")
	loop.SetMeta("_team_name", "alpha")

	mb := NewMailboxManager()
	mb.EnsureMailbox("bob")

	p := newOrchPluginForTest(loop, mb)

	input := json.RawMessage(`{"to":"bob","message":"hello bob","summary":""}`)
	if _, err := p.handleSendMessage(input); err != nil {
		t.Fatalf("handleSendMessage: %v", err)
	}

	msgs := mb.ReadMailbox("bob")
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message in bob's mailbox, got %d", len(msgs))
	}
	if got, want := msgs[0].From, "alice"; got != want {
		t.Errorf("From = %q, want %q (must derive from _teammate_name, not hardcoded coordinator)", got, want)
	}
	if got := msgs[0].Text; got != "hello bob" {
		t.Errorf("Text = %q, want %q", got, "hello bob")
	}
}

// TestSendMessage_FromCoordinator_Fallback — regression guard.
//
// A coordinator agent has no _teammate_name in loop meta (it's the parent,
// not a teammate). senderIdentity must fall back to "coordinator" so existing
// coordinator-driven flows keep working.
func TestSendMessage_FromCoordinator_Fallback(t *testing.T) {
	loop := newStubLoop() // no _teammate_name set

	mb := NewMailboxManager()
	mb.EnsureMailbox("alice")

	p := newOrchPluginForTest(loop, mb)

	input := json.RawMessage(`{"to":"alice","message":"task assignment","summary":""}`)
	if _, err := p.handleSendMessage(input); err != nil {
		t.Fatalf("handleSendMessage: %v", err)
	}

	msgs := mb.ReadMailbox("alice")
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message in alice's mailbox, got %d", len(msgs))
	}
	if got, want := msgs[0].From, "coordinator"; got != want {
		t.Errorf("coordinator fallback: From = %q, want %q", got, want)
	}
}

// TestSendMessage_LLMSuppliedFromIgnored — negative DoD for UC-063.
//
// Even if the LLM emits a `from` field in the JSON input, it must be
// silently dropped (the input schema has no `from` field by design) and the
// derived sender identity must be used. This is the core anti-spoofing
// guarantee: From cannot be forged via the tool input.
func TestSendMessage_LLMSuppliedFromIgnored(t *testing.T) {
	loop := newStubLoop()
	loop.SetMeta("_teammate_name", "alice")

	mb := NewMailboxManager()
	mb.EnsureMailbox("bob")

	p := newOrchPluginForTest(loop, mb)

	// Crafted input: LLM tries to spoof From="coordinator" while alice is
	// the actual sender. The struct unmarshal silently drops the unknown
	// field; senderIdentity() then provides the authoritative From.
	input := json.RawMessage(
		`{"to":"bob","from":"coordinator","message":"spoofed!","summary":""}`,
	)
	if _, err := p.handleSendMessage(input); err != nil {
		t.Fatalf("handleSendMessage: %v", err)
	}

	msgs := mb.ReadMailbox("bob")
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if got := msgs[0].From; got != "alice" {
		t.Errorf("From = %q, want %q (LLM-supplied from must be ignored)", got, "alice")
	}
}

// Compile-time: ensure stubLoopContext implements LoopContext + ctx import is used.
var _ agentapi.LoopContext = (*stubLoopContext)(nil)
var _ = context.TODO // keep import for future tests that may need it

// ── UC-064: cross-team rejection ──────────────────────────────────────

// TestReceive_DropsCrossTeamFromTeammate — UC-064, audit threat-T3 (other half).
//
// Bob (team beta, teammate, coordinatorMode=false) receives an event whose
// evt.Team is "alpha". The receive path must DROP it before adding to mailbox.
func TestReceive_DropsCrossTeamFromTeammate(t *testing.T) {
	bus := eventbus.NewMemoryEventBus()

	bob := NewMailboxManager()
	bob.SetEventBus(bus)
	bob.SetTeam("beta") // bob is in team beta
	// coordinatorMode left false — bob is a teammate, not a coordinator
	bob.EnsureMailbox("bob")
	bob.Subscribe("bob", "beta")

	// Alice in team alpha sends to bob. Without SetEventBus on alice's mailbox,
	// publish via the bus directly — same effect.
	alice := NewMailboxManager()
	alice.SetEventBus(bus)
	alice.SetTeam("alpha")

	if _, err := alice.SendMessage("alice", "bob", "cross-team injection", "", ""); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	// bob's mailbox MUST be empty — the cross-team event was dropped at receive.
	msgs := bob.ReadMailbox("bob")
	if len(msgs) != 0 {
		t.Errorf("bob's mailbox = %d messages, want 0 (cross-team should be dropped); got: %+v", len(msgs), msgs)
	}
}

// TestReceive_AcceptsSameTeam — sanity check the rejection isn't over-eager.
// Alice in alpha → carol in alpha must succeed.
func TestReceive_AcceptsSameTeam(t *testing.T) {
	bus := eventbus.NewMemoryEventBus()

	carol := NewMailboxManager()
	carol.SetEventBus(bus)
	carol.SetTeam("alpha")
	carol.EnsureMailbox("carol")
	carol.Subscribe("carol", "alpha")

	alice := NewMailboxManager()
	alice.SetEventBus(bus)
	alice.SetTeam("alpha")

	if _, err := alice.SendMessage("alice", "carol", "in-team msg", "", ""); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if got := len(carol.ReadMailbox("carol")); got != 1 {
		t.Errorf("carol's mailbox = %d, want 1 (same-team should pass)", got)
	}
}

// TestReceive_CoordinatorAcceptsCrossTeam — UC-064 coordinator exception.
// Bob the team-leader of beta (coordinatorMode=true) accepts a message whose
// evt.Team=="alpha" (peer coordinator from another team).
func TestReceive_CoordinatorAcceptsCrossTeam(t *testing.T) {
	bus := eventbus.NewMemoryEventBus()

	bob := NewMailboxManager()
	bob.SetEventBus(bus)
	bob.SetTeam("beta")
	bob.SetCoordinatorMode(true) // bob is a team-leader, accepts all teams
	bob.EnsureMailbox("bob")
	bob.Subscribe("bob", "beta")

	alice := NewMailboxManager()
	alice.SetEventBus(bus)
	alice.SetTeam("alpha")

	if _, err := alice.SendMessage("alice-coord", "bob", "cross-team peer-coord msg", "", ""); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if got := len(bob.ReadMailbox("bob")); got != 1 {
		t.Errorf("coordinator bob's mailbox = %d, want 1 (coordinatorMode should accept cross-team)", got)
	}
}

// TestReceive_NoTeamSetBypassesCheck — root agents (no team configured) still
// receive everything. Pre-existing behavior must not regress.
func TestReceive_NoTeamSetBypassesCheck(t *testing.T) {
	bus := eventbus.NewMemoryEventBus()

	root := NewMailboxManager()
	root.SetEventBus(bus)
	// no SetTeam — m.team is empty
	root.EnsureMailbox("root")
	root.Subscribe("root", "")

	alice := NewMailboxManager()
	alice.SetEventBus(bus)
	alice.SetTeam("alpha")

	if _, err := alice.SendMessage("alice", "root", "no-team-receiver msg", "", ""); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if got := len(root.ReadMailbox("root")); got != 1 {
		t.Errorf("root mailbox = %d, want 1 (empty m.team should accept any sender)", got)
	}
}
