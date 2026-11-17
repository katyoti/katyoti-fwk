// SPDX-License-Identifier: Apache-2.0

// Package subjects is the CROSS-PROCESS NATS address space: a leaf (the
// standard library only), so a binary that only talks on the bus links
// nothing of the task engine.
//
// Builder functions for every NATS subject the project produces. The
// SIBLING address space is the task engine's `topics.go` — the flat,
// in-process EventBus vocabulary used on engine.Channel within a single process. These two
// are distinct transports, not a rename (see the topics.go header for the
// boundary). Subjects here carry full tenant/universe/session scope
// because they cross process boundaries.
//
// Liveness: the TRANSPORT subjects (slot req/rep, registration,
// descriptor, connstate-changed, contributions-changed) are wired today.
// The session/task EVENT subjects (SessionAgentMessageSubject,
// SessionGroupSubject, TaskLifecycleSubject, TaskProtocolSubject,
// TaskLedgerSubject, TaskChildLifecycleSubject) are RESERVED for the
// deferred in-process→NATS event bridge; each `// Replaces today's
// <topic>` note is one row of the topic↔subject mapping (full table in
// modules/bus/nats_subjects_list.md).
//
// Per rule `nats-subject-builders-only`: never construct subject strings
// inline; always go through these builders (code-review enforced — the
// `import-discipline` lint covers NATS-client import scope, not inline
// strings).
//
// Subject hierarchy (longer prefix = narrower scope), as modules/bus/concepts.md
// describes it:
//
//	ktw.<tenant>.event.<type>                                              # tenant-scoped
//	ktw.<tenant>.universe.<u>.event.<type>                                 # universe-scoped
//	ktw.<tenant>.universe.<u>.slot.<provider>.<slottype>.<instance>.<method> # slot req/rep
//	ktw.<tenant>.universe.<u>.room.<r>.event.<type>                        # room broadcast
//	ktw.<tenant>.universe.<u>.session.<sid>.event.<type>                   # session-scoped
//	ktw.<tenant>.universe.<u>.session.<sid>.task.<tid>.<action>            # task lifecycle
//	ktw.<tenant>.universe.<u>.session.<sid>.task.<tid>.child.<child_tid>.<action>
//	ktw.<tenant>.universe.<u>.user.<uid>.notification                      # user notifications
//
// Token charset is [a-z0-9-]{1,63}. Builders panic on invalid tokens —
// fmt.Sprintf would silently produce broken subjects otherwise.
package subjects

import (
	"fmt"
	"regexp"
	"strings"
)

// tokenPattern is the strict charset enforced on every subject token.
// Lowercase + digits + hyphen, 1..63 chars. Matches the tenant-id charset
// declared in nats-architecture.md.
var tokenPattern = regexp.MustCompile(`^[a-z0-9-]{1,63}$`)

func mustToken(name, value string) {
	if !tokenPattern.MatchString(value) {
		panic(fmt.Sprintf("subject builder: invalid %s %q (must match [a-z0-9-]{1,63})", name, value))
	}
}

// ----------------------------------------------------------------------------
// Tenant scope
// ----------------------------------------------------------------------------

// TenantEventSubject builds a tenant-wide admin event subject.
//
//	ktw.<tenant>.event.<type>
func TenantEventSubject(tenantID, eventType string) string {
	mustToken("tenantID", tenantID)
	mustToken("eventType", eventType)
	return fmt.Sprintf("ktw.%s.event.%s", tenantID, eventType)
}

// ----------------------------------------------------------------------------
// Universe scope
// ----------------------------------------------------------------------------

// UniverseEventSubject builds a universe-wide event subject.
//
//	ktw.<tenant>.universe.<u>.event.<type>
func UniverseEventSubject(tenantID, universeID, eventType string) string {
	mustToken("tenantID", tenantID)
	mustToken("universeID", universeID)
	mustToken("eventType", eventType)
	return fmt.Sprintf("ktw.%s.universe.%s.event.%s", tenantID, universeID, eventType)
}

// SlotSubject builds the req/rep subject for a specific
// (provider, slottype, instance, method) tuple. Carries full addressing
// for the named-slot scheme: every responder is uniquely identified by
// the (provider, slottype, instance) prefix, eliminating the prior
// "every sidecar publishing slot 'resources' shares one queue group"
// collision.
//
//	ktw.<tenant>.universe.<u>.slot.<provider>.<slottype>.<instance>.<method>
//
// Token conventions:
//   - provider  — the binary's self-provider name (e.g. "ktw-agents")
//   - slottype  — the kebab `BusName` from `meta.ServiceDef`
//     (e.g. "resource-service", "terminal")
//   - instance  — the named-service instance name (typically "default";
//     dynamic instances like "session-abc123" allowed)
//   - method    — the kebab method name (e.g. "start-task", "list")
func SlotSubject(tenantID, universeID, provider, slottype, instance, method string) string {
	mustToken("tenantID", tenantID)
	mustToken("universeID", universeID)
	mustToken("provider", provider)
	mustToken("slottype", slottype)
	mustToken("instance", instance)
	mustMethodPath("method", method)
	// Multi-segment methods: `/` separators become `.` so NATS
	// wildcards apply per segment (cwm-v2-providers US-15, 2026-06-08).
	// Single-segment methods are unchanged.
	methodSubject := strings.ReplaceAll(method, "/", ".")
	return fmt.Sprintf("ktw.%s.universe.%s.slot.%s.%s.%s.%s",
		tenantID, universeID, provider, slottype, instance, methodSubject)
}

// SlotCancelSubject builds the subject a KTSP caller publishes on when it
// no longer needs a call it made to `provider`: a section scrolled away, a
// refresh superseding a read, a page closed. Every replica of the provider
// listens (a plain subscription, not the slot's queue group: the replica
// serving the call is not known to the caller) and cancels the context of
// the call it is serving under that id; the others ignore it.
//
//	ktw.<tenant>.universe.<u>.slot-cancel.<provider>
//
// Payload: `{"call_id": "<the id the call carried in its ktw.meta.callId header>"}`.
func SlotCancelSubject(tenantID, universeID, provider string) string {
	mustToken("tenantID", tenantID)
	mustToken("universeID", universeID)
	mustToken("provider", provider)
	return fmt.Sprintf("ktw.%s.universe.%s.slot-cancel.%s", tenantID, universeID, provider)
}

// PanelOutSubject builds the subject a display panel's events go out on,
// from the task that owns it: open, ops
// progress, ask, complete, close, each carrying the panel's sequence number.
// The panel server and every screen showing the panel subscribe.
//
//	ktw.<tenant>.universe.<u>.panel.<panelID>.out
func PanelOutSubject(tenantID, universeID, panelID string) string {
	mustToken("tenantID", tenantID)
	mustToken("universeID", universeID)
	mustToken("panelID", panelID)
	return fmt.Sprintf("ktw.%s.universe.%s.panel.%s.out", tenantID, universeID, panelID)
}

// PanelInSubject builds the subject the person's input to a display panel
// comes back on: submit, action, message, cancel. The run owning the panel
// waits on it; the panel server records it.
//
//	ktw.<tenant>.universe.<u>.panel.<panelID>.in
func PanelInSubject(tenantID, universeID, panelID string) string {
	mustToken("tenantID", tenantID)
	mustToken("universeID", universeID)
	mustToken("panelID", panelID)
	return fmt.Sprintf("ktw.%s.universe.%s.panel.%s.in", tenantID, universeID, panelID)
}

// PanelsPattern is the pattern of every display panel subject of a universe,
// out and in - what the panel server subscribes to.
//
//	ktw.<tenant>.universe.<u>.panel.>
func PanelsPattern(tenantID, universeID string) string {
	mustToken("tenantID", tenantID)
	mustToken("universeID", universeID)
	return fmt.Sprintf("ktw.%s.universe.%s.panel.>", tenantID, universeID)
}

// mustMethodPath validates a method path: one or more kebab tokens
// separated by `/`. Each segment must be a valid subject token.
// cwm-v2-providers US-15 (2026-06-08).
func mustMethodPath(name, value string) {
	if value == "" {
		panic(fmt.Sprintf("subject builder: empty %s", name))
	}
	for i, seg := range strings.Split(value, "/") {
		mustToken(fmt.Sprintf("%s[%d]", name, i), seg)
	}
}

// SlotQueueGroup is the canonical queue-group name for one named slot
// instance. With provider + instance embedded in the group, true
// replicas of the same logical instance (multi-process / multi-host)
// load-balance correctly; different sidecars publishing the same
// slottype no longer collide on the same group.
//
//	"slot:<provider>.<slottype>.<instance>"
func SlotQueueGroup(provider, slottype, instance string) string {
	mustToken("provider", provider)
	mustToken("slottype", slottype)
	mustToken("instance", instance)
	return "slot:" + provider + "." + slottype + "." + instance
}

// ConnStateChangedSubject builds the universe-wide subject the
// ConnectionManager publishes on whenever a sidecar's lifecycle state
// changes (DECLARED / SPAWNING / READY / IDLE_DRAINING / FAILED).
// Subscribers (GUI Inspector, observability, MCP refresh, …) listen
// here for "sidecar X just transitioned" — replaces the per-host
// `sidecar.disconnected` notification described in analysis §3.5 with
// a richer general-purpose event.
//
//	ktw.<tenant>.universe.<u>.event.connstate-changed
func ConnStateChangedSubject(tenantID, universeID string) string {
	return UniverseEventSubject(tenantID, universeID, "connstate-changed")
}

// ContributionsChangedSubject builds the universe-wide subject the
// catalog publishes on whenever the loaded Contributions set changes
// (initial readiness, refresh after a sidecar produces a new manifest
// cache file, or a single-provider reload). Subscribers (agent prompt
// builder, GUI catalog viewer, MCP bridge) listen here to refresh
// their materialised view without polling.
//
//	ktw.<tenant>.universe.<u>.event.contributions-changed
func ContributionsChangedSubject(tenantID, universeID string) string {
	return UniverseEventSubject(tenantID, universeID, "contributions-changed")
}

// RunLifecycleSubject is where a universe's task service publishes every
// transition of its runs (a RunLifecycleEvent, redacted): a GUI's Runs pane
// refreshes on it instead of polling.
//
//	ktw.<tenant>.universe.<u>.event.run-lifecycle
func RunLifecycleSubject(tenantID, universeID string) string {
	return UniverseEventSubject(tenantID, universeID, "run-lifecycle")
}

// UniversesChangedSubject builds the subject the orchestrator publishes on
// whenever the UNIVERSE REGISTRY changes -- a universe created, deleted,
// renamed, attached, or its sidecar membership edited.
//
// It is published on the ADMIN universe because the registry is the control
// plane's own state, not any one universe's: a GUI or CLI that just created
// "foo" is not connected to "foo" yet, but it is always connected to admin.
// Subscribers (the GUI's universe ribbon) refresh their list instead of
// polling -- without this, a universe created from a terminal stays invisible
// in an open GUI until something else happens to trigger a re-fetch.
//
//	ktw.<tenant>.universe.admin.event.universes-changed
func UniversesChangedSubject(tenantID, universeID string) string {
	return UniverseEventSubject(tenantID, universeID, "universes-changed")
}

// ProfileChangedSubject builds the subject the profile service publishes on
// after the profile is updated. Like the universe registry, the profile is
// control-plane state and is published on the ADMIN universe: `katyoti profile
// init` in a terminal updates an open GUI's menu without a restart.
//
//	ktw.<tenant>.universe.admin.event.profile-changed
func ProfileChangedSubject(tenantID, universeID string) string {
	return UniverseEventSubject(tenantID, universeID, "profile-changed")
}

// SidecarRegistrationSubject builds the sidecar self-registration subject.
// Persisted to the KTW_REGISTRATION stream (last-message-per-subject retention).
//
//	ktw.<tenant>.universe.<u>.registration.sidecar.<name>
func SidecarRegistrationSubject(tenantID, universeID, sidecarName string) string {
	mustToken("tenantID", tenantID)
	mustToken("universeID", universeID)
	mustToken("sidecarName", sidecarName)
	return fmt.Sprintf("ktw.%s.universe.%s.registration.sidecar.%s",
		tenantID, universeID, sidecarName)
}

// SidecarDescriptorSubject builds the descriptor-announce subject.
// Cached in the KTW_DESCRIPTOR KV.
//
//	ktw.<tenant>.universe.<u>.descriptor.<name>
func SidecarDescriptorSubject(tenantID, universeID, sidecarName string) string {
	mustToken("tenantID", tenantID)
	mustToken("universeID", universeID)
	mustToken("sidecarName", sidecarName)
	return fmt.Sprintf("ktw.%s.universe.%s.descriptor.%s",
		tenantID, universeID, sidecarName)
}

// ----------------------------------------------------------------------------
// Room scope (collaboration; sharing lifecycle)
// ----------------------------------------------------------------------------

// RoomEventSubject builds a room-wide event subject (delivered to all room members).
//
//	ktw.<tenant>.universe.<u>.room.<r>.event.<type>
func RoomEventSubject(tenantID, universeID, roomID, eventType string) string {
	mustToken("tenantID", tenantID)
	mustToken("universeID", universeID)
	mustToken("roomID", roomID)
	mustToken("eventType", eventType)
	return fmt.Sprintf("ktw.%s.universe.%s.room.%s.event.%s",
		tenantID, universeID, roomID, eventType)
}

// RoomMemberSubject builds a member-lifecycle subject. action ∈ joined / left / role-changed.
//
//	ktw.<tenant>.universe.<u>.room.<r>.member.<action>
func RoomMemberSubject(tenantID, universeID, roomID, action string) string {
	mustToken("tenantID", tenantID)
	mustToken("universeID", universeID)
	mustToken("roomID", roomID)
	mustToken("action", action)
	return fmt.Sprintf("ktw.%s.universe.%s.room.%s.member.%s",
		tenantID, universeID, roomID, action)
}

// RoomSessionAttachedSubject signals "a session has been attached to this room."
//
//	ktw.<tenant>.universe.<u>.room.<r>.session.attached
func RoomSessionAttachedSubject(tenantID, universeID, roomID string) string {
	mustToken("tenantID", tenantID)
	mustToken("universeID", universeID)
	mustToken("roomID", roomID)
	return fmt.Sprintf("ktw.%s.universe.%s.room.%s.session.attached",
		tenantID, universeID, roomID)
}

// RoomSessionDetachedSubject signals detachment.
//
//	ktw.<tenant>.universe.<u>.room.<r>.session.detached
func RoomSessionDetachedSubject(tenantID, universeID, roomID string) string {
	mustToken("tenantID", tenantID)
	mustToken("universeID", universeID)
	mustToken("roomID", roomID)
	return fmt.Sprintf("ktw.%s.universe.%s.room.%s.session.detached",
		tenantID, universeID, roomID)
}

// RoomSessionAccessChangedSubject signals an access-level change.
//
//	ktw.<tenant>.universe.<u>.room.<r>.session.access-changed
func RoomSessionAccessChangedSubject(tenantID, universeID, roomID string) string {
	mustToken("tenantID", tenantID)
	mustToken("universeID", universeID)
	mustToken("roomID", roomID)
	return fmt.Sprintf("ktw.%s.universe.%s.room.%s.session.access-changed",
		tenantID, universeID, roomID)
}

// ----------------------------------------------------------------------------
// Session scope (FLAT — room attachment is metadata, not in path)
// ----------------------------------------------------------------------------

// SessionEventSubject builds a session-level event subject.
//
//	ktw.<tenant>.universe.<u>.session.<sid>.event.<type>
func SessionEventSubject(tenantID, universeID, sessionID, eventType string) string {
	mustToken("tenantID", tenantID)
	mustToken("universeID", universeID)
	mustToken("sessionID", sessionID)
	mustToken("eventType", eventType)
	return fmt.Sprintf("ktw.%s.universe.%s.session.%s.event.%s",
		tenantID, universeID, sessionID, eventType)
}

// SessionMessageSubject builds the user/agent message stream subject for a session.
//
//	ktw.<tenant>.universe.<u>.session.<sid>.message
func SessionMessageSubject(tenantID, universeID, sessionID string) string {
	mustToken("tenantID", tenantID)
	mustToken("universeID", universeID)
	mustToken("sessionID", sessionID)
	return fmt.Sprintf("ktw.%s.universe.%s.session.%s.message",
		tenantID, universeID, sessionID)
}

// SessionAgentMessageSubject builds the direct-mailbox subject for a named agent.
// (Replaces today's to.<name>.message topic.)
//
//	ktw.<tenant>.universe.<u>.session.<sid>.to.<name>
func SessionAgentMessageSubject(tenantID, universeID, sessionID, agentName string) string {
	mustToken("tenantID", tenantID)
	mustToken("universeID", universeID)
	mustToken("sessionID", sessionID)
	mustToken("agentName", agentName)
	return fmt.Sprintf("ktw.%s.universe.%s.session.%s.to.%s",
		tenantID, universeID, sessionID, agentName)
}

// SessionGroupSubject builds the team-broadcast subject within a session.
// (Replaces today's group.<team>.message topic.)
//
//	ktw.<tenant>.universe.<u>.session.<sid>.group.<team>
func SessionGroupSubject(tenantID, universeID, sessionID, team string) string {
	mustToken("tenantID", tenantID)
	mustToken("universeID", universeID)
	mustToken("sessionID", sessionID)
	mustToken("team", team)
	return fmt.Sprintf("ktw.%s.universe.%s.session.%s.group.%s",
		tenantID, universeID, sessionID, team)
}

// ----------------------------------------------------------------------------
// Task scope (within session)
// ----------------------------------------------------------------------------

// TaskEventSubject builds a typed task-event subject.
//
//	ktw.<tenant>.universe.<u>.session.<sid>.task.<tid>.event.<type>
func TaskEventSubject(tenantID, universeID, sessionID, taskID, eventType string) string {
	mustToken("tenantID", tenantID)
	mustToken("universeID", universeID)
	mustToken("sessionID", sessionID)
	mustToken("taskID", taskID)
	mustToken("eventType", eventType)
	return fmt.Sprintf("ktw.%s.universe.%s.session.%s.task.%s.event.%s",
		tenantID, universeID, sessionID, taskID, eventType)
}

// TaskLifecycleSubject builds the task lifecycle subject. action ∈
// started / done / failed / suspended / cancelled. Workflow events ride here
// (workflow is a task strategy, not a separate namespace).
//
//	ktw.<tenant>.universe.<u>.session.<sid>.task.<tid>.<action>
func TaskLifecycleSubject(tenantID, universeID, sessionID, taskID, action string) string {
	mustToken("tenantID", tenantID)
	mustToken("universeID", universeID)
	mustToken("sessionID", sessionID)
	mustToken("taskID", taskID)
	mustToken("action", action)
	return fmt.Sprintf("ktw.%s.universe.%s.session.%s.task.%s.%s",
		tenantID, universeID, sessionID, taskID, action)
}

// TaskChildLifecycleSubject builds the child-task lifecycle subject.
//
//	ktw.<tenant>.universe.<u>.session.<sid>.task.<parent_tid>.child.<child_tid>.<action>
func TaskChildLifecycleSubject(tenantID, universeID, sessionID, parentTaskID, childTaskID, action string) string {
	mustToken("tenantID", tenantID)
	mustToken("universeID", universeID)
	mustToken("sessionID", sessionID)
	mustToken("parentTaskID", parentTaskID)
	mustToken("childTaskID", childTaskID)
	mustToken("action", action)
	return fmt.Sprintf("ktw.%s.universe.%s.session.%s.task.%s.child.%s.%s",
		tenantID, universeID, sessionID, parentTaskID, childTaskID, action)
}

// TaskProtocolSubject builds the protocol-exchange subject. The protocol kind
// (plan-approval / permission / shutdown / …) is carried in the event payload,
// NOT in the subject path. correlationID ties request to response.
//
//	ktw.<tenant>.universe.<u>.session.<sid>.task.<tid>.protocol.<id>
func TaskProtocolSubject(tenantID, universeID, sessionID, taskID, correlationID string) string {
	mustToken("tenantID", tenantID)
	mustToken("universeID", universeID)
	mustToken("sessionID", sessionID)
	mustToken("taskID", taskID)
	mustToken("correlationID", correlationID)
	return fmt.Sprintf("ktw.%s.universe.%s.session.%s.task.%s.protocol.%s",
		tenantID, universeID, sessionID, taskID, correlationID)
}

// TaskLedgerSubject builds the ledger update subject.
//
//	ktw.<tenant>.universe.<u>.session.<sid>.task.<tid>.ledger.<id>.<topic>
func TaskLedgerSubject(tenantID, universeID, sessionID, taskID, ledgerID, topic string) string {
	mustToken("tenantID", tenantID)
	mustToken("universeID", universeID)
	mustToken("sessionID", sessionID)
	mustToken("taskID", taskID)
	mustToken("ledgerID", ledgerID)
	mustToken("topic", topic)
	return fmt.Sprintf("ktw.%s.universe.%s.session.%s.task.%s.ledger.%s.%s",
		tenantID, universeID, sessionID, taskID, ledgerID, topic)
}

// ----------------------------------------------------------------------------
// User scope (cross-session notifications)
// ----------------------------------------------------------------------------

// UserNotificationSubject builds a user-notification subject.
//
//	ktw.<tenant>.universe.<u>.user.<uid>.notification
func UserNotificationSubject(tenantID, universeID, userID string) string {
	mustToken("tenantID", tenantID)
	mustToken("universeID", universeID)
	mustToken("userID", userID)
	return fmt.Sprintf("ktw.%s.universe.%s.user.%s.notification",
		tenantID, universeID, userID)
}

// UserInviteSubject builds a user-invite subject (room/session invitations).
//
//	ktw.<tenant>.universe.<u>.user.<uid>.invite
func UserInviteSubject(tenantID, universeID, userID string) string {
	mustToken("tenantID", tenantID)
	mustToken("universeID", universeID)
	mustToken("userID", userID)
	return fmt.Sprintf("ktw.%s.universe.%s.user.%s.invite",
		tenantID, universeID, userID)
}
