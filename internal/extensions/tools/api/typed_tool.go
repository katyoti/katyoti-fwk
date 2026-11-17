// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"fmt"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// MaxResultSizeDefault is the per-tool result-data size limit applied when
// no explicit size is configured. Tools producing larger output have their
// data persisted to disk by the kernel and a preview returned to the model.
const MaxResultSizeDefault = 256 * 1024

// FunctionTool is a single-handler tool produced by NewFunctionTool. It
// implements the full Tool interface with safe defaults for every method
// not directly relevant to a one-shot function call.
type FunctionTool[TInput, TOutput any] struct {
	name    string
	desc    string
	handler func(ctx context.Context, input TInput) (TOutput, error)
	opts    ToolOptions
	schema  json.RawMessage // generated once at construction
}

// ToolOptions configures a tool's metadata, execution properties, and
// permission policy. All fields are optional; NewFunctionTool applies the
// fail-closed defaults from spec FR-01 before user options are layered on.
type ToolOptions struct {
	// --- Identity ---
	Aliases   []string
	Enabled   func() bool                        // nil → always true
	Describer func(input json.RawMessage) string // nil → built-in default

	// --- Execution properties ---
	ReadOnly        bool
	ConcurrencySafe bool
	Destructive     bool
	Interrupt       InterruptBehavior
	MaxResultSize   int // 0 = unlimited; positive = byte limit. Default applied in NewFunctionTool.
	Deferred        bool
	SearchHint      string
	// Interactive marks a tool that blocks on user input and emits its
	// own conversation events (prompt_shown / user_input). Propagated to
	// agentapi.ToolDef.Interactive; the agent loop keys behavior off the
	// attribute, never off the tool's name.
	Interactive bool
	// Polling marks a tool meant to be called repeatedly with identical
	// arguments (status checks). Raises the loop-safety repetition
	// threshold. Propagated to agentapi.ToolDef.Polling.
	Polling bool
	// Idle marks a tool whose identical repetition is the canonical idle
	// shape of an interactive session. Exempt from the loop-safety guard
	// in keepalive mode. Propagated to agentapi.ToolDef.Idle.
	Idle bool

	// --- Permission ---
	// Permission, if non-nil, overrides the default policy derived from the
	// ReadOnly / Destructive flags. The default chain (see permission.go) is:
	//   ReadOnly → Allow
	//   Destructive → Ask (escalated)
	//   otherwise → Ask
	Permission PermissionFunc

	// --- Display ---
	// Display provides rendering metadata (icon, label, color, template)
	// for tool calls in the TUI/CLI output. nil → generic fallback.
	Display *ToolDisplay

	// --- Prompt ---
	Prompt string

	// --- Tool group only ---
	// GroupPrefix overrides the default per-method tool name prefix used by
	// NewToolGroup. nil = use the default ("<group-name>-"); non-nil pointer
	// to "" = no prefix; non-nil pointer to "x-" = "x-" prefix.
	// Set via WithGroupPrefix.
	GroupPrefix *string
}

// ToolOption is a functional option for NewFunctionTool.
type ToolOption func(*ToolOptions)

// WithReadOnly marks the tool as read-only and concurrency-safe.
// The default permission policy auto-allows read-only tools.
func WithReadOnly() ToolOption {
	return func(o *ToolOptions) {
		o.ReadOnly = true
		o.ConcurrencySafe = true
	}
}

// WithMutating marks the tool as mutating and not concurrency-safe.
// The default permission policy asks for user approval before each call.
// This is the fail-closed default — calling WithMutating() is only needed
// to make the intent explicit.
func WithMutating() ToolOption {
	return func(o *ToolOptions) {
		o.ReadOnly = false
		o.ConcurrencySafe = false
	}
}

// WithDestructive marks the tool as performing an irreversible action.
// Implies mutating + non-concurrent-safe and escalates the default
// permission prompt wording.
func WithDestructive() ToolOption {
	return func(o *ToolOptions) {
		o.ReadOnly = false
		o.ConcurrencySafe = false
		o.Destructive = true
	}
}

// WithInteractive marks the tool as user-interactive: it blocks on user
// input and emits its own prompt_shown / user_input conversation events.
func WithInteractive() ToolOption {
	return func(o *ToolOptions) { o.Interactive = true }
}

// WithPolling marks the tool as designed for repeated identical calls
// (status checks, mailbox polls) — the loop-safety guard relaxes its
// repetition threshold accordingly.
func WithPolling() ToolOption {
	return func(o *ToolOptions) { o.Polling = true }
}

// WithIdle marks identical repetition of this tool as the canonical
// idle shape of an interactive session — exempt from the loop-safety
// guard while the run is in keepalive mode.
func WithIdle() ToolOption {
	return func(o *ToolOptions) { o.Idle = true }
}

// WithAliases adds backward-compatible alternative names for the tool.
func WithAliases(names ...string) ToolOption {
	return func(o *ToolOptions) {
		o.Aliases = append(o.Aliases, names...)
	}
}

// WithEnabledFunc supplies a runtime predicate that decides whether the
// tool is currently active. Used for feature-flagged tools.
func WithEnabledFunc(fn func() bool) ToolOption {
	return func(o *ToolOptions) { o.Enabled = fn }
}

// WithDescriber installs a custom per-call describer that produces a
// human-readable summary of the tool invocation from the raw JSON input.
// Used in permission dialogs and progress UI.
func WithDescriber(fn func(input json.RawMessage) string) ToolOption {
	return func(o *ToolOptions) { o.Describer = fn }
}

// WithMaxResultSize sets the explicit byte limit on result data before
// kernel persistence kicks in.
func WithMaxResultSize(n int) ToolOption {
	return func(o *ToolOptions) { o.MaxResultSize = n }
}

// WithUnlimitedResult disables the result size limit. The kernel will
// never persist this tool's output to disk regardless of size.
func WithUnlimitedResult() ToolOption {
	return func(o *ToolOptions) { o.MaxResultSize = 0 }
}

// WithCancelOnInterrupt sets InterruptBehavior to Cancel — long-running
// tools whose partial results should be discarded when the user submits
// a new message mid-call.
func WithCancelOnInterrupt() ToolOption {
	return func(o *ToolOptions) { o.Interrupt = InterruptCancel }
}

// WithDeferred marks the tool's schema as deferred-loaded — discoverable
// only via ToolSearch. The hint is the keyword phrase used to find it.
func WithDeferred(hint string) ToolOption {
	return func(o *ToolOptions) {
		o.Deferred = true
		o.SearchHint = hint
	}
}

// WithPrompt sets the tool's contribution to the agent system message.
func WithPrompt(prompt string) ToolOption {
	return func(o *ToolOptions) { o.Prompt = prompt }
}

// WithPermissionCheck installs a custom permission policy that overrides
// the default chain.
func WithPermissionCheck(fn PermissionFunc) ToolOption {
	return func(o *ToolOptions) { o.Permission = fn }
}

// WithAllowPermission installs a permission policy that always allows the
// tool to run without prompting. Use with care — this bypasses the default
// fail-closed policy.
func WithAllowPermission() ToolOption {
	return func(o *ToolOptions) { o.Permission = AllowFunc() }
}

// WithAskPermission installs a permission policy that always asks the user.
func WithAskPermission() ToolOption {
	return func(o *ToolOptions) { o.Permission = AskFunc("") }
}

// WithGroupPrefix overrides the default per-method tool name prefix used by
// NewToolGroup. The default is "<group-name>-" so a method GotoDefinition
// in a group named "lsp" becomes "lsp-goto-definition". Pass "" for no
// prefix or "x-" for a custom one ("code-goto-definition"). Keep it kebab:
// tool names are kebab-case (rule).
//
// Has no effect on NewFunctionTool — it is only consulted by NewToolGroup.
func WithGroupPrefix(prefix string) ToolOption {
	return func(o *ToolOptions) { o.GroupPrefix = &prefix }
}

// NewFunctionTool builds a single-handler typed tool.
//
// Defaults (fail-closed per spec FR-01): mutating, not concurrency-safe,
// not destructive, blocks on interrupt, 32 KB result limit, asks for
// permission before every call.
//
// Example:
//
//	var grepTool = api.NewFunctionTool[Input, Output](
//	    "grep", "Search file contents with regex.",
//	    handler,
//	    api.WithReadOnly(),
//	)
func NewFunctionTool[TInput, TOutput any](
	name, desc string,
	handler func(context.Context, TInput) (TOutput, error),
	opts ...ToolOption,
) *FunctionTool[TInput, TOutput] {
	o := ToolOptions{
		// Fail-closed defaults per spec FR-01.
		ReadOnly:        false,
		ConcurrencySafe: false,
		Destructive:     false,
		Interrupt:       InterruptBlock,
		MaxResultSize:   MaxResultSizeDefault,
	}
	for _, opt := range opts {
		opt(&o)
	}
	return &FunctionTool[TInput, TOutput]{
		name:    name,
		desc:    desc,
		handler: handler,
		opts:    o,
		schema:  MustGenerateSchema[TInput](),
	}
}

// --- Tool interface implementation ---

func (t *FunctionTool[I, O]) ToolDef() agentapi.ToolDef {
	return agentapi.ToolDef{
		Name:        t.name,
		Description: t.desc,
		InputSchema: t.schema,
		Display:     t.opts.Display,
		Interactive: t.opts.Interactive,
		Polling:     t.opts.Polling,
		Idle:        t.opts.Idle,
	}
}

func (t *FunctionTool[I, O]) ToolCall(ctx context.Context, input json.RawMessage) (*agentapi.ToolCallResult, error) {
	var in I
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	out, err := t.handler(ctx, in)
	if err != nil {
		return nil, err
	}
	return &agentapi.ToolCallResult{Data: out}, nil
}

func (t *FunctionTool[I, O]) Aliases() []string { return t.opts.Aliases }

func (t *FunctionTool[I, O]) IsEnabled() bool {
	if t.opts.Enabled == nil {
		return true
	}
	return t.opts.Enabled()
}

func (t *FunctionTool[I, O]) Describe(input json.RawMessage) string {
	if t.opts.Describer != nil {
		return t.opts.Describer(input)
	}
	return defaultDescribe(t.name, input)
}

func (t *FunctionTool[I, O]) SearchHint() string                   { return t.opts.SearchHint }
func (t *FunctionTool[I, O]) IsReadOnly() bool                     { return t.opts.ReadOnly }
func (t *FunctionTool[I, O]) IsConcurrencySafe() bool              { return t.opts.ConcurrencySafe }
func (t *FunctionTool[I, O]) IsDestructive() bool                  { return t.opts.Destructive }
func (t *FunctionTool[I, O]) InterruptBehavior() InterruptBehavior { return t.opts.Interrupt }
func (t *FunctionTool[I, O]) MaxResultSize() int                   { return t.opts.MaxResultSize }
func (t *FunctionTool[I, O]) ShouldDefer() bool                    { return t.opts.Deferred }
func (t *FunctionTool[I, O]) Prompt() string                       { return t.opts.Prompt }

func (t *FunctionTool[I, O]) CheckPermissions(ctx context.Context, input json.RawMessage) PermissionResult {
	policy := t.opts.Permission
	if policy == nil {
		policy = PolicyFromOptions(t.opts)
	}
	return policy(ctx, input)
}

// defaultDescribe builds the fallback per-call description: "<name> <truncated input>".
func defaultDescribe(name string, input json.RawMessage) string {
	const maxInputChars = 256
	if len(input) == 0 {
		return name
	}
	in := string(input)
	if len(in) > maxInputChars {
		in = in[:maxInputChars] + "…"
	}
	return name + " " + in
}
