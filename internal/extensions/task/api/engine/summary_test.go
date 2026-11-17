// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestComputeSummary_FromSampleJournals(t *testing.T) {
	dir := t.TempDir()

	// Write a sample LLM journal
	llmPath := filepath.Join(dir, "test.llm.jsonl")
	llmContent := `{"type":"session_start","seq":1,"turn":0,"ts":"2026-03-01T10:00:00Z","data":{"task":"test-task","model":"claude-3","provider":"anthropic"}}
{"type":"llm_request","seq":2,"turn":1,"ts":"2026-03-01T10:00:01Z","data":{"turn":1,"message_count":1,"tool_count":3}}
{"type":"llm_response","seq":3,"turn":1,"ts":"2026-03-01T10:00:03Z","data":{"turn":1,"stopReason":"tool_use","blocks":2,"usage":{"input_tokens":100,"output_tokens":50},"durationMs":2000,"ttftMs":500}}
{"type":"tool_call","seq":4,"turn":1,"ts":"2026-03-01T10:00:03Z","data":{"turn":1,"tool":"ask-user","id":"tc1","input":{"question":"Name?"}}}
{"type":"tool_result","seq":6,"turn":1,"ts":"2026-03-01T10:00:10Z","data":{"turn":1,"tool":"ask-user","id":"tc1","result":"Alice","durationMs":7000}}
{"type":"llm_request","seq":7,"turn":2,"ts":"2026-03-01T10:00:11Z","data":{"turn":2,"message_count":3,"tool_count":3}}
{"type":"llm_response","seq":8,"turn":2,"ts":"2026-03-01T10:00:13Z","data":{"turn":2,"stopReason":"tool_use","blocks":1,"usage":{"input_tokens":200,"output_tokens":80},"durationMs":1500,"ttftMs":400}}
{"type":"tool_call","seq":9,"turn":2,"ts":"2026-03-01T10:00:13Z","data":{"turn":2,"tool":"write_file","id":"tc2","input":{"path":"out.md"}}}
{"type":"tool_result","seq":10,"turn":2,"ts":"2026-03-01T10:00:14Z","data":{"turn":2,"tool":"write_file","id":"tc2","result":"ok","durationMs":100}}
{"type":"session_end","seq":11,"turn":2,"ts":"2026-03-01T10:00:15Z","data":{"turns":2,"usage":{"input_tokens":300,"output_tokens":130},"durationMs":15000}}
`
	if err := os.WriteFile(llmPath, []byte(llmContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write a sample user journal
	userPath := filepath.Join(dir, "test.user.jsonl")
	userContent := `{"type":"session_start","seq":1,"turn":0,"ts":"2026-03-01T10:00:00Z","data":{"task":"test-task"}}
{"type":"message_shown","seq":3,"turn":1,"ts":"2026-03-01T10:00:03Z","data":{"text":"Hello! What is your name?"}}
{"type":"prompt_shown","seq":5,"turn":1,"tool_id":"tc1","ts":"2026-03-01T10:00:03Z","data":{"question":"Name?","type":"text"}}
{"type":"user_input","seq":6,"turn":1,"tool_id":"tc1","ts":"2026-03-01T10:00:10Z","data":{"answer":"Alice"}}
{"type":"tool_feedback","seq":10,"turn":2,"tool_id":"tc2","ts":"2026-03-01T10:00:14Z","data":{"tool":"write_file","summary":"ok"}}
{"type":"session_end","seq":11,"turn":2,"ts":"2026-03-01T10:00:15Z","data":{"turns":2,"durationMs":15000}}
`
	if err := os.WriteFile(userPath, []byte(userContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Compute summary
	summary, err := ComputeSummary(llmPath, userPath)
	if err != nil {
		t.Fatalf("ComputeSummary: %v", err)
	}

	// Verify metrics
	if summary.Task != "test-task" {
		t.Errorf("Task = %q, want 'test-task'", summary.Task)
	}
	if summary.Model != "claude-3" {
		t.Errorf("Model = %q, want 'claude-3'", summary.Model)
	}
	if summary.Turns != 2 {
		t.Errorf("Turns = %d, want 2", summary.Turns)
	}
	if summary.InputTokens != 300 {
		t.Errorf("InputTokens = %d, want 300", summary.InputTokens)
	}
	if summary.OutputTokens != 130 {
		t.Errorf("OutputTokens = %d, want 130", summary.OutputTokens)
	}
	if summary.TotalTokens != 430 {
		t.Errorf("TotalTokens = %d, want 430", summary.TotalTokens)
	}
	if summary.DurationMs != 15000 {
		t.Errorf("DurationMs = %d, want 15000", summary.DurationMs)
	}
	if summary.AskUserCount != 1 {
		t.Errorf("AskUserCount = %d, want 1", summary.AskUserCount)
	}
	if summary.ContentOnlyTurns != 0 {
		t.Errorf("ContentOnlyTurns = %d, want 0", summary.ContentOnlyTurns)
	}

	// Tool calls
	if summary.ToolCalls["ask-user"] != 1 {
		t.Errorf("ToolCalls[ask_user] = %d, want 1", summary.ToolCalls["ask-user"])
	}
	if summary.ToolCalls["write_file"] != 1 {
		t.Errorf("ToolCalls[write_file] = %d, want 1", summary.ToolCalls["write_file"])
	}

	// Timing averages
	if summary.AvgLLMDurationMs != 1750 { // (2000+1500)/2
		t.Errorf("AvgLLMDurationMs = %d, want 1750", summary.AvgLLMDurationMs)
	}
	if summary.AvgTTFTMs != 450 { // (500+400)/2
		t.Errorf("AvgTTFTMs = %d, want 450", summary.AvgTTFTMs)
	}

	// Save JSON
	summaryPath := filepath.Join(dir, "test.summary.json")
	if err := summary.SaveJSON(summaryPath); err != nil {
		t.Fatalf("SaveJSON: %v", err)
	}
	if _, err := os.Stat(summaryPath); err != nil {
		t.Errorf("Summary JSON file not found: %v", err)
	}
}

func TestComputeSummary_EmptyPaths(t *testing.T) {
	summary, err := ComputeSummary("", "")
	if err != nil {
		t.Fatalf("ComputeSummary with empty paths: %v", err)
	}
	if summary.Turns != 0 {
		t.Errorf("Turns = %d, want 0", summary.Turns)
	}
}
