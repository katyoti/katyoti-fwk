// SPDX-License-Identifier: Apache-2.0

package assembly

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// completeToolName is the canonical tool name we filter out of the
// catalog when the run's mode is `chat`. Hardcoded
// because the tool's identity is part of the wire contract — every
// task plugin emits a `ToolDef{Name: "complete", ...}` — and a
// rename would already require an audit of dozens of unrelated
// call sites.
const completeToolName = "complete"

// chatFilterWrapper is a thin decorator that hides the
// `complete` tool from the LLM's view of an inner plugin's tools.
// All other behavior (system message, hooks, stability) passes
// through unchanged so the wrapper is invisible to anything except
// the model-facing tool catalog.
//
// Why a per-plugin decorator vs a one-shot filter at dispatch time:
// the agent loop calls `plugin.ToolDefs()` per turn (so a dynamic
// plugin can change its tool list), and `BuildDispatcher` routes
// `HandleToolCall` by tool name back to the owning plugin. Wrapping
// each plugin keeps that round-trip working — `complete` is filtered
// at the source (the plugin that owns it) and the wrapper's
// defensive `HandleToolCall` short-circuits any rare race where the
// LLM somehow tried to call it anyway (e.g. via a cached schema).
type chatFilterWrapper struct {
	inner api.AgentPlugin
}

func newChatFilterWrapper(inner api.AgentPlugin) api.AgentPlugin {
	return &chatFilterWrapper{inner: inner}
}

func (w *chatFilterWrapper) Name() string {
	return w.inner.Name()
}

func (w *chatFilterWrapper) SystemMessage() string {
	return w.inner.SystemMessage()
}

func (w *chatFilterWrapper) ToolDefs() []api.ToolDef {
	src := w.inner.ToolDefs()
	if len(src) == 0 {
		return src
	}
	// Fast path: most plugins don't own complete; if none of theirs
	// match we skip the allocation entirely.
	hasComplete := false
	for _, d := range src {
		if d.Name == completeToolName {
			hasComplete = true
			break
		}
	}
	if !hasComplete {
		return src
	}
	out := make([]api.ToolDef, 0, len(src)-1)
	for _, d := range src {
		if d.Name == completeToolName {
			continue
		}
		out = append(out, d)
	}
	return out
}

func (w *chatFilterWrapper) HandleToolCall(ctx context.Context, name string, input json.RawMessage) (*api.ToolCallResult, error) {
	// Defensive: should be unreachable because the LLM doesn't see
	// `complete` in ToolDefs(). If a stale schema or a retry races,
	// we return an explicit error so the LLM corrects on the next
	// turn instead of silently terminating the run.
	if name == completeToolName {
		return nil, fmt.Errorf("chat mode: tool %q is not available in interactive mode", name)
	}
	return w.inner.HandleToolCall(ctx, name, input)
}

func (w *chatFilterWrapper) Hooks() api.PluginHooks {
	return w.inner.Hooks()
}

func (w *chatFilterWrapper) Stable() bool {
	return w.inner.Stable()
}

// Compile-time interface check.
var _ api.AgentPlugin = (*chatFilterWrapper)(nil)
