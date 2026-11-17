// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"testing"
)

// A tool call reads back whole — its function's name and arguments with it —
// alone and inside a message: a conversation saved with a run (a run waiting,
// restored after a restart) goes on with the calls it made.
func TestAToolCallReadsBackWhole(t *testing.T) {
	tc := ToolCall{ID: "call-1", Type: "function", FunctionCall: &FunctionCall{Name: "ask-user", Arguments: `{"question":"ok?"}`}}
	b, err := json.Marshal(tc)
	if err != nil {
		t.Fatal(err)
	}
	var back ToolCall
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.ID != "call-1" || back.Type != "function" || back.FunctionCall == nil ||
		back.FunctionCall.Name != "ask-user" || back.FunctionCall.Arguments != `{"question":"ok?"}` {
		t.Fatalf("read back %+v / %+v, want the call whole", back, back.FunctionCall)
	}

	msg := MessageContent{Role: ChatMessageTypeAI, Parts: []ContentPart{tc}}
	mb, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	var mback MessageContent
	if err := json.Unmarshal(mb, &mback); err != nil {
		t.Fatal(err)
	}
	got, ok := mback.Parts[0].(ToolCall)
	if !ok || got.FunctionCall == nil || got.FunctionCall.Name != "ask-user" {
		t.Fatalf("message part = %#v, want the tool call whole", mback.Parts[0])
	}

	if err := json.Unmarshal([]byte(`{"tool_call":{"id":"x","type":"function"}}`), &back); err == nil {
		t.Error("a tool call without its type reads back: want an error")
	}
}
