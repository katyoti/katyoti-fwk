// SPDX-License-Identifier: Apache-2.0

package terminal

import (
	"encoding/json"
	"testing"
)

func TestMessageContentRoundTrip(t *testing.T) {
	// Create a tool content
	tool := ToolContent{
		Type:     "tool",
		ToolName: "file_read",
		Status:   "success",
		Input:    map[string]any{"workspace_path": "types.go"},
		Output:   map[string]any{"lines": 120},
		Duration: 45,
		Display:  &DisplayData{Icon: "📄", Label: "Read", Title: "types.go"},
	}
	content := NewContent(tool)

	// Wrap in a Message
	msg := Message{
		ID:        "m1",
		SessionID: "s1",
		SeqNo:     1,
		Role:      "tool",
		Timestamp: "2026-04-13T10:00:00Z",
		Content:   content,
	}

	// Marshal
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Unmarshal
	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Content.Type() != "tool" {
		t.Errorf("content type = %q, want tool", decoded.Content.Type())
	}

	// Decode into concrete type
	var tc ToolContent
	if err := decoded.Content.As(&tc); err != nil {
		t.Fatalf("as ToolContent: %v", err)
	}
	if tc.ToolName != "file_read" {
		t.Errorf("toolName = %q, want file_read", tc.ToolName)
	}
	if tc.Display == nil || tc.Display.Icon != "📄" {
		t.Errorf("display.icon = %v, want 📄", tc.Display)
	}
}

func TestTextContentShortcut(t *testing.T) {
	content := Text("Hello **world**")
	data, _ := json.Marshal(content)

	var decoded MessageContent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Type() != "text" {
		t.Errorf("type = %q, want text", decoded.Type())
	}
	var tc TextContent
	decoded.As(&tc)
	if tc.Text != "Hello **world**" {
		t.Errorf("text = %q, want Hello **world**", tc.Text)
	}
}

func TestMsgNewEventRoundTrip(t *testing.T) {
	evt := MsgNewEvent{
		Type:    "msg.new",
		Session: "s1",
		SeqNo:   5,
		Msg: Message{
			ID:        "m5",
			SessionID: "s1",
			SeqNo:     5,
			Role:      "assistant",
			Timestamp: "2026-04-13T10:05:00Z",
			Content:   Text("Analyzing the codebase..."),
		},
	}

	data, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded MsgNewEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Msg.Content.Type() != "text" {
		t.Errorf("content type = %q, want text", decoded.Msg.Content.Type())
	}
}

func TestSessionModel(t *testing.T) {
	root := Session{
		ID:     "s1",
		Name:   "spec-extract",
		Status: "running",
		TaskID: "t1",
	}
	child := Session{
		ID:              "s2",
		ParentSessionID: "s1",
		Name:            "background-delegation",
		Status:          "running",
		TaskID:          "t2",
		ParentTaskID:    "t1",
	}

	if root.ParentSessionID != "" {
		t.Error("root session should have empty parentSessionId")
	}
	if child.ParentSessionID != "s1" {
		t.Errorf("child parentSessionId = %q, want s1", child.ParentSessionID)
	}
	if child.ParentTaskID != "t1" {
		t.Errorf("child parentTaskId = %q, want t1", child.ParentTaskID)
	}
}
