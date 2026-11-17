// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/em"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// ToolDisplay is the rendering metadata for tool calls (legacy re-export).
// Kept for backward compat with existing tool registrations.
type ToolDisplay = agentapi.ToolDisplay

// WithDisplay sets the tool's display rendering metadata (legacy).
// New tools should register a ToolDecorator instead.
func WithDisplay(d ToolDisplay) ToolOption {
	return func(o *ToolOptions) {
		o.Display = &d
	}
}

// ── ToolDecorator Extension Point ─────────────────────────────────────

// ToolView is the display data for a tool call — UI-agnostic.
// TUI renders with lipgloss, GUI with CSS, pipe mode as plain text.
type ToolView struct {
	Icon  string `json:"icon,omitempty"`  // "📄", "🔍", "🐚"
	Label string `json:"label,omitempty"` // "Read", "Grep", "Bash"
	Color string `json:"color,omitempty"` // foreground hint
	Title string `json:"title,omitempty"` // resolved from input: "types.go"
	Body  string `json:"body,omitempty"`  // detail or result summary
}

// ToolDecorator transforms tool events into display views.
// Registered per tool name via DecoratorPoint.
type ToolDecorator interface {
	OnCall(input json.RawMessage) ToolView
	OnResult(result json.RawMessage, durationMs int) ToolView
	OnError(errMsg string, durationMs int) ToolView
}

// SimpleDecorator implements ToolDecorator with configurable functions.
// Most tools use this — only complex tools need a custom implementation.
type SimpleDecorator struct {
	Icon            string
	Label           string
	Color           string
	TitleFromInput  func(json.RawMessage) string // nil = extract first string field
	SummarizeResult func(json.RawMessage) string // nil = "{N keys}"
}

func (d SimpleDecorator) OnCall(input json.RawMessage) ToolView {
	title := ""
	if d.TitleFromInput != nil {
		title = d.TitleFromInput(input)
	} else {
		title = extractFirstStringField(input)
	}
	return ToolView{
		Icon:  d.Icon,
		Label: d.Label,
		Color: d.Color,
		Title: title,
	}
}

func (d SimpleDecorator) OnResult(result json.RawMessage, durationMs int) ToolView {
	body := ""
	if d.SummarizeResult != nil {
		body = d.SummarizeResult(result)
	} else {
		body = summarizeJSON(result)
	}
	return ToolView{
		Icon:  d.Icon,
		Label: d.Label,
		Color: d.Color,
		Body:  body,
	}
}

func (d SimpleDecorator) OnError(errMsg string, durationMs int) ToolView {
	return ToolView{
		Icon:  d.Icon,
		Label: d.Label,
		Color: d.Color,
		Body:  errMsg,
	}
}

// ── Extension point ───────────────────────────────────────────────────

// DecoratorEntry pairs a tool name with its decorator.
type DecoratorEntry struct {
	Name      string
	Decorator ToolDecorator
}

// DecoratorPoint is the EM extension point for tool decorators.
var DecoratorPoint = em.NewIndexedPoint[DecoratorEntry](
	func(e DecoratorEntry) string { return e.Name },
)

// GetDecorator returns the decorator for a tool, or a default fallback.
func GetDecorator(toolName string) ToolDecorator {
	if DecoratorPoint.Has(toolName) {
		return DecoratorPoint.Get(toolName).Decorator
	}
	return SimpleDecorator{Icon: "🔧", Label: toolName}
}

// RegisterDecorator creates a DecoratorEntry for EM contribution.
func RegisterDecorator(name string, d ToolDecorator) DecoratorEntry {
	return DecoratorEntry{Name: name, Decorator: d}
}

// ── Helpers ───────────────────────────────────────────────────────────

// extractFirstStringField tries common field names from input JSON.
func extractFirstStringField(input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}
	var data map[string]any
	if json.Unmarshal(input, &data) != nil {
		return ""
	}
	candidates := []string{
		"workspace_path", "absolute_path", "file_path", "path",
		"pattern", "command", "question", "prompt",
		"query", "url", "task_name", "skill_name", "summary",
	}
	for _, key := range candidates {
		if v, ok := data[key].(string); ok && v != "" {
			if len(v) > 80 {
				return v[:77] + "..."
			}
			return v
		}
	}
	return ""
}

// summarizeJSON produces a compact summary from a JSON result.
func summarizeJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "ok"
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if len(s) > 80 {
			return s[:77] + "..."
		}
		return s
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return string(raw)[:min(80, len(raw))]
	}
	// ask_user answer
	if answer, ok := m["answer"].(string); ok {
		if len(answer) > 60 {
			return fmt.Sprintf("%q...", answer[:57])
		}
		return fmt.Sprintf("%q", answer)
	}
	// file tools
	if fileMap, ok := m["file"].(map[string]any); ok {
		p, _ := fileMap["file_path"].(string)
		p = shortenPath(p)
		if bw, ok := m["bytes_written"].(float64); ok {
			return fmt.Sprintf("%s (%s)", p, formatBytes(int(bw)))
		}
		if content, ok := fileMap["content"].(string); ok {
			return fmt.Sprintf("%s (%d lines)", p, strings.Count(content, "\n"))
		}
		if p != "" {
			return p
		}
	}
	// glob
	if _, ok := m["filenames"].([]any); ok {
		count, _ := m["num_files"].(float64)
		if count == 0 {
			if msg, ok := m["message"].(string); ok {
				return msg
			}
			return "no files found"
		}
		return fmt.Sprintf("%d files", int(count))
	}
	// grep
	if matches, ok := m["matches"].([]any); ok {
		return fmt.Sprintf("%d matches", len(matches))
	}
	if content, ok := m["content"].(string); ok {
		return fmt.Sprintf("%d lines", strings.Count(content, "\n"))
	}
	// bash
	if stdout, ok := m["stdout"].(string); ok {
		lines := strings.Count(stdout, "\n")
		exitCode, _ := m["exit_code"].(float64)
		if exitCode != 0 {
			return fmt.Sprintf("exit %d (%d lines)", int(exitCode), lines)
		}
		return fmt.Sprintf("ok (%d lines)", lines)
	}
	// message field
	if msg, ok := m["message"].(string); ok {
		return msg
	}
	return fmt.Sprintf("{%d keys}", len(m))
}

func shortenPath(p string) string {
	parts := strings.Split(strings.ReplaceAll(p, "\\", "/"), "/")
	if len(parts) > 3 {
		return "…/" + strings.Join(parts[len(parts)-3:], "/")
	}
	return filepath.Base(p)
}

func formatBytes(b int) string {
	switch {
	case b >= 1024*1024:
		return fmt.Sprintf("%.1fMB", float64(b)/(1024*1024))
	case b >= 1024:
		return fmt.Sprintf("%.1fKB", float64(b)/1024)
	default:
		return fmt.Sprintf("%dB", b)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
