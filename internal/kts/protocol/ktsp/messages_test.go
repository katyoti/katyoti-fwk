// SPDX-License-Identifier: Apache-2.0

package ktsp

import (
	"encoding/json"
	"testing"
)

func TestNewRequest(t *testing.T) {
	req, err := NewRequest(1, "workspace/read", map[string]string{"path": "test.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if req.Slot != "workspace" {
		t.Errorf("Slot = %q, want %q", req.Slot, "workspace")
	}
	if req.Method != "read" {
		t.Errorf("Method = %q, want %q", req.Method, "read")
	}
	if req.ID != 1 {
		t.Errorf("ID = %v, want 1", req.ID)
	}
}

func TestNewRequestNilParams(t *testing.T) {
	req, err := NewRequest("abc", "ktsp/ping", nil)
	if err != nil {
		t.Fatal(err)
	}
	if req.Params != nil {
		t.Errorf("Params = %v, want nil", req.Params)
	}
}

func TestRequestParseParams(t *testing.T) {
	req, _ := NewRequest(1, "workspace/read", map[string]string{"path": "test.txt"})
	var p struct {
		Path string `json:"path"`
	}
	if err := req.ParseParams(&p); err != nil {
		t.Fatal(err)
	}
	if p.Path != "test.txt" {
		t.Errorf("Path = %q, want %q", p.Path, "test.txt")
	}
}

func TestRequestFields(t *testing.T) {
	req, _ := NewRequest(42, "doc-analyze/prose", map[string]string{"text": "hello"})
	if req.Slot != "doc-analyze" {
		t.Errorf("Slot = %q, want %q", req.Slot, "doc-analyze")
	}
	if req.Method != "prose" {
		t.Errorf("Method = %q, want %q", req.Method, "prose")
	}
	if req.Path() != "doc-analyze/prose" {
		t.Errorf("Path() = %q, want %q", req.Path(), "doc-analyze/prose")
	}
}

func TestRequestThreeLevelPath(t *testing.T) {
	req, _ := NewRequest(1, "sm/session/create", nil)
	if req.ContainerID != "sm" {
		t.Errorf("ContainerID = %q, want %q", req.ContainerID, "sm")
	}
	if req.Slot != "session" {
		t.Errorf("Slot = %q, want %q", req.Slot, "session")
	}
	if req.Method != "create" {
		t.Errorf("Method = %q, want %q", req.Method, "create")
	}
}

func TestResponseSerializationRoundtrip(t *testing.T) {
	resp, _ := NewResponse(1, map[string]string{"status": "ok"})
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}

	var decoded Response
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}

	var result map[string]string
	if err := decoded.ParseResult(&result); err != nil {
		t.Fatal(err)
	}
	if result["status"] != "ok" {
		t.Errorf("status = %q, want %q", result["status"], "ok")
	}
}

func TestErrorResponse(t *testing.T) {
	resp := NewErrorResponse(1, ErrSlotNotFound("missing"))
	if resp.Error == nil {
		t.Fatal("Error should not be nil")
	}
	if resp.Error.Code != CodeSlotNotFound {
		t.Errorf("Code = %d, want %d", resp.Error.Code, CodeSlotNotFound)
	}
	if resp.Result != nil {
		t.Error("Result should be nil on error response")
	}
}

func TestNotification(t *testing.T) {
	notif, err := NewNotification("journal/emit", map[string]string{"type": "tool_call"})
	if err != nil {
		t.Fatal(err)
	}
	if notif.Path() != "journal/emit" {
		t.Errorf("Path() = %q, want %q", notif.Path(), "journal/emit")
	}
	if notif.Slot != "journal" {
		t.Errorf("Slot = %q, want %q", notif.Slot, "journal")
	}
}

func TestErrorCodes(t *testing.T) {
	tests := []struct {
		err  *RPCError
		code int
	}{
		{ErrSlotNotFound("x"), CodeSlotNotFound},
		{ErrToolNotFound("x"), CodeToolNotFound},
		{ErrTaskNotFound("x"), CodeTaskNotFound},
		{ErrResourceNotFound("x"), CodeResourceNotFound},
		{ErrContainerBusy(), CodeContainerBusy},
		{ErrMethodNotFound("x"), CodeMethodNotFound},
		{ErrInternalError("x"), CodeInternalError},
	}
	for _, tt := range tests {
		if !IsCode(tt.err, tt.code) {
			t.Errorf("IsCode(%v, %d) = false, want true", tt.err, tt.code)
		}
	}
}

func TestRPCErrorImplementsError(t *testing.T) {
	err := ErrSlotNotFound("workspace")
	var e error = err // compile-time check
	if e.Error() == "" {
		t.Error("Error() should return non-empty string")
	}
}

func TestTaskStateTerminal(t *testing.T) {
	if TaskRunning.IsTerminal() {
		t.Error("running should not be terminal")
	}
	if !TaskCompleted.IsTerminal() {
		t.Error("completed should be terminal")
	}
	if !TaskFailed.IsTerminal() {
		t.Error("failed should be terminal")
	}
	if !TaskCancelled.IsTerminal() {
		t.Error("cancelled should be terminal")
	}
}
