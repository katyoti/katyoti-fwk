// SPDX-License-Identifier: Apache-2.0

// Package api defines the HumanGate service interface — the bidirectional
// user interaction protocol. UI implementations (terminal protocol, CLI) provide it.
// Agents and task runners consume it via application.UseService[HumanGate].
package api

import (
	"context"
	"errors"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// ErrUserDetached is returned by a pending Prompt when the gate's
// last UI client disconnects (after a grace period for reconnects).
// Consumers — the ask-user interactive channel, primarily — catch it
// to fall over to an async channel (the workspace inbox) instead of
// failing the ask: the question is preserved where the user will
// find it, and the task suspends instead of erroring.
var ErrUserDetached = errors.New("humangate: user detached (no connected UI client)")

// HumanGateDef describes the HumanGate service for KTSP publishing.
var HumanGateDef = meta.ServiceDef{
	Name:    "humangate",
	Short:   "User interaction gateway",
	Long:    `Provides bidirectional communication between agents and users. Two servers: room.HumanGate (terminal protocol for TUI/GUI/remote) and CLIPrompter (pipe mode fallback). Agents consume it via ask-user tool; the kernel permission gate consumes PromptPermission.`,
	Version: "2.0.0",
	Methods: map[string]meta.MethodDef{
		"Prompt":           meta.Method("Ask user for input (text, select, confirm, or form)"),
		"PromptPermission": meta.Method("Ask user to authorize a tool call with always-remember options"),
	},
}

// HumanGate is the published service interface for user interaction.
// Scope: PerRequester (each UI client gets its own instance).
//
// PROVIDED BY: room.HumanGate (terminal protocol), CLIPrompter (pipe mode)
// CONSUMED BY: ask-user tool (LLM agents), permission gate (kernel),
//
//	task workflow (machine strategy)
type HumanGate interface {
	// Prompt handles all user interaction: text, select, confirm, form.
	// Single method — the Type field in PromptRequest determines the prompt style.
	// Implementations that don't support a type return an error (check Capabilities first).
	Prompt(ctx context.Context, in PromptRequest) (PromptResponse, error)

	// PromptPermission is separate because it has a different consumer (kernel
	// permission gate) and a different response shape (4-way decision + rule).
	PromptPermission(ctx context.Context, in PromptPermissionRequest) (PromptPermissionResponse, error)

	// Capabilities returns the prompt types this gate supports.
	// Standard: ["text", "select", "confirm"]. Terminal protocol adds: "form".
	// Used by the ask-user tool to adapt the LLM-visible schema.
	Capabilities() []string
}

// ── Prompt request/response (unified) ────────────────────────────────────

// PromptRequest is the unified prompt input. The Type field determines which
// fields are used:
//   - "text": Question only (free-form text input)
//   - "select": Question + Options (pick one)
//   - "confirm": Question only (yes/no)
//   - "form": Question + FormSchema (schema-driven form)
type PromptRequest struct {
	Type        string         `json:"type"                   desc:"Prompt type: text, select, confirm, form" required:"true"`
	Question    string         `json:"question"               desc:"Question or description to display"       required:"true"`
	Options     []string       `json:"options,omitempty"       desc:"Available choices (select type)"`
	FormSchema  map[string]any `json:"form_schema,omitempty"  desc:"JSON Schema + UI Schema (form type)"`
	Placeholder string         `json:"placeholder,omitempty"  desc:"Input placeholder hint"`
}

// PromptResponse is the unified prompt output.
//   - "text": Answer contains the free-form text
//   - "select": Answer contains the selected option string
//   - "confirm": Confirmed is true/false
//   - "form": Data contains the submitted form data as JSON
type PromptResponse struct {
	Answer    string         `json:"answer,omitempty"    desc:"Text or selected option"`
	Confirmed bool           `json:"confirmed,omitempty" desc:"Confirm result (true/false)"`
	Data      map[string]any `json:"data,omitempty"      desc:"Form data (form type)"`
}

// ── Permission request/response (separate consumer) ──────────────────────

// PermissionDecision is the four-way outcome of a permission prompt.
//
// DenyOnce is the zero value: an unhelpful prompter (or one that errors
// out) defaults to "no, just this once" — the safe choice.
type PermissionDecision int

const (
	PermissionDecisionDenyOnce PermissionDecision = iota
	PermissionDecisionAllowOnce
	PermissionDecisionAllowAlways
	PermissionDecisionDenyAlways
)

// String returns a stable lowercase identifier for logs.
func (d PermissionDecision) String() string {
	switch d {
	case PermissionDecisionDenyOnce:
		return "deny_once"
	case PermissionDecisionAllowOnce:
		return "allow_once"
	case PermissionDecisionAllowAlways:
		return "allow_always"
	case PermissionDecisionDenyAlways:
		return "deny_always"
	default:
		return "unknown"
	}
}

// PermissionSuggestion is a tool-proposed "always allow" / "always deny"
// rule that the prompt UI offers to the user.
type PermissionSuggestion struct {
	Label string `json:"label" desc:"Display label for the prompt UI"`
	Rule  string `json:"rule"  desc:"Permrules rule string this suggestion would add"`
}

// PromptPermissionRequest asks the user to authorize a tool call.
type PromptPermissionRequest struct {
	ToolName    string                 `json:"tool_name"              desc:"Name of the tool requesting permission" required:"true"`
	Question    string                 `json:"question"               desc:"Why permission is needed"               required:"true"`
	Suggestions []PermissionSuggestion `json:"suggestions,omitempty"  desc:"Tool-proposed always-allow rules"`
}

// PromptPermissionResponse is the user's permission decision.
type PromptPermissionResponse struct {
	Decision     PermissionDecision `json:"decision"                desc:"User's decision"`
	SelectedRule string             `json:"selected_rule,omitempty" desc:"Rule string when decision is AllowAlways or DenyAlways"`
}

// ── Per-run gate in context (parallel-chat isolation) ───────────────────
//
// Each ServeTask run creates its OWN HumanGate. Registering it as the
// app-global "default" service collides when two runs are live at once
// (the second overwrites the first, so ask-user answers misroute). To keep
// each run's gate isolated, ServeTask stamps the run's gate onto its ctx and
// the ask-user interactive channel reads it from there FIRST — falling back
// to the service registry only when ctx carries none.
type humanGateCtxKey struct{}

// WithHumanGate returns a ctx carrying this run's gate.
func WithHumanGate(ctx context.Context, gate HumanGate) context.Context {
	if gate == nil {
		return ctx
	}
	return context.WithValue(ctx, humanGateCtxKey{}, gate)
}

// HumanGateFromContext returns the run's gate stamped by WithHumanGate, or
// nil when none is set (caller then resolves via the service registry).
func HumanGateFromContext(ctx context.Context) HumanGate {
	if g, ok := ctx.Value(humanGateCtxKey{}).(HumanGate); ok {
		return g
	}
	return nil
}
