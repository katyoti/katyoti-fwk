// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
)

// PermissionBehavior is the outcome of a tool's permission check.
//
// PermissionAsk is intentionally the zero value (fail-closed per spec FR-01):
// a tool that returns PermissionResult{} without explicitly setting Behavior
// will prompt the user rather than auto-allow.
//
// See decision (modules/tools/decisions/OD-001-what-shapes-does-an-agent-tool-take-and-through.cwm.md).
type PermissionBehavior int

const (
	// PermissionAsk — the user must approve before the tool executes. The kernel resolves
	// HumanGate via application.UseService[HumanGate](ctx) and prompts.
	// This is the zero value: fail-closed default per spec FR-01.
	PermissionAsk PermissionBehavior = iota

	// PermissionAllow — the tool may execute. ModifiedInput on the result, if non-empty,
	// replaces the original input.
	PermissionAllow

	// PermissionDeny — the tool is blocked. The kernel emits an error tool_result with
	// the result Message.
	PermissionDeny

	// PermissionPassthrough — MCP only. The kernel delegates the permission decision to
	// the upstream MCP server. Reserved for ms04-p3 MCP work.
	PermissionPassthrough
)

// String returns a stable lowercase identifier for logs and tool_result error messages.
func (p PermissionBehavior) String() string {
	switch p {
	case PermissionAllow:
		return "allow"
	case PermissionAsk:
		return "ask"
	case PermissionDeny:
		return "deny"
	case PermissionPassthrough:
		return "passthrough"
	default:
		return "unknown"
	}
}

// PermissionResult is the structured outcome of a tool's CheckPermissions call.
//
// Behavior is required. Message is required for PermissionAsk and PermissionDeny.
// ModifiedInput is honored only on PermissionAllow and replaces the original input
// before ToolCall is invoked.
//
// Suggestions and Dimensions feed the permission rules system
// (the permission profiles, decision — modules/agent/tools.md §5):
//
//   - Dimensions exposes a flat map of named values the kernel can match
//     against permission profile rules. Tools populate it with the parts of
//     their input that user-facing rules care about — e.g. file_read sets
//     `{"absolute_path": "/etc/hosts"}` so a rule like
//     `file_read.absolute_path:/etc/**` can match. When a tool returns Ask
//     without populating Dimensions, the kernel falls back to
//     DefaultDimensionExtractor (top-level JSON string fields).
//
//   - Suggestions are tool-proposed "always allow" rule strings that the
//     PromptPermission UI offers to the user. The user can promote one to
//     a session-wide allow rule, eliminating future prompts for matching
//     calls. See PermissionSuggestion for the rule string format.
type PermissionResult struct {
	Behavior      PermissionBehavior
	Message       string                 // human-readable; required for Ask/Deny
	ModifiedInput json.RawMessage        // optional; PermissionAllow only
	Suggestions   []PermissionSuggestion // optional UI proposals
	Dimensions    map[string]string      // optional; named input dimensions for rule matching
}

// PermissionSuggestion is a tool-proposed "always allow" or "always deny"
// rule that the PromptPermission UI offers to the user. When the user
// picks one, the kernel adds it to the active session profile so future
// matching calls are silently allowed (or denied).
//
// Label is the human-readable text shown in the prompt menu (e.g.
// "Always allow reads under /home/user").
//
// Rule is a permrules rule string in the format:
//
//   - → match any tool, any input
//     <tool>:**                  → match this tool, any input
//     <tool>.<field>:<glob>      → match this tool when dimensions[field] matches glob
//
// Tool authors should propose 2-4 rules at varying levels of specificity
// — e.g. one for the exact path, one for the parent directory, one for
// the broadest reasonable scope. The user picks the level they trust.
type PermissionSuggestion struct {
	Label string
	Rule  string
}

// DefaultDimensionExtractor walks the top-level JSON string fields of an
// input payload and returns them as a map. Used as a fallback by the
// kernel permission gate when a tool returns PermissionAsk without
// populating PermissionResult.Dimensions itself.
//
// Behavior:
//   - Top-level string fields are included as-is.
//   - Numeric, bool, object, and array fields are skipped.
//   - Malformed JSON returns an empty map (no error — fail-soft so the
//     gate can still proceed to PromptPermission with an empty rule
//     match attempt).
//
// This extractor is intentionally simple. Tools that need richer
// extraction (e.g. `bash` extracting just the command verb) should
// populate Dimensions themselves rather than rely on the default.
func DefaultDimensionExtractor(input json.RawMessage) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	var raw map[string]any
	if err := json.Unmarshal(input, &raw); err != nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

// PermissionFunc is the per-tool permission policy. Implementations must be
// non-blocking and side-effect-free; they decide whether the tool may run,
// not whether it has run successfully.
type PermissionFunc func(ctx context.Context, input json.RawMessage) PermissionResult

// InterruptBehavior controls what happens to a running tool when the agent
// loop's parent context is cancelled (e.g., the user submits a new message
// while a long-running tool is mid-call).
//
// See spec: docs/agent_tools/tool_system_spec.md FR-07
type InterruptBehavior int

const (
	// InterruptBlock (default) — the tool runs to completion. The cancellation
	// applies to the next tool call, not this one.
	InterruptBlock InterruptBehavior = iota

	// InterruptCancel — the tool's context is cancelled and its result is
	// discarded. Long-running tools (bash, web_fetch) opt into this when
	// abandoning a partial result is the right behavior.
	InterruptCancel
)

// String returns a stable lowercase identifier for logs.
func (i InterruptBehavior) String() string {
	switch i {
	case InterruptBlock:
		return "block"
	case InterruptCancel:
		return "cancel"
	default:
		return "unknown"
	}
}
