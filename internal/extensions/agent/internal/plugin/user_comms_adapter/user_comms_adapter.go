// SPDX-License-Identifier: Apache-2.0

// Package user_comms_adapter contributes the user-comms-adapter agent
// plugin.
//
// What this plugin does: read the resolved `UserCommsInput` value off
// the run request, inject a channel-specific system-prompt block so
// the agent knows HOW the user channel behaves, and get auto-attached
// by the agent factory whenever the resolver finalized a non-empty
// value.
//
// What this plugin does NOT do (yet): swap the `ask_user` /
// `notify_user` tool implementations per channel — that lives in the
// tool-side dispatcher (see internal/extensions/tools/ask_user/).
// This plugin makes the agent AWARE of its channel via the
// system-prompt block; the tool dispatcher picks the actual
// implementation.
//
// Why a separate plugin (not an inline factory branch):
// channel-specific behavior belongs in plugins, not in the loop's
// factory. This plugin is the canonical seam for every future
// per-channel adjustment — the `RequiresHumanGate` Describe metadata
// surface, any future tool registrations gated by channel — all
// hang off this one place.
package user_comms_adapter

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// channelPrompt returns the system-prompt block to inject for a
// resolved UserCommsInput value. Empty value = no block (the
// resolver didn't run; legacy programmatic path; safer to say
// nothing than to misrepresent the channel).
//
// The blocks are short by design — the LLM needs the contract,
// not a manual. Each ends by telling the agent which tool surface
// to use, so Phase C's tool swap maps 1:1 onto these phrasings
// when the time comes.
func channelPrompt(input string) string {
	switch input {
	case "direct":
		return "<user_channel>\n" +
			"You are talking to a user IN REAL TIME via an interactive chat. " +
			"When you call `ask-user`, the user typically responds within seconds. " +
			"Use `ask-user` liberally — clarifying questions, choices, confirmations. " +
			"Keep your turns short; treat the conversation as a back-and-forth.\n" +
			"</user_channel>"
	case "inbox":
		return "<user_channel>\n" +
			"You are running HEADLESS — there is no live chat session attached. " +
			"`ask-user` calls land in the target node's Inbox as an `approval-request` " +
			"item. The user MAY take hours to respond, or may never read it. " +
			"Therefore: do NOT ask questions unless you genuinely cannot proceed without " +
			"the answer. When you must ask, frame the question so a reasonable default " +
			"answer is obvious from context. Long inbox queues are user-hostile; one " +
			"well-framed question beats five clarifications.\n" +
			"</user_channel>"
	case "bubble_to_parent":
		return "<user_channel>\n" +
			"You are a CHILD task. Your user-input requests are routed THROUGH your " +
			"parent agent — the parent decides whether to forward them to the user, " +
			"answer on the user's behalf, or reject them. Treat the parent as your " +
			"effective audience: ask only what the parent cannot answer from context.\n" +
			"</user_channel>"
	case "via_leader":
		return "<user_channel>\n" +
			"You are a TEAMMATE on a team. User-input requests route through your team " +
			"leader's channel — the leader decides what reaches the user. Talk to the " +
			"leader, not to the user directly. Keep your traffic concise; the leader is " +
			"juggling multiple teammates.\n" +
			"</user_channel>"
	case "none":
		return "<user_channel>\n" +
			"You have NO user channel. `ask-user` will fail. You must complete the task " +
			"using ONLY the information already in your context + the available tools. " +
			"If you genuinely cannot proceed without user input, return a clear " +
			"explanation of what you need and stop — do not call `ask-user`.\n" +
			"</user_channel>"
	default:
		// Empty or unrecognized — say nothing. The factory shouldn't
		// attach the plugin in this case (no input → no plugin), but
		// if it slips through, silence is the safe default.
		return ""
	}
}

// plugin is the AgentPlugin implementation. Stateless — the
// channelPrompt is computed once at construction from the
// resolved input value.
type plugin struct {
	input  string
	prompt string
}

// NewPlugin constructs the plugin for a resolved UserCommsInput
// value. Returns nil if the value is empty or unrecognized — the
// caller should NOT attach a plugin in that case.
func NewPlugin(input string) api.AgentPlugin {
	p := channelPrompt(input)
	if p == "" {
		return nil
	}
	return &plugin{input: input, prompt: p}
}

func (p *plugin) Name() string            { return "user-comms-adapter" }
func (p *plugin) SystemMessage() string   { return p.prompt }
func (p *plugin) ToolDefs() []api.ToolDef { return nil }
func (p *plugin) Stable() bool            { return true }
func (p *plugin) Hooks() api.PluginHooks  { return api.PluginHooks{} }

func (p *plugin) HandleToolCall(_ context.Context, name string, _ json.RawMessage) (*api.ToolCallResult, error) {
	return nil, fmt.Errorf("user-comms-adapter does not handle tool calls (got %q)", name)
}
