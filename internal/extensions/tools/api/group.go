// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"fmt"
	stdreflect "reflect"
	"strings"
	"unicode"

	corereflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// ConnectableToolGroup is an optional interface a tool group implementation
// can satisfy when it owns a long-lived external resource (LSP server, REPL,
// MCP server, browser session, etc.).
//
// Lifecycle (managed by the plugin strategy that wraps the group):
//
//  1. Open(ctx) is called lazily by the plugin factory before the group's
//     first tool is invoked. A non-nil error fails plugin construction.
//  2. Close(ctx) is called when the scoped-service container disposes the
//     group instance — see internal/core/services scoping.
//
// Implementations that already satisfy io.Closer get Close() routed via
// the existing scoped-service teardown for free.
type ConnectableToolGroup interface {
	Open(ctx context.Context) error
	Close(ctx context.Context) error
}

// NewToolGroup builds one Tool per exported method of TGroup, all sharing
// the supplied options. Each method becomes an agent-callable tool named
// "<prefix><method-kebab>" — e.g. a GotoDefinition method in a group named
// "lsp" becomes the tool "lsp-goto-definition". Tool names are kebab-case
// (rule); the wire name an LLM provider needs is not this
// package's concern.
//
// TGroup MUST be a Go interface type whose every exported method has the
// signature
//
//	func(ctx context.Context, in TIn) (TOut, error)
//
// where TIn is a struct (or pointer to a struct) usable for JSON-schema
// generation. Any deviation panics at NewToolGroup time — registration is
// the right place to fail loudly.
//
// Example:
//
//	type LSPTools interface {
//	    GotoDefinition(ctx context.Context, in GotoDefinitionInput) (GotoDefinitionOutput, error)
//	    FindReferences(ctx context.Context, in FindReferencesInput) (FindReferencesOutput, error)
//	    // ...
//	}
//
//	tools := api.NewToolGroup[LSPTools](
//	    "lsp", "Language Server Protocol operations.", lspImpl,
//	    api.WithReadOnly(),
//	)
//	// → []Tool{ lsp-goto-definition, lsp-find-references, ... }
func NewToolGroup[TGroup any](
	name, desc string,
	impl TGroup,
	opts ...ToolOption,
) []Tool {
	groupType := stdreflect.TypeFor[TGroup]()
	if groupType == nil {
		panic("NewToolGroup: unable to resolve TGroup type")
	}
	if groupType.Kind() != stdreflect.Interface {
		panic(fmt.Sprintf("NewToolGroup[%s]: TGroup must be an interface type, got %s", groupType.Name(), groupType.Kind()))
	}

	// Resolve options once; per-method tools share them by value.
	o := ToolOptions{
		ReadOnly:        false,
		ConcurrencySafe: false,
		Destructive:     false,
		Interrupt:       InterruptBlock,
		MaxResultSize:   MaxResultSizeDefault,
	}
	for _, opt := range opts {
		opt(&o)
	}

	prefix := name + "-"
	if o.GroupPrefix != nil {
		prefix = *o.GroupPrefix
	}

	// Reflect over the impl so we can call concrete methods (cheaper than
	// going through the interface dispatch table on every call).
	implVal := stdreflect.ValueOf(impl)
	if !implVal.IsValid() {
		panic(fmt.Sprintf("NewToolGroup[%s]: impl is the zero value", groupType.Name()))
	}

	tools := make([]Tool, 0, groupType.NumMethod())
	for i := 0; i < groupType.NumMethod(); i++ {
		ifaceMethod := groupType.Method(i)
		toolName := prefix + camelToKebab(ifaceMethod.Name)
		tools = append(tools, buildGroupMethodTool(name, desc, toolName, ifaceMethod, implVal, o))
	}
	return tools
}

// buildGroupMethodTool validates one interface method and produces a Tool
// adapter for it.
func buildGroupMethodTool(
	groupName, groupDesc, toolName string,
	method stdreflect.Method,
	implVal stdreflect.Value,
	opts ToolOptions,
) Tool {
	// Validate signature: func(ctx context.Context, in TIn) (TOut, error)
	mtype := method.Type
	if mtype.NumIn() != 2 {
		panic(fmt.Sprintf("NewToolGroup[%s].%s: method must take exactly 2 args (ctx, input), got %d",
			groupName, method.Name, mtype.NumIn()))
	}
	if mtype.NumOut() != 2 {
		panic(fmt.Sprintf("NewToolGroup[%s].%s: method must return exactly 2 values (output, error), got %d",
			groupName, method.Name, mtype.NumOut()))
	}
	ctxType := mtype.In(0)
	if !ctxType.Implements(stdreflect.TypeFor[context.Context]()) {
		panic(fmt.Sprintf("NewToolGroup[%s].%s: first arg must be context.Context, got %s",
			groupName, method.Name, ctxType))
	}
	if !mtype.Out(1).Implements(stdreflect.TypeFor[error]()) {
		panic(fmt.Sprintf("NewToolGroup[%s].%s: second return must be error, got %s",
			groupName, method.Name, mtype.Out(1)))
	}

	inputType := mtype.In(1)
	schema, err := corereflect.JSONSchemaForType(inputType)
	if err != nil {
		panic(fmt.Sprintf("NewToolGroup[%s].%s: schema generation failed: %v",
			groupName, method.Name, err))
	}

	// MethodByName resolves at construction time so the per-call hot path
	// only does input unmarshal + Call().
	methodVal := implVal.MethodByName(method.Name)
	if !methodVal.IsValid() {
		panic(fmt.Sprintf("NewToolGroup[%s].%s: impl does not implement this method (interface contract violation)",
			groupName, method.Name))
	}

	return &groupMethodTool{
		name:       toolName,
		desc:       groupDesc + " — " + humanize(method.Name),
		schema:     schema,
		opts:       opts,
		method:     methodVal,
		inputType:  inputType,
		methodName: method.Name,
	}
}

// groupMethodTool adapts one interface method into the Tool interface.
type groupMethodTool struct {
	name       string
	desc       string
	schema     json.RawMessage
	opts       ToolOptions
	method     stdreflect.Value // bound method on the impl
	inputType  stdreflect.Type
	methodName string
}

func (t *groupMethodTool) ToolDef() agentapi.ToolDef {
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

func (t *groupMethodTool) ToolCall(ctx context.Context, input json.RawMessage) (*agentapi.ToolCallResult, error) {
	// Allocate a new instance of the input type and unmarshal into it.
	inPtr := stdreflect.New(t.inputType)
	if len(input) > 0 {
		if err := json.Unmarshal(input, inPtr.Interface()); err != nil {
			return nil, fmt.Errorf("parse input for %s: %w", t.name, err)
		}
	}

	results := t.method.Call([]stdreflect.Value{
		stdreflect.ValueOf(ctx),
		inPtr.Elem(),
	})
	if errVal := results[1]; !errVal.IsNil() {
		return nil, errVal.Interface().(error)
	}
	return &agentapi.ToolCallResult{Data: results[0].Interface()}, nil
}

func (t *groupMethodTool) Aliases() []string { return t.opts.Aliases }

func (t *groupMethodTool) IsEnabled() bool {
	if t.opts.Enabled == nil {
		return true
	}
	return t.opts.Enabled()
}

func (t *groupMethodTool) Describe(input json.RawMessage) string {
	if t.opts.Describer != nil {
		return t.opts.Describer(input)
	}
	return defaultDescribe(t.name, input)
}

func (t *groupMethodTool) SearchHint() string                   { return t.opts.SearchHint }
func (t *groupMethodTool) IsReadOnly() bool                     { return t.opts.ReadOnly }
func (t *groupMethodTool) IsConcurrencySafe() bool              { return t.opts.ConcurrencySafe }
func (t *groupMethodTool) IsDestructive() bool                  { return t.opts.Destructive }
func (t *groupMethodTool) InterruptBehavior() InterruptBehavior { return t.opts.Interrupt }
func (t *groupMethodTool) MaxResultSize() int                   { return t.opts.MaxResultSize }
func (t *groupMethodTool) ShouldDefer() bool                    { return t.opts.Deferred }
func (t *groupMethodTool) Prompt() string                       { return t.opts.Prompt }

func (t *groupMethodTool) CheckPermissions(ctx context.Context, input json.RawMessage) PermissionResult {
	policy := t.opts.Permission
	if policy == nil {
		policy = PolicyFromOptions(t.opts)
	}
	return policy(ctx, input)
}

// camelToKebab converts CamelCase to kebab-case, correctly handling runs of
// uppercase letters:
//
//	"GotoDefinition" → "goto-definition"
//	"HTTPGet"        → "http-get"
//	"ParseURL"       → "parse-url"
//	"ID"             → "id"
//
// Rule: insert a hyphen before an uppercase letter that is either
// (a) preceded by a lowercase letter, or
// (b) followed by a lowercase letter and not at position 0
// — then lowercase the whole thing.
func camelToKebab(s string) string {
	rs := []rune(s)
	var b strings.Builder
	b.Grow(len(rs) + 4)
	for i, r := range rs {
		if i > 0 && unicode.IsUpper(r) {
			prevLower := unicode.IsLower(rs[i-1])
			nextLower := i+1 < len(rs) && unicode.IsLower(rs[i+1])
			if prevLower || nextLower {
				b.WriteByte('-')
			}
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// humanize converts CamelCase to space-separated words: "GotoDefinition" → "Goto Definition".
func humanize(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && unicode.IsUpper(r) {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}
