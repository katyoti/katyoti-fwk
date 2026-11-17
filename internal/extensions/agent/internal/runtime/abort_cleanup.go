// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"encoding/json"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	llm "github.com/katyoti/katyoti-fwk/internal/models/api"
)

// abortCleanupPlugin generates synthetic tool_result messages for orphaned tool_use
// blocks when the run is aborted. Maintains conversation state integrity.
type abortCleanupPlugin struct{}

func (p *abortCleanupPlugin) Name() string            { return "abort_cleanup" }
func (p *abortCleanupPlugin) SystemMessage() string   { return "" }
func (p *abortCleanupPlugin) ToolDefs() []api.ToolDef { return nil }
func (p *abortCleanupPlugin) Stable() bool            { return true }
func (p *abortCleanupPlugin) HandleToolCall(_ context.Context, _ string, _ json.RawMessage) (*api.ToolCallResult, error) {
	return nil, nil
}

func (p *abortCleanupPlugin) Hooks() api.PluginHooks {
	return api.PluginHooks{
		OnAbort: func(ctx api.LoopContext) error {
			messages := ctx.Messages()
			if len(messages) == 0 {
				return nil
			}

			// Find orphaned tool calls in the last assistant message
			last := messages[len(messages)-1]
			if last.Role != api.RoleAssistant {
				return nil
			}

			var orphanedResults []llm.ToolCallResponse
			for _, part := range last.Parts {
				if tc, ok := part.(llm.ToolCall); ok {
					orphanedResults = append(orphanedResults, llm.ToolCallResponse{
						ToolCallID: tc.ID,
						Name:       tc.FunctionCall.Name,
						Content:    "Aborted by user.",
					})
				}
			}

			if len(orphanedResults) == 0 {
				return nil
			}

			for _, msg := range api.NewToolResultMessages(orphanedResults...) {
				ctx.PushMessage(msg)
			}
			return nil
		},
	}
}

// NewAbortCleanupPlugin creates the abort cleanup plugin.
func NewAbortCleanupPlugin() api.AgentPlugin {
	return &abortCleanupPlugin{}
}
