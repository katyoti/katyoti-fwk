// SPDX-License-Identifier: Apache-2.0

// Package complete_ui_component implements the `complete-ui-component`
// agent tool. The terminal marker for streamed UI artifacts: marks an
// artifact as no-longer-mutable and flips its status to 'complete' in
// the GUI (visible affordance).
//
// For ephemeral one-shot artifacts emitted via display-ui-component
// without `persistent: true`, the artifact auto-completes on
// tool_use_done — calling this tool is harmless but redundant. For
// persistent artifacts, calling this tool is the only way to mark them
// done after the agent has finished a sequence of patch-ui-component
// updates (e.g., a task list where the last task just got ticked).
//
// Wired by display-dsl-framework S6 (ms06, 2026-05-05).
package complete_ui_component

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// CompleteUIComponentInput is the input schema.
type CompleteUIComponentInput struct {
	ArtifactID string `json:"artifactId" jsonschema:"The artifact id to mark complete. Must match a previously-emitted display-ui-component call." required:"true"`
}

// CompleteUIComponentOutput is the tool's result.
type CompleteUIComponentOutput struct {
	Success    bool   `json:"success"`
	ArtifactID string `json:"artifactId"`
	Message    string `json:"message"`
}

// NewCompleteUIComponentTool creates the complete-ui-component tool.
func NewCompleteUIComponentTool() toolsapi.Tool {
	return toolsapi.NewFunctionTool(
		"complete-ui-component",
		toolDescription,
		executeComplete,
		toolsapi.WithReadOnly(),
		toolsapi.WithDescriber(describeComplete),
		toolsapi.WithDisplay(toolsapi.ToolDisplay{
			Icon: "✅", Label: "Complete UI", Color: "#10B981",
			Template: "{{.artifactId}}",
		}),
	)
}

func executeComplete(ctx context.Context, in CompleteUIComponentInput) (CompleteUIComponentOutput, error) {
	if strings.TrimSpace(in.ArtifactID) == "" {
		return CompleteUIComponentOutput{}, fmt.Errorf("complete-ui-component: artifactId is required")
	}

	bus, err := application.UseService[engine.EventBus](ctx)
	if err != nil || bus == nil {
		// EventBus unavailable — log and return success anyway. The
		// agent shouldn't fail on a marker tool when the wiring is
		// missing.
		return CompleteUIComponentOutput{
			Success:    true,
			ArtifactID: in.ArtifactID,
			Message:    fmt.Sprintf("UI artifact %q marked complete (no event bus)", in.ArtifactID),
		}, nil
	}

	evt := agentapi.UIArtifactCompleteEvent{ArtifactID: in.ArtifactID}
	evt.StampEvent(agentapi.EventUIArtifactComplete, agentapi.RunIDFromContext(ctx))
	bus.Publish(agentapi.EventUIArtifactComplete, evt)

	return CompleteUIComponentOutput{
		Success:    true,
		ArtifactID: in.ArtifactID,
		Message:    fmt.Sprintf("UI artifact %q marked complete", in.ArtifactID),
	}, nil
}

func describeComplete(raw json.RawMessage) string {
	var in CompleteUIComponentInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "complete-ui-component <invalid>"
	}
	return fmt.Sprintf("complete-ui-component %q", in.ArtifactID)
}

const toolDescription = `Mark a previously-displayed UI artifact as complete.

After this tool fires, the artifact's status flips to 'complete' in the GUI
(visible badge + grayed actions where appropriate) and patch-ui-component calls
targeting this artifactId are rejected.

Use cases:
- A long-running task list where you've finished updating it across turns.
- A dashboard that streamed in over multiple patch-ui-component calls and is
  now the final view.
- Any persistent artifact that should no longer be mutated.

Skip this tool for ephemeral display-ui-component calls without persistent: true
— those auto-complete when the tool call finishes streaming.

Parameters:
- artifactId: The id you originally passed to display-ui-component.
`
