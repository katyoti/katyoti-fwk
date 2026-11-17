// SPDX-License-Identifier: Apache-2.0

package journal

import (
	"os"
	"path/filepath"
	"testing"
)

// Minimal JSONL fixture covering the main event types.
const testJSONL = `{"type":"session_start","seq":1,"turn":0,"data":{"model":"claude-haiku-4-5-20251001","provider":"anthropic","task":"test-task"}}
{"type":"llm_request","seq":2,"turn":1,"data":{"messages":[{"role":"user","content":[{"type":"text","text":"Please start the task."}]}],"systemPrompt":"You are a test assistant.","toolNames":["ask-user","write_file"],"turn":1}}
{"type":"llm_response","seq":3,"turn":1,"data":{"content":[{"type":"text","text":"Hello! I will ask you a question."},{"type":"tool_use","name":"ask-user","input":{"question":"What color?"}}],"durationMs":1200,"turn":1,"usage":{"input_tokens":500,"output_tokens":50}}}
{"type":"tool_call","seq":4,"turn":1,"data":{"tool":"ask-user","input":{"question":"What color?","type":"text"},"turn":1}}
{"type":"tool_result","seq":5,"turn":1,"data":{"tool":"ask-user","result":"{\"answer\":\"blue\"}","turn":1}}
{"type":"llm_request","seq":6,"turn":2,"data":{"messages":[{"role":"user","content":[{"type":"text","text":"blue"}]}],"turn":2}}
{"type":"llm_response","seq":7,"turn":2,"data":{"content":[{"type":"text","text":"Great choice! Writing a file now."},{"type":"tool_use","name":"write_file","input":{"path":"output.md"}}],"durationMs":800,"turn":2,"usage":{"input_tokens":600,"output_tokens":40}}}
{"type":"tool_call","seq":8,"turn":2,"data":{"tool":"write_file","input":{"path":"output.md","content":"# Blue"},"turn":2}}
{"type":"tool_result","seq":9,"turn":2,"data":{"tool":"write_file","result":"ok","turn":2}}
{"type":"session_end","seq":10,"turn":0,"data":{"durationMs":5000,"turns":2,"usage":{"input_tokens":1100,"output_tokens":90}}}
`

func TestParseConversation(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "llm.jsonl"), []byte(testJSONL), 0o644); err != nil {
		t.Fatal(err)
	}

	conv, err := ParseConversation(dir)
	if err != nil {
		t.Fatalf("ParseConversation: %v", err)
	}

	// Session metadata
	if conv.Task != "test-task" {
		t.Errorf("Task = %q, want %q", conv.Task, "test-task")
	}
	if conv.Model != "claude-haiku-4-5-20251001" {
		t.Errorf("Model = %q, want %q", conv.Model, "claude-haiku-4-5-20251001")
	}
	if conv.Provider != "anthropic" {
		t.Errorf("Provider = %q, want %q", conv.Provider, "anthropic")
	}
	if conv.SystemMessage != "You are a test assistant." {
		t.Errorf("SystemMessage = %q, want %q", conv.SystemMessage, "You are a test assistant.")
	}
	if len(conv.ToolNames) != 2 {
		t.Errorf("ToolNames len = %d, want 2", len(conv.ToolNames))
	}

	// Turns
	if len(conv.Turns) != 2 {
		t.Fatalf("Turns = %d, want 2", len(conv.Turns))
	}

	// Turn 1
	t1 := conv.Turns[0]
	if t1.Number != 1 {
		t.Errorf("Turn 1 Number = %d", t1.Number)
	}
	if t1.AssistText != "Hello! I will ask you a question." {
		t.Errorf("Turn 1 AssistText = %q", t1.AssistText)
	}
	if len(t1.ToolCalls) != 1 {
		t.Fatalf("Turn 1 ToolCalls = %d, want 1", len(t1.ToolCalls))
	}
	if t1.ToolCalls[0].Name != "ask-user" {
		t.Errorf("Turn 1 tool = %q, want ask_user", t1.ToolCalls[0].Name)
	}
	if t1.ToolCalls[0].Input != "What color?" {
		t.Errorf("Turn 1 tool input = %q", t1.ToolCalls[0].Input)
	}
	if t1.DurationMs != 1200 {
		t.Errorf("Turn 1 DurationMs = %d, want 1200", t1.DurationMs)
	}

	// Turn 2
	t2 := conv.Turns[1]
	if t2.AssistText != "Great choice! Writing a file now." {
		t.Errorf("Turn 2 AssistText = %q", t2.AssistText)
	}
	if len(t2.ToolCalls) != 1 {
		t.Fatalf("Turn 2 ToolCalls = %d, want 1", len(t2.ToolCalls))
	}
	if t2.ToolCalls[0].Name != "write_file" {
		t.Errorf("Turn 2 tool = %q, want write_file", t2.ToolCalls[0].Name)
	}
	if t2.ToolCalls[0].Input != "output.md" {
		t.Errorf("Turn 2 tool input = %q, want output.md", t2.ToolCalls[0].Input)
	}

	// Session totals
	if conv.Duration != 5000 {
		t.Errorf("Duration = %d, want 5000", conv.Duration)
	}
	if conv.TotalTok != 1190 {
		t.Errorf("TotalTok = %d, want 1190", conv.TotalTok)
	}
}

func TestRenderMarkdown(t *testing.T) {
	conv := &Conversation{
		Task:     "test-task",
		Model:    "claude-haiku-4-5-20251001",
		Provider: "anthropic",
		TotalTok: 1190,
		Duration: 5000,
		Turns: []Turn{
			{
				Number:     1,
				AssistText: "Hello!",
				ToolCalls: []ToolCallInfo{
					{Name: "ask-user", Input: "What color?", Result: "blue"},
				},
				DurationMs: 1200,
				InputTok:   500,
				OutputTok:  50,
			},
		},
	}

	md := RenderMarkdown(conv)
	if md == "" {
		t.Fatal("RenderMarkdown returned empty string")
	}
	// Check key sections exist
	for _, want := range []string{"# Session: test-task", "## Turn 1", "Hello!", "ask-user"} {
		if !contains(md, want) {
			t.Errorf("markdown missing %q", want)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
