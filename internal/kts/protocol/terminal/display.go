// SPDX-License-Identifier: Apache-2.0

package terminal

import "encoding/json"

// DisplayData is the resolved rendering info for a tool call.
// Carried in ToolContent.Display and Message.Metadata.Display.
// UI-agnostic — TUI renders with lipgloss, GUI with CSS.
type DisplayData struct {
	Icon  string `json:"icon,omitempty"`  // "📄", "🔍", "🐚"
	Label string `json:"label,omitempty"` // "Read", "Grep", "Bash"
	Color string `json:"color,omitempty"` // "#82AAFF" — UI interprets
	Title string `json:"title,omitempty"` // "types.go" — resolved from input
	Body  string `json:"body,omitempty"`  // optional detail line
}

// DisplayResolver produces rendering metadata from tool input.
// Each tool registers a resolver. The Terminal Translator calls it
// before emitting a ToolContent message.
type DisplayResolver func(toolName string, input json.RawMessage) DisplayData

// DefaultDisplayResolver extracts the first string field from input as title.
func DefaultDisplayResolver(toolName string, input json.RawMessage) DisplayData {
	title := extractFirstString(input)
	return DisplayData{Label: toolName, Title: title}
}

func extractFirstString(input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(input, &m); err != nil {
		return ""
	}
	candidates := []string{
		"workspace_path", "absolute_path", "file_path", "path",
		"pattern", "command", "question", "prompt",
		"query", "url", "task_name", "skill_name", "summary",
	}
	for _, key := range candidates {
		if v, ok := m[key].(string); ok && v != "" {
			if len(v) > 80 {
				return v[:77] + "..."
			}
			return v
		}
	}
	return ""
}
