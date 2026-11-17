// SPDX-License-Identifier: Apache-2.0

package orchestration

import (
	corereflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// --- Typed input structs for orchestration tools ---

type CreateTeamInput struct {
	Name        string `json:"name"                  jsonschema:"Team name" required:"true"`
	Description string `json:"description,omitempty" jsonschema:"What this team does"`
}

type SpawnTeammateInput struct {
	Team     string `json:"team"                 jsonschema:"Team name to join" required:"true"`
	TaskName string `json:"task_name,omitempty"  jsonschema:"Registered task to run (optional if prompt is provided)"`
	Prompt   string `json:"prompt,omitempty"     jsonschema:"Instructions for the teammate (required when task_name is omitted)"`
	Name     string `json:"name,omitempty"       jsonschema:"Teammate name (auto-generated if empty)"`
}

type ListTeamInput struct {
	Team string `json:"team" jsonschema:"Team name" required:"true"`
}

type CompleteTeamInput struct {
	Team    string `json:"team"              jsonschema:"Team name" required:"true"`
	Failed  bool   `json:"failed,omitempty"  jsonschema:"true if the team's work failed"`
	Summary string `json:"summary,omitempty" jsonschema:"Brief outcome summary for the team narrative"`
}

type SendMessageInput struct {
	To      string `json:"to"                jsonschema:"Recipient name or '*' for broadcast" required:"true"`
	Message string `json:"message"           jsonschema:"Message text" required:"true"`
	Summary string `json:"summary,omitempty" jsonschema:"Brief summary for notification"`
}

type CheckMessagesInput struct {
	AgentName string `json:"agent_name" jsonschema:"Your agent name" required:"true"`
}

type RequestPermissionInput struct {
	ToolName    string `json:"tool_name"           jsonschema:"Tool requesting permission for" required:"true"`
	Description string `json:"description"         jsonschema:"What you want to do and why" required:"true"`
	Input       string `json:"input,omitempty"      jsonschema:"The tool input (for review)"`
}

type ResolvePermissionInput struct {
	RequestID string `json:"request_id"          jsonschema:"Permission request ID" required:"true"`
	Decision  string `json:"decision"            jsonschema:"approve or deny" required:"true"`
	Reason    string `json:"reason,omitempty"    jsonschema:"Reason for the decision"`
}

type ListPermissionsInput struct {
	// No fields — lists all pending permissions
}

// AnswerChildInput answers a child agent's bubbled ask-user question (you are
// acting as the child's user).
type AnswerChildInput struct {
	QuestionID string `json:"question_id" jsonschema:"The question_id from the child's question" required:"true"`
	Answer     string `json:"answer"      jsonschema:"Your answer to the child's question" required:"true"`
}

// AwaitChildInput blocks until a delegated child's next lifecycle event.
type AwaitChildInput struct {
	TaskID   string `json:"task_id,omitempty"   jsonschema:"The child task_id from delegate. Omit to wait for ANY of your children's next event."`
	TimeoutS int    `json:"timeout_s,omitempty" jsonschema:"Max seconds to block before returning 'timeout' (default 120). On timeout just call await-child again."`
}

// StopChildInput ends a driven (finish:driven) child you delegated.
type StopChildInput struct {
	TaskID string `json:"task_id"           jsonschema:"The child task_id from delegate" required:"true"`
	Reason string `json:"reason,omitempty"  jsonschema:"Why you're ending it (recorded; e.g. 'wizard finished, deliverable produced')"`
}

// --- ToolDef builders ---

func orchToolDefs() []agentapi.ToolDef {
	return []agentapi.ToolDef{
		{
			Name:        "create-team",
			Description: "Create a named team for coordinating multiple agents.",
			InputSchema: corereflect.MustJSONSchema[CreateTeamInput](),
		},
		{
			Name:        "spawn-teammate",
			Description: "Spawn a teammate in a team. Runs as a background agent.",
			InputSchema: corereflect.MustJSONSchema[SpawnTeammateInput](),
		},
		{
			Name:        "list-team",
			Description: "List members and status of a team.",
			InputSchema: corereflect.MustJSONSchema[ListTeamInput](),
		},
		{
			Name:        "complete-team",
			Description: "Declare a team's work finished (or failed) — closes the team lifecycle and records the outcome in the session narrative. Team-leader only.",
			InputSchema: corereflect.MustJSONSchema[CompleteTeamInput](),
		},
		{
			Name:        "send-message",
			Description: "Send a message to a teammate by name, or broadcast to all with '*'.",
			InputSchema: corereflect.MustJSONSchema[SendMessageInput](),
		},
		{
			Name:        "check-messages",
			Description: "Check your mailbox for unread messages.",
			InputSchema: corereflect.MustJSONSchema[CheckMessagesInput](),
			Polling:     true,
		},
		{
			Name:        "request-permission",
			Description: "Request permission from the team leader for a tool use.",
			InputSchema: corereflect.MustJSONSchema[RequestPermissionInput](),
		},
		{
			Name:        "resolve-permission",
			Description: "Approve or deny a permission request (team-leader only).",
			InputSchema: corereflect.MustJSONSchema[ResolvePermissionInput](),
		},
		{
			Name:        "list-permissions",
			Description: "List all pending permission requests.",
			InputSchema: corereflect.MustJSONSchema[ListPermissionsInput](),
		},
		{
			Name:        "answer-child",
			Description: "Answer a delegated agent's question (when a child task routes its ask-user to you — you act as its user). Use the question_id surfaced in the conversation.",
			InputSchema: corereflect.MustJSONSchema[AnswerChildInput](),
		},
		{
			Name:        "await-child",
			Description: "Wait (BLOCKING) until a delegated child's next event: it asks you a question, completes, or fails. Returns 'asked' (then a question appears — answer it with answer-child), 'completed', 'failed', or 'timeout'. Call this between answers to drive a child WITHOUT polling task-status. On 'timeout', just call await-child again.",
			InputSchema: corereflect.MustJSONSchema[AwaitChildInput](),
			Blocking:    true,
		},
		{
			Name:        "stop-child",
			Description: "End a DRIVEN child you delegated (finish:driven — an open chat that never completes itself). Hard-stops it so your await-child returns. Use when the conversation is done. A finish:self child completes on its own — you don't need this for it.",
			InputSchema: corereflect.MustJSONSchema[StopChildInput](),
		},
	}
}
