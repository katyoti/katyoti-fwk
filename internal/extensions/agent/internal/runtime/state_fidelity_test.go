// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine/enginetest"
	llm "github.com/katyoti/katyoti-fwk/internal/models/api"
)

// THE AGENT'S SAVED STATE READS BACK WHOLE — what a restart reads of an agent
// (its conversation, every kind of part, its usage, its last response, a batch
// stopped on an awaiting call) is what was saved, field for field.
//
// The differences that are the JSON's, and that the agent's code reads as such:
//   - a number in a free-form map (a model's GenerationInfo, a structured
//     output, a tool's Resume) reads back as float64 — written float64 here;
//   - a cache instruction's Duration is not saved (json:"-");
//   - a message whose one part is an empty text reads back with no part.
func TestAnAgentsStateReadsBackWhole(t *testing.T) {
	saved := &agentState{
		TaskName: "dossier",
		Messages: []api.Message{
			{Role: llm.ChatMessageTypeSystem, Parts: []llm.ContentPart{llm.TextContent{Text: "you are"}}},
			{Role: llm.ChatMessageTypeHuman, Parts: []llm.ContentPart{
				llm.TextContent{Text: "look"},
				llm.ImageURLContent{URL: "https://x.test/y.png", Detail: "high"},
				llm.BinaryContent{MIMEType: "application/pdf", Data: []byte{0, 1, 2, 255}},
				llm.WithCacheControl(llm.TextContent{Text: "cached"}, &llm.CacheControl{Type: "ephemeral"}),
			}},
			{Role: llm.ChatMessageTypeAI, Parts: []llm.ContentPart{
				llm.TextContent{Text: "calling"},
				llm.ToolCall{ID: "c1", Type: "function", FunctionCall: &llm.FunctionCall{Name: "file-read", Arguments: `{"path":"a"}`}},
			}},
			{Role: llm.ChatMessageTypeTool, Parts: []llm.ContentPart{llm.ToolCallResponse{ToolCallID: "c1", Name: "file-read", Content: "data"}}},
		},
		Turn:     4,
		MaxTurns: 50,
		TotalUsage: api.Usage{InputTokens: 10, OutputTokens: 20, CacheReadInputTokens: 3, CacheCreationInputTokens: 4,
			Model: "m", Provider: "p", Cost: &llm.Cost{InputUSD: 0.1, OutputUSD: 0.2, TotalUSD: 0.3}},
		StartedAt: time.Date(2026, 10, 4, 9, 30, 0, 123, time.UTC),
		LastResponse: &llm.ContentResponse{Choices: []*llm.ContentChoice{{
			Content: "calling", StopReason: "tool_use", ReasoningContent: "because",
			GenerationInfo: map[string]any{"input_tokens": 10.0, "model": "m"},
			FuncCall:       &llm.FunctionCall{Name: "ask", Arguments: "{}"},
			ToolCalls: []llm.ToolCall{
				{ID: "c1", Type: "function", FunctionCall: &llm.FunctionCall{Name: "file-read", Arguments: `{"path":"a"}`}},
				{ID: "c2", Type: "function", FunctionCall: &llm.FunctionCall{Name: "ask", Arguments: `{"big":9007199254740993}`}},
			},
		}}},
		LastLLMDurationMs: 1200,
		LastTTFTMs:        300,
		StructuredOutput:  map[string]any{"score": 3.0, "tags": []any{"a"}},
		Batch: &toolBatch{
			Results:    []llm.ToolCallResponse{{ToolCallID: "c1", Name: "file-read", Content: "data"}},
			APIResults: []api.ToolResult{{ToolUseID: "c1", ToolName: "file-read", Content: "data", IsError: true}},
			Complete:   &api.ErrRunComplete{Summary: "s", Files: []string{"f"}, Output: map[string]any{"k": "v"}},
			Next:       1,
			Input:      `{"big":9007199254740993}`,
			Resume:     map[string]any{"request": "q-1", "deadline": "2026-10-04T10:00:00Z"},
		},
	}

	m := &AgentLoopMachine{plugins: NewPluginSet(), hooks: &HookChain{}, loop: NewLoopContext()}
	got, err := m.ensureTyped(enginetest.ReadBack(t, machine.State{Name: "awaiting", Data: saved}))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, saved) {
		want, _ := json.MarshalIndent(saved, "", " ")
		have, _ := json.MarshalIndent(got, "", " ")
		t.Errorf("read back differs.\nsaved:\n%s\nread back:\n%s", want, have)
	}
}
