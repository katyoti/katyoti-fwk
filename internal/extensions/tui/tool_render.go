// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
)

// renderToolStart produces a one-line tool-call display from ToolStartMsg.
// Result will be appended on the SAME message via renderToolAppendResult.
//
// With Display:  "  📄 Read   internal/kts/protocol/ktsp/types.go"
// Without:       "  Calling file_read..."
func renderToolStart(msg ToolStartMsg) string {
	d := msg.Display
	if d == nil || d.Label == "" {
		return fmt.Sprintf("  Calling %s...", msg.Name)
	}

	icon := d.Icon
	if icon != "" {
		icon += " "
	}

	// Try template first, then smart field extraction, then tool name.
	body := renderTemplate(d.Template, msg.Input)
	if body == "" {
		body = extractBody(msg.Input)
	}
	if body == "" {
		body = msg.Name
	}

	return fmt.Sprintf("  %s%-6s %s", icon, d.Label, body)
}

// extractBody tries common field names from the tool input JSON, returning
// the first non-empty string value found. This handles LLM variations
// (workspace_path vs file_path vs path, question vs prompt, etc.)
func extractBody(inputJSON string) string {
	if inputJSON == "" {
		return ""
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(inputJSON), &data); err != nil {
		return ""
	}

	// Try field names in priority order — covers the most common tools.
	candidates := []string{
		"workspace_path", "absolute_path", "file_path", "path",
		"pattern", "command", "question", "prompt",
		"query", "url", "task_name", "skill_name",
		"summary",
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

// renderToolAppendResult produces a compact result suffix to append to the
// tool-call line. This replaces the separate ToolDoneMsg rendering — the
// call and result appear as ONE entry, not two.
//
// Success: "  📄 Read   types.go → ok (2.4KB)"
// Error:   "  📄 Read   types.go → ERROR: file not found"
func renderToolAppendResult(msg ToolDoneMsg) string {
	summary := compactResult(msg.Result, msg.IsErr)
	if msg.IsErr {
		return fmt.Sprintf("  ⚠  → %s", summary)
	}
	return fmt.Sprintf("       → %s", summary)
}

// compactResult extracts a short human-readable summary from the raw tool
// result JSON instead of dumping the full JSON object.
func compactResult(raw string, isErr bool) string {
	if isErr {
		if len(raw) > 100 {
			return raw[:97] + "..."
		}
		return raw
	}

	// Try to extract a meaningful summary from common result shapes.
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err == nil {
		return summarizeMap(m)
	}

	// Plain string result (e.g., "complete")
	if len(raw) > 80 {
		return raw[:77] + "..."
	}
	return raw
}

// summarizeMap produces a compact one-line summary from a tool result map.
func summarizeMap(m map[string]any) string {
	// file-read/file-write: show file path + size hint
	if fileMap, ok := m["file"].(map[string]any); ok {
		path, _ := fileMap["file_path"].(string)
		path = shortenPath(path)
		if bytesWritten, ok := m["bytes_written"].(float64); ok {
			return fmt.Sprintf("%s (%s)", path, formatBytes(int(bytesWritten)))
		}
		if content, ok := fileMap["content"].(string); ok {
			lines := strings.Count(content, "\n")
			return fmt.Sprintf("%s (%d lines)", path, lines)
		}
		if path != "" {
			return path
		}
	}

	// glob: show file count
	if _, ok := m["filenames"].([]any); ok {
		count, _ := m["num_files"].(float64)
		if count == 0 {
			msg, _ := m["message"].(string)
			if msg != "" {
				return msg
			}
			return "no files found"
		}
		return fmt.Sprintf("%d files", int(count))
	}

	// grep: show match info
	if matches, ok := m["matches"].([]any); ok {
		return fmt.Sprintf("%d matches", len(matches))
	}
	if content, ok := m["content"].(string); ok {
		lines := strings.Count(content, "\n")
		return fmt.Sprintf("%d lines", lines)
	}

	// bash: show exit code + truncated output
	if stdout, ok := m["stdout"].(string); ok {
		lines := strings.Count(stdout, "\n")
		exitCode, _ := m["exit_code"].(float64)
		if exitCode != 0 {
			return fmt.Sprintf("exit %d (%d lines)", int(exitCode), lines)
		}
		return fmt.Sprintf("ok (%d lines)", lines)
	}

	// ask-user: show answer
	if answer, ok := m["answer"].(string); ok {
		if len(answer) > 60 {
			return fmt.Sprintf("%q...", answer[:57])
		}
		return fmt.Sprintf("%q", answer)
	}

	// Generic: show key count
	return fmt.Sprintf("{%d keys}", len(m))
}

func shortenPath(path string) string {
	// Show only the last 2-3 path segments
	parts := strings.Split(strings.ReplaceAll(path, "\\", "/"), "/")
	if len(parts) > 3 {
		return "…/" + strings.Join(parts[len(parts)-3:], "/")
	}
	return path
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

// renderTemplate executes a Go text/template against the JSON input.
// Returns empty string on any failure — the caller falls back to the tool name.
//
// Uses missingkey=error so a template referencing a key the LLM didn't send
// (e.g., {{.file_path}} when the input has no file_path) falls back cleanly
// instead of rendering "<no value>".
func renderTemplate(tmpl string, inputJSON string) string {
	if tmpl == "" || inputJSON == "" {
		return ""
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(inputJSON), &data); err != nil {
		return ""
	}

	t, err := template.New("").Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return ""
	}

	var buf strings.Builder
	if err := t.Execute(&buf, data); err != nil {
		return "" // missing key → fall back to tool name
	}

	result := strings.TrimSpace(buf.String())
	if result == "" || result == "<nil>" {
		return ""
	}
	if len(result) > 100 {
		result = result[:97] + "..."
	}
	return result
}
