// SPDX-License-Identifier: Apache-2.0

package orchestration

import (
	"fmt"
	"sync"
	"time"

	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// TeammateMessage is a message between agents.
type TeammateMessage struct {
	ID        string    `json:"id"`
	From      string    `json:"from"`
	To        string    `json:"to"` // agent name or "*" for broadcast
	Text      string    `json:"text"`
	Summary   string    `json:"summary,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	Read      bool      `json:"read"`
	Color     string    `json:"color,omitempty"`
}

// MessageOutput is the result of sending a message.
type MessageOutput struct {
	Delivered bool   `json:"delivered"`
	To        string `json:"to"`
	MessageID string `json:"messageId"`
}

// MailboxManager manages per-agent message queues.
//
// Mailbox is pull-based: agents call ReadMailbox/ReadUnread to fetch messages.
// When an EventBus is attached via SetEventBus, SendMessage ALSO publishes
// AgentMessageEvent on the appropriate topic (to.<name>.message for direct,
// group.<team>.message for broadcast — if Team is set). This enables workflow
// nodes to subscribe to agent-to-agent messages via WaitSpec, bridging the
// mailbox-based API with the event-driven engine.
//
// Subscribe(agentName, teamName) sets up the inverse direction: events received
// on to.<agentName>.message and group.<teamName>.message are routed into the
// local mailbox. This is what makes cross-agent mailbox messaging actually work
// when each agent has its own MailboxManager instance.
type MailboxManager struct {
	mu        sync.RWMutex
	mailboxes map[string][]TeammateMessage // agent name → messages
	nextID    int
	bus       engine.EventBus // optional — nil means no event publishing
	team      string          // team name for group broadcasts (optional)
	unsubs    []func()        // unsubscribe functions for active topic subscriptions

	// UC-064: when true, the mailbox accepts incoming events from any team
	// (e.g. a coordinator that needs to receive escalations from every team
	// it manages). When false (the default), receiveEvent drops events whose
	// evt.Team doesn't match m.team. Set to true via SetCoordinatorMode in
	// handleCreateTeam — the team-leader / coordinator path. Teammates leave
	// it false so cross-team injection is structurally rejected.
	coordinatorMode bool
}

// NewMailboxManager creates a mailbox manager.
func NewMailboxManager() *MailboxManager {
	return &MailboxManager{
		mailboxes: make(map[string][]TeammateMessage),
	}
}

// SetEventBus attaches an EventBus for mirroring mailbox sends to topics.
// When set, each SendMessage also publishes:
//   - to.<to>.message for direct messages
//   - group.<team>.message for broadcasts (if team is configured via SetTeam)
//
// Callers can leave this nil to disable event publishing (pure mailbox mode).
func (m *MailboxManager) SetEventBus(bus engine.EventBus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bus = bus
}

// SetTeam sets the team name used for group.<team>.message broadcasts.
// Also used by UC-064's receiveEvent cross-team check — incoming events whose
// evt.Team differs from this value are dropped (unless coordinatorMode is on).
func (m *MailboxManager) SetTeam(team string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.team = team
}

// Team returns the team this mailbox broadcasts to (empty if unset). Used
// by the team-lens narrative to attribute lead-side messages to a team.
func (m *MailboxManager) Team() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.team
}

// SetCoordinatorMode toggles the receive-side cross-team-allow flag.
// When true (typically set on the team-leader / coordinator), the mailbox
// accepts every incoming AgentMessageEvent regardless of evt.Team —
// coordinators need to receive escalations from any team they manage and
// peer-coordinator messages across teams. When false (the default, used by
// teammates), receiveEvent drops events whose evt.Team mismatches m.team.
// UC-064 / threat-T3.
func (m *MailboxManager) SetCoordinatorMode(on bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.coordinatorMode = on
}

// Subscribe registers EventBus subscriptions that route incoming messages
// into the local mailbox. Subscribes to:
//   - to.<agentName>.message (direct messages for this agent)
//   - group.<teamName>.message (team broadcasts, if teamName is non-empty)
//
// Requires SetEventBus to have been called first (no-op otherwise).
// Call Unsubscribe() on shutdown to detach.
func (m *MailboxManager) Subscribe(agentName, teamName string) {
	m.mu.Lock()
	bus := m.bus
	m.mu.Unlock()
	if bus == nil || agentName == "" {
		return
	}

	unsub := bus.Subscribe(engine.AgentMessageTopic(agentName), func(_ string, payload any) {
		m.receiveEvent(agentName, payload)
	})
	m.mu.Lock()
	m.unsubs = append(m.unsubs, unsub)
	m.mu.Unlock()

	if teamName != "" {
		unsubGrp := bus.Subscribe(engine.GroupMessageTopic(teamName), func(_ string, payload any) {
			m.receiveEvent(agentName, payload)
		})
		m.mu.Lock()
		m.unsubs = append(m.unsubs, unsubGrp)
		m.mu.Unlock()
	}
}

// Unsubscribe detaches all subscriptions set up by Subscribe.
func (m *MailboxManager) Unsubscribe() {
	m.mu.Lock()
	unsubs := m.unsubs
	m.unsubs = nil
	m.mu.Unlock()
	for _, u := range unsubs {
		if u != nil {
			u()
		}
	}
}

// receiveEvent is called by the EventBus subscription handler. It pushes the
// AgentMessageEvent payload into the local mailbox under agentName.
//
// UC-064 / threat-T3: drops cross-team events when this mailbox represents a
// teammate (coordinatorMode=false) AND both the receiver's team and the
// event's team are set AND they differ. The event's Team field is server-
// derived from the publisher's m.team (set from `_team_name` loop meta),
// not LLM-controllable, so a malicious sender cannot bypass the check by
// faking Team in the JSON input.
func (m *MailboxManager) receiveEvent(agentName string, payload any) {
	evt, ok := payload.(*engine.AgentMessageEvent)
	if !ok || evt == nil {
		return
	}
	// Avoid re-inserting messages we just sent (loopback prevention via From check).
	// A message from ourselves to ourselves shouldn't echo back.
	if evt.From == agentName {
		return
	}

	// Cross-team drop. Reads m.team / m.coordinatorMode under read lock, then
	// re-acquires write lock below for the mailbox append. Two locks because
	// the early return is the common case for the dropped path.
	m.mu.RLock()
	myTeam := m.team
	coord := m.coordinatorMode
	bus := m.bus
	m.mu.RUnlock()
	if myTeam != "" && evt.Team != "" && evt.Team != myTeam && !coord {
		// Cross-team rejection — UC-064. Emit an audit-trail event so the drop
		// is no longer silent (R5+I1 / security T3 follow-up). The drop event
		// is best-effort: a nil bus skips emission. Message text is omitted
		// from the event to avoid leaking content into the audit channel.
		if bus != nil {
			bus.Publish(engine.MailboxCrossTeamDropTopic(), &engine.MailboxCrossTeamDropEvent{
				From:     evt.From,
				To:       agentName,
				FromTeam: evt.Team,
				ToTeam:   myTeam,
				Reason:   "sender team does not match receiver team (non-coordinator receiver)",
			})
		}
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	msgID := fmt.Sprintf("msg-%d", m.nextID)
	msg := TeammateMessage{
		ID:        msgID,
		From:      evt.From,
		To:        agentName,
		Text:      evt.Text,
		Summary:   evt.Summary,
		Timestamp: time.Now(),
		Read:      false,
	}
	m.mailboxes[agentName] = append(m.mailboxes[agentName], msg)
}

// SendMessage sends a message to a specific agent or broadcasts to all.
func (m *MailboxManager) SendMessage(from, to, text, summary, color string) (*MessageOutput, error) {
	if to == "" {
		return nil, fmt.Errorf("recipient is required")
	}
	if text == "" {
		return nil, fmt.Errorf("message text is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.nextID++
	msgID := fmt.Sprintf("msg-%d", m.nextID)

	msg := TeammateMessage{
		ID:        msgID,
		From:      from,
		To:        to,
		Text:      text,
		Summary:   summary,
		Timestamp: time.Now(),
		Read:      false,
		Color:     color,
	}

	if to == "*" {
		// Broadcast to all mailboxes
		for name := range m.mailboxes {
			if name != from {
				m.mailboxes[name] = append(m.mailboxes[name], msg)
			}
		}
	} else {
		m.mailboxes[to] = append(m.mailboxes[to], msg)
	}

	// Mirror to EventBus if attached.
	if m.bus != nil {
		event := &engine.AgentMessageEvent{
			From:    from,
			To:      to,
			Text:    text,
			Summary: summary,
			Team:    m.team,
		}
		if to == "*" && m.team != "" {
			m.bus.Publish(engine.GroupMessageTopic(m.team), event)
		} else if to != "*" {
			m.bus.Publish(engine.AgentMessageTopic(to), event)
		}
		// Flat mirror so the SessionRecorder journals the delivery (the
		// templated topics above carry the agent name and can't be subscribed
		// to by a fixed-topic recorder). Makes comms survive into the cold
		// session Team view, not just the live one.
		m.bus.Publish(engine.AgentMessageAuditTopic(), event)
	}

	return &MessageOutput{
		Delivered: true,
		To:        to,
		MessageID: msgID,
	}, nil
}

// ReadMailbox returns all messages for an agent.
func (m *MailboxManager) ReadMailbox(agentName string) []TeammateMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()
	msgs := m.mailboxes[agentName]
	result := make([]TeammateMessage, len(msgs))
	copy(result, msgs)
	return result
}

// ReadUnread returns only unread messages for an agent.
func (m *MailboxManager) ReadUnread(agentName string) []TeammateMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var unread []TeammateMessage
	for _, msg := range m.mailboxes[agentName] {
		if !msg.Read {
			unread = append(unread, msg)
		}
	}
	return unread
}

// MarkAsRead marks all messages for an agent as read.
func (m *MailboxManager) MarkAsRead(agentName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.mailboxes[agentName] {
		m.mailboxes[agentName][i].Read = true
	}
}

// EnsureMailbox creates a mailbox for an agent if it doesn't exist.
func (m *MailboxManager) EnsureMailbox(agentName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.mailboxes[agentName]; !ok {
		m.mailboxes[agentName] = nil
	}
}

// ClearMailbox removes all messages for an agent.
func (m *MailboxManager) ClearMailbox(agentName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.mailboxes, agentName)
}
