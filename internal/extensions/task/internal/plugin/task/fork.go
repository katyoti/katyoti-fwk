// SPDX-License-Identifier: Apache-2.0

package task

import (
	"strings"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/presets"
	llm "github.com/katyoti/katyoti-fwk/internal/models/api"
)

const (
	// forkBoilerplateTag is injected into forked messages to prevent recursive forking.
	forkBoilerplateTag = "[FORK_CONTEXT: You are a forked sub-agent. Do NOT fork again.]"

	// forkPlaceholder replaces tool result content in forked messages for prompt cache sharing.
	forkPlaceholder = "[tool result omitted for cache efficiency]"
)

// ForkManager builds forked messages and synthetic task definitions for fork sub-agents.
type ForkManager struct{}

// NewForkManager creates a ForkManager.
func NewForkManager() *ForkManager {
	return &ForkManager{}
}

// IsInForkChild returns true if the messages contain the fork boilerplate tag,
// indicating this agent is already a fork child and should not re-fork.
func (fm *ForkManager) IsInForkChild(messages []agentapi.Message) bool {
	for _, msg := range messages {
		for _, part := range msg.Parts {
			if tc, ok := part.(llm.TextContent); ok {
				if strings.Contains(tc.Text, forkBoilerplateTag) {
					return true
				}
			}
		}
	}
	return false
}

// BuildForkedMessages creates forked messages from the parent's conversation history.
// Tool result content is replaced with placeholders for prompt cache sharing.
// A child directive and boilerplate tag are appended.
func (fm *ForkManager) BuildForkedMessages(parentMessages []agentapi.Message, childPrompt string) []agentapi.Message {
	forked := make([]agentapi.Message, 0, len(parentMessages)+1)

	for _, msg := range parentMessages {
		// Deep copy parts — replace tool result content with placeholder
		parts := make([]llm.ContentPart, len(msg.Parts))
		for i, part := range msg.Parts {
			if tr, ok := part.(llm.ToolCallResponse); ok {
				tr.Content = forkPlaceholder
				parts[i] = tr
			} else {
				parts[i] = part
			}
		}
		forked = append(forked, agentapi.Message{Role: msg.Role, Parts: parts})
	}

	// Append child directive + boilerplate tag as a user message
	forked = append(forked, agentapi.NewTextMessage(agentapi.RoleUser, forkBoilerplateTag+"\n\n"+childPrompt))

	return forked
}

// BuildForkTaskDef creates an inline task definition for a fork sub-agent.
// The fork agent inherits the parent's prompt (body) and uses the forked messages
// as initial conversation context via the InitialMessages field.
func (fm *ForkManager) BuildForkTaskDef(parentBody string, childPrompt string, forkedMessages []agentapi.Message, plugins []any) map[string]any {
	if plugins == nil {
		plugins = []any{"augmented"}
	}
	return map[string]any{
		"name":        "fork",
		"type":        "agent",
		"description": "Fork sub-agent: " + truncate(childPrompt, 80),
		"plugins":     plugins,
		"body":        parentBody,

		presets.ConfigKeyCapability:        "fast",
		presets.OverrideKeyInitialMessages: forkedMessages,
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
