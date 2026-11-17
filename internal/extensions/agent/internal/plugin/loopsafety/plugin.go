// SPDX-License-Identifier: Apache-2.0

// Package loopsafety guards the agent loop against runaway repetition:
// the same tool called with identical arguments over and over almost
// certainly means the loop is stuck.
//
// The guard is attribute-driven — tools declare their repetition
// semantics on their ToolDef instead of the loop knowing tool names:
//
//   - default: 3 identical calls → denied
//   - ToolDef.Polling (task-status, check-messages): threshold 10 —
//     repeated identical polls are the tool's job
//   - ToolDef.Idle (ask-user) + keepalive mode: exempt — repeating
//     "wait for the user" IS the canonical chat shape, not a runaway
//
// On trip the tool call is DENIED via OnPreToolExec with a corrective
// reason the model can act on; MaxTurns remains the hard ceiling if it
// keeps trying.
package loopsafety

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

const (
	defaultThreshold = 3
	pollingThreshold = 10
)

// ToolAttrs reports a tool's repetition semantics (polling, idle, blocking)
// from its ToolDef. Provided by the factory as a closure over the assembled
// plugin set so the lookup sees the live tool list each call.
type ToolAttrs func(toolName string) (polling, idle, blocking bool)

// Plugin is the loop-safety guard. One instance per run — counters are
// per-run by construction.
type Plugin struct {
	mu         sync.Mutex
	counts     map[string]int
	exemptIdle bool // keepalive / chat mode: idle tools bypass the guard
	attrs      ToolAttrs
}

// NewPlugin creates the guard. exemptIdle is true for interactive
// keepalive runs (chat) where idle-declared tools repeat by design.
// attrs may be nil (no tool gets polling/idle treatment).
func NewPlugin(exemptIdle bool, attrs ToolAttrs) *Plugin {
	return &Plugin{
		counts:     make(map[string]int),
		exemptIdle: exemptIdle,
		attrs:      attrs,
	}
}

func (p *Plugin) Name() string          { return "loop-safety" }
func (p *Plugin) SystemMessage() string { return "" }
func (p *Plugin) ToolDefs() []api.ToolDef {
	return nil
}
func (p *Plugin) HandleToolCall(context.Context, string, json.RawMessage) (*api.ToolCallResult, error) {
	return nil, fmt.Errorf("loop-safety: no tools")
}
func (p *Plugin) Stable() bool { return true }

func (p *Plugin) Hooks() api.PluginHooks {
	return api.PluginHooks{
		OnPreToolExec: p.onPreToolExec,
	}
}

func (p *Plugin) onPreToolExec(_ api.LoopContext, toolName string, input map[string]any) (*api.ToolDecision, error) {
	polling, idle, blocking := false, false, false
	if p.attrs != nil {
		polling, idle, blocking = p.attrs(toolName)
	}

	// Blocking tools (await-child) park until a real external event or a
	// timeout — they are self-rate-limiting and cannot busy-spin, so the
	// repetition guard never applies (MaxTurns is still the hard ceiling).
	if blocking {
		return &api.ToolDecision{Decision: api.Allow}, nil
	}

	// Idle tools repeat by design in keepalive mode (ask-user's "wait
	// for the user" after every turn IS the chat shape). Outside
	// keepalive they keep the default threshold — three identical asks
	// from a one-shot task DO indicate a stuck loop.
	if idle && p.exemptIdle {
		return &api.ToolDecision{Decision: api.Allow}, nil
	}

	// Canonical key: json.Marshal sorts map keys, so two calls with the
	// same arguments hash identically regardless of field order or
	// whitespace in the model's raw output.
	argJSON, err := json.Marshal(input)
	if err != nil {
		argJSON = []byte(fmt.Sprintf("%v", input))
	}
	key := toolName + ":" + string(argJSON)

	threshold := defaultThreshold
	if polling {
		threshold = pollingThreshold
	}

	p.mu.Lock()
	p.counts[key]++
	count := p.counts[key]
	p.mu.Unlock()

	if count >= threshold {
		return &api.ToolDecision{
			Decision: api.Deny,
			Reason: fmt.Sprintf(
				"loop-safety: %q called %d times with identical arguments — the call is being repeated without progress. Change the arguments, take a different action, or finish the task.",
				toolName, count),
		}, nil
	}
	return &api.ToolDecision{Decision: api.Allow}, nil
}

var _ api.AgentPlugin = (*Plugin)(nil)
