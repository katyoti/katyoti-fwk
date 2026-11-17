// SPDX-License-Identifier: Apache-2.0

// Package keepalive contributes the `chat-loop-keepalive` agent
// plugin — a reusable rule that turns the agent loop's natural
// "no tool call → run ends" behavior into "no tool call → loop once
// more with a nudge".
//
// Why this exists. For one-shot tasks ("audit this codebase", "draft
// a plan") the loop's default behavior is correct: when the LLM
// produces a final text response with no tool calls, the run is done.
// For chat-style tasks the same default is wrong: an LLM that
// answers the user's question and forgets to call `ask-user` leaves
// the loop suspended forever — the GUI shows the response, the input
// stays disabled, the user has no way to continue.
//
// The agent loop already exposes the right seam: when zero tool
// calls come back from the LLM, it fires the `OnDecision` hook
// chain (see `agent_machine.go:598-611`). Any plugin that sets
// `decision.Continue = true` and appends `decision.Messages` makes
// the loop run another turn with those messages prepended to the
// next API call. That is exactly what this plugin does.
//
// On the second turn the LLM receives a brief, user-role nudge —
// "your last turn ended with text only; in chat sessions every turn
// must end with `ask-user`" — and almost always complies. If it
// stubbornly produces another text-only turn, the plugin nudges
// again. The agent loop's `MaxTurns` ceiling (from the LLM profile)
// is the safety net against runaway loops.
//
// `mode: chat` tasks get this plugin auto-attached by the agent
// factory (alongside the `complete`-tool filter); explicit opt-in via
// `plugins: [chat-loop-keepalive]` in TASK.md also works for tasks
// that want the nudge without the rest of chat mode. One-shot tasks
// keep their existing termination semantics.
package keepalive

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// strategyName is the public registration key; consumers reference
// it via `plugins: [chat-loop-keepalive]` in TASK.md.
const strategyName = "chat-loop-keepalive"

// nudgeMessage is the user-role reminder injected when the LLM
// produced a text-only turn. Phrased to be short + actionable so it
// doesn't bloat the next API call beyond what's needed.
//
// Rendered as `role: user` (not system) because some providers treat
// mid-conversation system messages as advisory and may ignore them;
// a user-role reminder is universally honored as fresh input.
const nudgeMessage = "Your last turn ended with text only — no tool was called. " +
	"In a chat session every turn MUST end with the `ask-user` tool so the " +
	"user can send their next message. " +
	"If you have a specific follow-up question, call `ask-user` with it. " +
	"If you have nothing specific to ask (the typical case after completing " +
	"a user request), call `ask-user` with an EMPTY question — that signals " +
	"\"yield to user; wait for next message\" and the chat input becomes " +
	"available without a prompt banner. Do NOT invent generic phrasings like " +
	"\"Anything else?\" / \"What's next?\" — they're noise; an empty " +
	"ask-user is the correct idle signal. Do not emit another text-only " +
	"turn; do not call `complete` unless the user explicitly asked to end " +
	"the session."

// keepalivePlugin is the AgentPlugin implementation. Stateless — the
// nudgeMessage is constant and the OnDecision hook decision is the
// same regardless of context.
//
// Note that we deliberately do NOT inspect the agent's prior
// messages here. The hook fires only on the "zero tool calls" path
// (see agent_machine.go:598); the loop already proved the LLM
// produced no tool call this turn, which IS the trigger condition.
// Adding more checks here would just be defensive code without a
// behavior change.
type keepalivePlugin struct{}

func (p *keepalivePlugin) Name() string                 { return strategyName }
func (p *keepalivePlugin) SystemMessage() string        { return "" }
func (p *keepalivePlugin) ToolDefs() []agentapi.ToolDef { return nil }
func (p *keepalivePlugin) Stable() bool                 { return true }

func (p *keepalivePlugin) HandleToolCall(_ context.Context, name string, _ json.RawMessage) (*agentapi.ToolCallResult, error) {
	// This plugin owns no tools — should never be reached. Defensive
	// error rather than nil so a wiring bug surfaces immediately.
	return nil, fmt.Errorf("keepalive plugin does not handle tool calls (got %q)", name)
}

func (p *keepalivePlugin) Hooks() agentapi.PluginHooks {
	return agentapi.PluginHooks{
		OnDecision: func(_ agentapi.LoopContext, decision *agentapi.Decision) error {
			decision.Continue = true
			decision.Messages = append(decision.Messages,
				agentapi.NewTextMessage(agentapi.RoleUser, nudgeMessage),
			)
			return nil
		},
	}
}

// strategyDef registers the plugin under "chat-loop-keepalive".
// The factory returns a single shared instance — the plugin holds
// no per-session state.
var strategyDef = strategies.FuncStrategy[agentapi.PluginFactory](
	strategyName, "Chat-loop keep-alive (force ask-user on text-only turns)",
	func() (agentapi.PluginFactory, error) {
		return &agentapi.FuncFactory{
			FactoryName: strategyName,
			CreateFunc: func(_ context.Context, _ agentapi.LoopContext, _ []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
				return []agentapi.AgentPlugin{&keepalivePlugin{}}, nil
			},
		}, nil
	},
)

// AgentPluginExtension is the ktw-agents-side wiring helper. Add it
// to `buildExtensions(...)` in `app/ktw-agents/cmd/bootstrap.go`.
// Mirrors `cwmlocal.AgentPluginExtension` so the registration shape
// is uniform across plugin packages.
//
// Kept for TASK.md files that declare `plugins: [chat-loop-keepalive]`
// explicitly. Tasks SHOULD use `mode: chat` instead — the factory
// auto-attaches the same hook + filters the `complete` tool from
// the catalog. See `AgentRunRequest.Mode` godoc.
func AgentPluginExtension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute(strategyName, ".plugin", func(app *application.Application) error {
			return agentapi.ContributePluginStrategy(app, strategyDef)
		})
	}
}

// NewPlugin returns a freshly constructed keepalive plugin.
//
// Exposed so the agent factory can auto-attach the OnDecision hook
// for `mode: chat` runs WITHOUT routing through the strategy resolver
// (the resolver is the right path for static `plugins: [...]`
// declarations; the auto-attach is a derived behavior of chat mode
// and shouldn't require the user to also add the plugin name to
// their plugin list).
//
// The returned plugin is stateless — sharing one instance across
// goroutines is safe.
func NewPlugin() agentapi.AgentPlugin {
	return &keepalivePlugin{}
}
