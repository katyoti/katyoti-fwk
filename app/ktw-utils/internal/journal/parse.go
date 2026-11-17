// SPDX-License-Identifier: Apache-2.0

package journal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Conversation represents the parsed conversation from a session.
type Conversation struct {
	SessionID     string
	Task          string
	Model         string
	Provider      string
	SystemMessage string   // system prompt sent to the LLM
	ToolNames     []string // available tool names
	Turns         []Turn
	TotalTok      int
	Duration      int // ms
}

// Turn represents one agent turn: user message(s) + assistant response + tool interactions.
type Turn struct {
	Number     int
	UserText   string // combined user text blocks
	AssistText string // combined assistant text blocks
	ToolCalls  []ToolCallInfo
	DurationMs int
	InputTok   int
	OutputTok  int
}

// ToolCallInfo captures a tool call and its result.
type ToolCallInfo struct {
	Name   string
	Input  string // brief summary of input
	Result string // brief summary of result
	Error  bool
}

// jsonEntry is the raw JSONL entry structure.
type jsonEntry struct {
	Type string          `json:"type"`
	Seq  int             `json:"seq"`
	Turn int             `json:"turn"`
	Data json.RawMessage `json:"data"`
}

// ParseConversation reads llm.jsonl and extracts a structured conversation.
func ParseConversation(sessionDir string) (*Conversation, error) {
	f, err := os.Open(filepath.Join(sessionDir, "llm.jsonl"))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	conv := &Conversation{}
	turnMap := make(map[int]*Turn)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB line buffer

	for scanner.Scan() {
		var entry jsonEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}

		switch entry.Type {
		case "session_start":
			var d struct {
				Model    string `json:"model"`
				Provider string `json:"provider"`
				Task     string `json:"task"`
			}
			json.Unmarshal(entry.Data, &d)
			conv.Task = d.Task
			conv.Model = d.Model
			conv.Provider = d.Provider

		case "llm_request":
			var d struct {
				Turn          int      `json:"turn"`
				SystemMessage string   `json:"systemPrompt"`
				ToolNames     []string `json:"toolNames"`
				Messages      []struct {
					Role    string `json:"role"`
					Content []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
				} `json:"messages"`
			}
			json.Unmarshal(entry.Data, &d)

			if d.SystemMessage != "" {
				conv.SystemMessage = d.SystemMessage
			}
			if len(d.ToolNames) > 0 {
				conv.ToolNames = d.ToolNames
			}

			t := getOrCreateTurn(turnMap, entry.Turn)

			// Extract user text from the last user message
			for i := len(d.Messages) - 1; i >= 0; i-- {
				msg := d.Messages[i]
				if msg.Role == "user" {
					var texts []string
					for _, block := range msg.Content {
						if block.Type == "text" && block.Text != "" {
							texts = append(texts, block.Text)
						}
					}
					if len(texts) > 0 && t.UserText == "" {
						t.UserText = strings.Join(texts, "\n")
					}
					break
				}
			}

		case "llm_response":
			var d struct {
				Turn       int `json:"turn"`
				DurationMs int `json:"durationMs"`
				Content    []struct {
					Type  string          `json:"type"`
					Text  string          `json:"text"`
					Name  string          `json:"name"`
					Input json.RawMessage `json:"input"`
				} `json:"content"`
				Usage struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			}
			json.Unmarshal(entry.Data, &d)

			t := getOrCreateTurn(turnMap, entry.Turn)
			t.DurationMs = d.DurationMs
			t.InputTok = d.Usage.InputTokens
			t.OutputTok = d.Usage.OutputTokens

			var texts []string
			for _, block := range d.Content {
				if block.Type == "text" && block.Text != "" {
					texts = append(texts, block.Text)
				}
			}
			if len(texts) > 0 {
				t.AssistText = strings.Join(texts, "\n")
			}

		case "tool_call":
			var d struct {
				Tool  string          `json:"tool"`
				Input json.RawMessage `json:"input"`
			}
			json.Unmarshal(entry.Data, &d)

			t := getOrCreateTurn(turnMap, entry.Turn)
			tc := ToolCallInfo{
				Name:  d.Tool,
				Input: summarizeInput(d.Tool, d.Input),
			}
			t.ToolCalls = append(t.ToolCalls, tc)

		case "tool_result":
			var d struct {
				Tool   string `json:"tool"`
				Result string `json:"result"`
			}
			json.Unmarshal(entry.Data, &d)

			t := getOrCreateTurn(turnMap, entry.Turn)
			// Match to last unresolved tool call of same name
			for i := len(t.ToolCalls) - 1; i >= 0; i-- {
				if t.ToolCalls[i].Name == d.Tool && t.ToolCalls[i].Result == "" {
					t.ToolCalls[i].Result = summarizeResult(d.Result)
					if isErrorResult(d.Result) {
						t.ToolCalls[i].Error = true
					}
					break
				}
			}

		case "session_end":
			var d struct {
				DurationMs int `json:"durationMs"`
				Turns      int `json:"turns"`
				Usage      struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			}
			json.Unmarshal(entry.Data, &d)
			conv.Duration = d.DurationMs
			conv.TotalTok = d.Usage.InputTokens + d.Usage.OutputTokens
		}
	}

	// Collect turns in order
	maxTurn := 0
	for k := range turnMap {
		if k > maxTurn {
			maxTurn = k
		}
	}
	for i := 1; i <= maxTurn; i++ {
		if t, ok := turnMap[i]; ok {
			conv.Turns = append(conv.Turns, *t)
		}
	}

	return conv, scanner.Err()
}

func getOrCreateTurn(m map[int]*Turn, turn int) *Turn {
	if t, ok := m[turn]; ok {
		return t
	}
	t := &Turn{Number: turn}
	m[turn] = t
	return t
}

func summarizeInput(tool string, raw json.RawMessage) string {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	switch tool {
	case "ask-user":
		if q, ok := m["question"].(string); ok {
			return q
		}
	case "write_file":
		if p, ok := m["path"].(string); ok {
			return p
		}
	case "read_file":
		if p, ok := m["path"].(string); ok {
			return p
		}
	case "edit_file":
		if p, ok := m["path"].(string); ok {
			return p
		}
	case "notify":
		if msg, ok := m["message"].(string); ok {
			if len(msg) > 80 {
				return msg[:80] + "..."
			}
			return msg
		}
	case "complete", "task_complete":
		if s, ok := m["summary"].(string); ok {
			if len(s) > 100 {
				return s[:100] + "..."
			}
			return s
		}
	case "skills-get-instruction":
		if name, ok := m["skill_name"].(string); ok {
			return name
		}
		// `skills-get-reference` case dropped 2026-05-24 — the tool was
		// retired; references are fetched via the generic `contributions-get`
		// tool now, whose input shape (`{"uri": "..."}`) is parsed by the
		// generic contribution arm of this switch (or falls through to the
		// raw-input default).
	}
	// Generic: show first string field
	for _, v := range m {
		if s, ok := v.(string); ok {
			if len(s) > 80 {
				return s[:80] + "..."
			}
			return s
		}
	}
	return ""
}

func summarizeResult(result string) string {
	if len(result) > 200 {
		return result[:200] + "..."
	}
	return result
}

func isErrorResult(result string) bool {
	return strings.Contains(result, `"status":"error"`) ||
		strings.Contains(result, `"error":`)
}

// RenderMarkdown renders the conversation as a human-readable markdown string.
func RenderMarkdown(conv *Conversation) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# Session: %s (%s)\n\n", conv.Task, formatModel(conv.Model)))
	b.WriteString(fmt.Sprintf("**Provider**: %s | **Turns**: %d | **Tokens**: %d | **Duration**: %s\n\n",
		conv.Provider, len(conv.Turns), conv.TotalTok, formatDuration(conv.Duration)))

	if len(conv.ToolNames) > 0 {
		b.WriteString("**Tools**: ")
		b.WriteString(strings.Join(conv.ToolNames, ", "))
		b.WriteString("\n\n")
	}

	if conv.SystemMessage != "" {
		b.WriteString("<details>\n<summary>System Message</summary>\n\n")
		b.WriteString("```\n")
		b.WriteString(conv.SystemMessage)
		if !strings.HasSuffix(conv.SystemMessage, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("```\n\n")
		b.WriteString("</details>\n\n")
	}

	b.WriteString("---\n\n")

	for _, t := range conv.Turns {
		b.WriteString(fmt.Sprintf("## Turn %d\n\n", t.Number))

		if t.UserText != "" && t.UserText != "Please start the task." {
			b.WriteString("### User\n\n")
			b.WriteString(t.UserText)
			b.WriteString("\n\n")
		}

		if t.AssistText != "" {
			b.WriteString("### Assistant\n\n")
			b.WriteString(t.AssistText)
			b.WriteString("\n\n")
		}

		for _, tc := range t.ToolCalls {
			status := "✓"
			if tc.Error {
				status = "✗"
			}
			if tc.Input != "" {
				b.WriteString(fmt.Sprintf("**%s** `%s` → %s %s\n\n", status, tc.Name, tc.Input, formatToolResult(tc)))
			} else {
				b.WriteString(fmt.Sprintf("**%s** `%s`%s\n\n", status, tc.Name, formatToolResult(tc)))
			}
		}

		if t.DurationMs > 0 {
			b.WriteString(fmt.Sprintf("*%s, %d+%d tokens*\n\n",
				formatDuration(t.DurationMs), t.InputTok, t.OutputTok))
		}

		b.WriteString("---\n\n")
	}

	return b.String()
}

func formatModel(model string) string {
	// Shorten common model names
	model = strings.ReplaceAll(model, "claude-haiku-4-5-20251001", "claude-haiku-4.5")
	model = strings.ReplaceAll(model, "claude-sonnet-4-5-20250514", "claude-sonnet-4.5")
	return model
}

func formatDuration(ms int) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.1fs", float64(ms)/1000)
}

func formatToolResult(tc ToolCallInfo) string {
	if tc.Result == "" {
		return ""
	}
	r := tc.Result
	if len(r) > 100 {
		r = r[:100] + "..."
	}
	if tc.Error {
		return "\n  Error: " + r
	}
	return " → " + r
}
