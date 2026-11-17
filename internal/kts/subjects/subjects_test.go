// SPDX-License-Identifier: Apache-2.0

package subjects

import (
	"strings"
	"testing"
)

// TestSubjectBuilders_LockSchema is a golden-style test that pins the subject
// schema. Changes to the produced strings here are intentional and require
// updating nats-architecture.md in lockstep.
func TestSubjectBuilders_LockSchema(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"tenant-event", TenantEventSubject("acme", "user-joined"), "ktw.acme.event.user-joined"},
		{"universe-event", UniverseEventSubject("acme", "client-x", "ready"), "ktw.acme.universe.client-x.event.ready"},
		{"slot", SlotSubject("acme", "client-x", "ktw-docs", "doc-service", "default", "render"),
			"ktw.acme.universe.client-x.slot.ktw-docs.doc-service.default.render"},
		{"slot-cancel", SlotCancelSubject("acme", "client-x", "ktw-docs"),
			"ktw.acme.universe.client-x.slot-cancel.ktw-docs"},
		{"panel-out", PanelOutSubject("acme", "client-x", "dp-0192f1"),
			"ktw.acme.universe.client-x.panel.dp-0192f1.out"},
		{"panel-in", PanelInSubject("acme", "client-x", "dp-0192f1"),
			"ktw.acme.universe.client-x.panel.dp-0192f1.in"},
		{"panels-pattern", PanelsPattern("acme", "client-x"),
			"ktw.acme.universe.client-x.panel.>"},
		{"slot-queue-group", SlotQueueGroup("ktw-docs", "doc-service", "default"),
			"slot:ktw-docs.doc-service.default"},
		{"sidecar-registration", SidecarRegistrationSubject("acme", "client-x", "ktw-example"),
			"ktw.acme.universe.client-x.registration.sidecar.ktw-example"},
		{"sidecar-descriptor", SidecarDescriptorSubject("acme", "client-x", "ktw-example"),
			"ktw.acme.universe.client-x.descriptor.ktw-example"},

		{"room-event", RoomEventSubject("acme", "client-x", "alpha", "topic-changed"),
			"ktw.acme.universe.client-x.room.alpha.event.topic-changed"},
		{"room-member-joined", RoomMemberSubject("acme", "client-x", "alpha", "joined"),
			"ktw.acme.universe.client-x.room.alpha.member.joined"},
		{"room-session-attached", RoomSessionAttachedSubject("acme", "client-x", "alpha"),
			"ktw.acme.universe.client-x.room.alpha.session.attached"},
		{"room-session-detached", RoomSessionDetachedSubject("acme", "client-x", "alpha"),
			"ktw.acme.universe.client-x.room.alpha.session.detached"},
		{"room-session-access-changed", RoomSessionAccessChangedSubject("acme", "client-x", "alpha"),
			"ktw.acme.universe.client-x.room.alpha.session.access-changed"},

		{"session-event", SessionEventSubject("acme", "client-x", "s1", "completed"),
			"ktw.acme.universe.client-x.session.s1.event.completed"},
		{"session-message", SessionMessageSubject("acme", "client-x", "s1"),
			"ktw.acme.universe.client-x.session.s1.message"},
		{"session-agent-message", SessionAgentMessageSubject("acme", "client-x", "s1", "researcher"),
			"ktw.acme.universe.client-x.session.s1.to.researcher"},
		{"session-group", SessionGroupSubject("acme", "client-x", "s1", "design-team"),
			"ktw.acme.universe.client-x.session.s1.group.design-team"},

		{"task-event", TaskEventSubject("acme", "client-x", "s1", "t1", "log"),
			"ktw.acme.universe.client-x.session.s1.task.t1.event.log"},
		{"task-lifecycle-started", TaskLifecycleSubject("acme", "client-x", "s1", "t1", "started"),
			"ktw.acme.universe.client-x.session.s1.task.t1.started"},
		{"task-child-lifecycle", TaskChildLifecycleSubject("acme", "client-x", "s1", "t1", "t2", "done"),
			"ktw.acme.universe.client-x.session.s1.task.t1.child.t2.done"},
		{"task-protocol", TaskProtocolSubject("acme", "client-x", "s1", "t1", "abc-123"),
			"ktw.acme.universe.client-x.session.s1.task.t1.protocol.abc-123"},
		{"task-ledger", TaskLedgerSubject("acme", "client-x", "s1", "t1", "lg1", "snapshot"),
			"ktw.acme.universe.client-x.session.s1.task.t1.ledger.lg1.snapshot"},

		{"user-notification", UserNotificationSubject("acme", "client-x", "alice"),
			"ktw.acme.universe.client-x.user.alice.notification"},
		{"user-invite", UserInviteSubject("acme", "client-x", "alice"),
			"ktw.acme.universe.client-x.user.alice.invite"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, c.got, c.want)
		}
	}
}

// TestSubjectBuilders_TokenValidation panics on bad input. We assert
// recovery to verify the validation, not just observe the panic.
func TestSubjectBuilders_TokenValidation(t *testing.T) {
	cases := []struct {
		name string
		fn   func()
	}{
		{"uppercase tenant", func() { TenantEventSubject("Acme", "x") }},
		{"dot in token", func() { TenantEventSubject("ac.me", "x") }},
		{"empty token", func() { TenantEventSubject("", "x") }},
		{"too long", func() { TenantEventSubject(strings.Repeat("a", 64), "x") }},
		// `slash in method` was a panic case under the old single-segment
		// rule. cwm-v2-providers US-15 (2026-06-08): `/` in method is
		// now a valid segment separator (validated per-segment by
		// `mustMethodPath`). A SLASH IN ANY OTHER token still panics —
		// see the SlotQueueGroup/`slash in provider` case below.
		{"slash in provider", func() { SlotQueueGroup("ktw/docs", "doc-service", "default") }},
		{"empty middle segment in method", func() { SlotSubject("acme", "u", "ktw-docs", "doc-service", "default", "a//b") }},
		{"underscore in token", func() { TaskEventSubject("acme", "u", "s1", "t1", "log_event") }},
		{"dot in a panel id", func() { PanelOutSubject("acme", "u", "dp.1") }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("expected panic for %q", c.name)
				}
			}()
			c.fn()
		})
	}
}

// TestSubjectBuilders_AllStartWithKtw — every subject starts with ktw.<tenant>
// per the architecture ADR.
func TestSubjectBuilders_AllStartWithKtw(t *testing.T) {
	subjects := []string{
		TenantEventSubject("t", "e"),
		UniverseEventSubject("t", "u", "e"),
		SlotSubject("t", "u", "p", "s", "i", "m"),
		SlotCancelSubject("t", "u", "p"),
		PanelOutSubject("t", "u", "dp-1"),
		PanelInSubject("t", "u", "dp-1"),
		PanelsPattern("t", "u"),
		SidecarRegistrationSubject("t", "u", "n"),
		SidecarDescriptorSubject("t", "u", "n"),
		RoomEventSubject("t", "u", "r", "e"),
		RoomMemberSubject("t", "u", "r", "joined"),
		RoomSessionAttachedSubject("t", "u", "r"),
		RoomSessionDetachedSubject("t", "u", "r"),
		RoomSessionAccessChangedSubject("t", "u", "r"),
		SessionEventSubject("t", "u", "s", "e"),
		SessionMessageSubject("t", "u", "s"),
		SessionAgentMessageSubject("t", "u", "s", "n"),
		SessionGroupSubject("t", "u", "s", "g"),
		TaskEventSubject("t", "u", "s", "tk", "e"),
		TaskLifecycleSubject("t", "u", "s", "tk", "done"),
		TaskChildLifecycleSubject("t", "u", "s", "p", "c", "done"),
		TaskProtocolSubject("t", "u", "s", "tk", "id"),
		TaskLedgerSubject("t", "u", "s", "tk", "lg", "snapshot"),
		UserNotificationSubject("t", "u", "uid"),
		UserInviteSubject("t", "u", "uid"),
	}
	for _, s := range subjects {
		if !strings.HasPrefix(s, "ktw.t.") {
			t.Errorf("subject does not start with ktw.<tenant>: %q", s)
		}
	}
}
