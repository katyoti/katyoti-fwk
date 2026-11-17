// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"testing"
	"time"
)

func TestTopicConstructors(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		// Child lifecycle
		{"ChildStartedTopic", ChildStartedTopic("researcher"), "child.researcher.started"},
		{"ChildDoneTopic", ChildDoneTopic("researcher"), "child.researcher.done"},
		{"ChildFailedTopic", ChildFailedTopic("researcher"), "child.researcher.failed"},
		{"ChildSuspendedTopic", ChildSuspendedTopic("researcher"), "child.researcher.suspended"},
		{"ChildProgressTopic", ChildProgressTopic("researcher"), "child.researcher.progress"},
		{"ChildCancelTopic", ChildCancelTopic("researcher"), "to.researcher.cancel"},

		// Agent-to-agent
		{"AgentMessageTopic", AgentMessageTopic("alice"), "to.alice.message"},
		{"AgentTaskTopic", AgentTaskTopic("alice"), "to.alice.task"},
		{"AgentResultTopic", AgentResultTopic("alice"), "to.alice.result"},

		// Group
		{"GroupMessageTopic", GroupMessageTopic("ingest"), "group.ingest.message"},
		{"GroupAnnouncementTopic", GroupAnnouncementTopic("ingest"), "group.ingest.announcement"},

		// Workflow lifecycle
		{"WorkflowStartedTopic", WorkflowStartedTopic("run-abc"), "workflow.run-abc.started"},
		{"WorkflowDoneTopic", WorkflowDoneTopic("run-abc"), "workflow.run-abc.done"},
		{"WorkflowFailedTopic", WorkflowFailedTopic("run-abc"), "workflow.run-abc.failed"},

		// Protocols
		{"PlanApprovalTopic", PlanApprovalTopic("plan-1"), "protocol.plan-1.plan-approval"},
		{"PermissionTopic", PermissionTopic("perm-1"), "protocol.perm-1.permission"},
		{"ShutdownTopic", ShutdownTopic("shut-1"), "protocol.shut-1.shutdown"},

		// Ledger
		{"LedgerSnapshotTopic", LedgerSnapshotTopic("led-1"), "ledger.led-1.snapshot"},
		{"LedgerUpdateTopic", LedgerUpdateTopic("led-1"), "ledger.led-1.update"},
		{"LedgerCompleteTopic", LedgerCompleteTopic("led-1"), "ledger.led-1.complete"},

		// Cancel request
		{"CancelRequestTopic", CancelRequestTopic("run-abc"), "cancel.request.run-abc"},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

func TestEventStamper_AllEvents(t *testing.T) {
	// Verify each event type implements EventStamper correctly.
	events := []EventStamper{
		&AgentMessageEvent{},
		&AgentTaskEvent{},
		&AgentResultEvent{},
		&ChildProgressEvent{},
		&WorkflowStartedEvent{},
		&WorkflowDoneEvent{},
		&WorkflowFailedEvent{},
		&PlanApprovalRequest{},
		&PlanApprovalResponse{},
		&PermissionRequest{},
		&PermissionResponse{},
		&ShutdownRequest{},
		&ShutdownResponse{},
		&LedgerSnapshotEvent{},
		&LedgerUpdateEvent{},
		&LedgerCompleteEvent{},
	}

	for _, e := range events {
		e.StampEvent("my.topic", "run-123")
	}

	// Spot-check one — verify StampEvent actually sets fields.
	evt := &AgentMessageEvent{From: "alice", To: "bob", Text: "hello"}
	before := time.Now()
	evt.StampEvent("to.bob.message", "run-1")
	after := time.Now()

	if evt.Type != "to.bob.message" {
		t.Errorf("Type = %q", evt.Type)
	}
	if evt.Emitter != "run-1" {
		t.Errorf("Emitter = %q", evt.Emitter)
	}
	if evt.Timestamp.Before(before) || evt.Timestamp.After(after) {
		t.Errorf("Timestamp = %v, outside [%v, %v]", evt.Timestamp, before, after)
	}
}

func TestAgentMessageEvent_Fields(t *testing.T) {
	evt := &AgentMessageEvent{
		From: "alice",
		To:   "bob",
		Text: "hello",
		Team: "engineering",
	}
	evt.StampEvent(AgentMessageTopic("bob"), "alice")

	if evt.From != "alice" {
		t.Errorf("From = %q", evt.From)
	}
	if evt.To != "bob" {
		t.Errorf("To = %q", evt.To)
	}
	if evt.Team != "engineering" {
		t.Errorf("Team = %q", evt.Team)
	}
}

func TestProtocolCorrelation(t *testing.T) {
	// Request and response share the same correlation ID.
	req := &PlanApprovalRequest{
		ID:        "plan-42",
		AgentName: "coordinator",
		Plan:      "draft plan text",
	}
	resp := &PlanApprovalResponse{
		ID:       "plan-42",
		Approved: true,
	}

	if req.ID != resp.ID {
		t.Errorf("correlation ID mismatch: req=%q resp=%q", req.ID, resp.ID)
	}
}
