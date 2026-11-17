// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"io"

	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
)

// PluginStrategyDef is the strategy definition type for the agent.plugin domain.
type PluginStrategyDef = strategies.StrategyDef[PluginFactory]

// PluginFactory is a configured factory that creates plugin instances per-session.
// Receives LoopContext so plugins can access shared loop state from creation time.
// The refs slice contains all PluginRef entries that resolved to this factory's
// strategy type, enabling factories to filter or instantiate per-ref. Most
// factories ignore refs (single instance per type); the skill factory uses them
// to filter the loaded skill list (e.g., `skill:foo` loads only "foo").
type PluginFactory interface {
	Name() string
	CreatePlugins(ctx context.Context, loop LoopContext, refs []PluginRef) ([]AgentPlugin, error)
}

// FuncFactory implements PluginFactory using a function.
// Used by extension packages to declare simple plugin strategies.
type FuncFactory struct {
	FactoryName string
	CreateFunc  func(ctx context.Context, loop LoopContext, refs []PluginRef) ([]AgentPlugin, error)
}

func (f *FuncFactory) Name() string { return f.FactoryName }
func (f *FuncFactory) CreatePlugins(ctx context.Context, loop LoopContext, refs []PluginRef) ([]AgentPlugin, error) {
	return f.CreateFunc(ctx, loop, refs)
}

// --- Context keys for plugin-scoped values ---

type pluginContextKey struct{}
type variablesKey struct{}
type traceWriterKey struct{}
type agentModeKey struct{}
type userCommsInputKey struct{}
type currentTurnKey struct{}
type runIDKey struct{}

// WithPluginContext stores plugin context variables in context.
// PluginContextTarget is the plugin-context key naming the CWM object a run
// targets: the cwm plugin renders that object's `<cwm_object_context>` block.
const PluginContextTarget = "_target"

// ObjectContextPlugin is the plugin the agent factory attaches to every run
// with a target object, when the binary registers it: what a chat on that
// object receives (the object-context extension).
const ObjectContextPlugin = "object-context"

func WithPluginContext(ctx context.Context, m map[string]any) context.Context {
	return context.WithValue(ctx, pluginContextKey{}, m)
}

// PluginContextFrom retrieves plugin context variables, or nil.
func PluginContextFrom(ctx context.Context) map[string]any {
	if m, ok := ctx.Value(pluginContextKey{}).(map[string]any); ok {
		return m
	}
	return nil
}

// WithAgentMode stores the resolved delegation mode in context.
func WithAgentMode(ctx context.Context, mode string) context.Context {
	return context.WithValue(ctx, agentModeKey{}, mode)
}

// AgentModeFrom retrieves the delegation mode, or "" (defaults to worker).
func AgentModeFrom(ctx context.Context) string {
	if m, ok := ctx.Value(agentModeKey{}).(string); ok {
		return m
	}
	return ""
}

// WithUserCommsInput stores the resolved UserComms.Input channel value
// in context. The `ask-user` tool reads this at execute time via
// UserCommsInputFromContext to select its channel implementation
// (interactive / inbox / silent). Set by the agent factory once per
// run, immediately after computing `req.UserCommsInput`.
func WithUserCommsInput(ctx context.Context, input string) context.Context {
	if input == "" {
		return ctx
	}
	return context.WithValue(ctx, userCommsInputKey{}, input)
}

// UserCommsInputFromContext retrieves the resolved UserComms.Input
// channel value. Returns "" if no value was set (legacy programmatic
// paths that bypassed the resolver — callers should treat "" as
// "default to interactive for safety so chat-style flows that bypass
// the dispatch helper still work", matching the Phase A2 rule).
func UserCommsInputFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(userCommsInputKey{}).(string); ok {
		return v
	}
	return ""
}

// WithCurrentTurn stores the loop's current turn number in context.
// The loop sets it on the tool-dispatch context each turn so tools
// that emit their own conversation events (e.g. ask-user's
// prompt_shown / user_input) can tag them with the turn they belong
// to without the loop knowing about specific tools.
func WithCurrentTurn(ctx context.Context, turn int) context.Context {
	return context.WithValue(ctx, currentTurnKey{}, turn)
}

// CurrentTurnFromContext retrieves the loop's current turn, or 0 when
// the caller is not running inside an agent loop dispatch.
func CurrentTurnFromContext(ctx context.Context) int {
	if v, ok := ctx.Value(currentTurnKey{}).(int); ok {
		return v
	}
	return 0
}

// WithRunID stores the current run's id on the tool-dispatch context. The loop
// sets it each turn so tools that publish their OWN conversation events
// (ask-user's prompt_shown / user_input) can stamp them with the emitter — the
// per-session translator filters on this, so without it those events leak into
// every parallel session.
func WithRunID(ctx context.Context, runID string) context.Context {
	return context.WithValue(ctx, runIDKey{}, runID)
}

// RunIDFromContext retrieves the current run id, or "" outside a dispatch.
func RunIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(runIDKey{}).(string); ok {
		return v
	}
	return ""
}

// WithVariables stores resolved built-in variables in context.
func WithVariables(ctx context.Context, v map[string]string) context.Context {
	return context.WithValue(ctx, variablesKey{}, v)
}

// VariablesFrom retrieves resolved variables, or nil.
func VariablesFrom(ctx context.Context) map[string]string {
	if v, ok := ctx.Value(variablesKey{}).(map[string]string); ok {
		return v
	}
	return nil
}

// WithTraceWriter stores a trace writer in context for the debug plugin.
func WithTraceWriter(ctx context.Context, w io.Writer) context.Context {
	return context.WithValue(ctx, traceWriterKey{}, w)
}

// TraceWriterFrom retrieves the trace writer, or nil.
func TraceWriterFrom(ctx context.Context) io.Writer {
	if w, ok := ctx.Value(traceWriterKey{}).(io.Writer); ok {
		return w
	}
	return nil
}

// PluginsConfig holds plugin defaults and named declarations loaded from Plugins.yaml.
type PluginsConfig struct {
	Defaults []string    `yaml:"defaults"`
	Plugins  []PluginRef `yaml:"plugins"`
}
