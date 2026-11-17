// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"encoding/json"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// LocalToolDispatcher implements agentapi.ToolDispatcher by wrapping another
// ToolDispatcher. Used when the engine runs standalone (not as a KTSP container).
type LocalToolDispatcher struct {
	dispatcher agentapi.ToolDispatcher
}

// NewLocalToolDispatcher creates a tool dispatcher that wraps the given ToolDispatcher.
func NewLocalToolDispatcher(d agentapi.ToolDispatcher) *LocalToolDispatcher {
	return &LocalToolDispatcher{dispatcher: d}
}

// Dispatch routes a tool call to the registered handler.
func (d *LocalToolDispatcher) Dispatch(ctx context.Context, name string, input json.RawMessage) (string, error) {
	return d.dispatcher.Dispatch(ctx, name, input)
}

// ToolDefs returns all registered tool definitions.
func (d *LocalToolDispatcher) ToolDefs() []agentapi.ToolDef {
	return d.dispatcher.ToolDefs()
}
