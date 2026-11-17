// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"encoding/json"
	"testing"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// stubDispatcher implements agent.ToolDispatcher for testing the engine container.
type stubDispatcher struct {
	tools    map[string]string // name → result
	toolDefs []agentapi.ToolDef
}

func newStubDispatcher() *stubDispatcher {
	return &stubDispatcher{
		tools: make(map[string]string),
	}
}

func (d *stubDispatcher) Dispatch(ctx context.Context, name string, input json.RawMessage) (string, error) {
	result, ok := d.tools[name]
	if !ok {
		return "", &toolNotFoundError{name: name}
	}
	return result, nil
}

func (d *stubDispatcher) ToolDefs() []agentapi.ToolDef {
	return d.toolDefs
}

type toolNotFoundError struct {
	name string
}

func (e *toolNotFoundError) Error() string {
	return "unknown tool: " + e.name
}

// --- EngineContainer tests ---

func TestEngineContainer_Info(t *testing.T) {
	d := newStubDispatcher()
	ec := NewEngineContainer(d)

	info := ec.Info()
	if info.Name != "engine" {
		t.Errorf("expected name 'engine', got %q", info.Name)
	}

	// Should have 3 slots: ktsp (built-in), agent, tool
	if len(info.Slots) != 3 {
		t.Fatalf("expected 3 slots, got %d", len(info.Slots))
	}

	slotMap := make(map[string]bool)
	for _, s := range info.Slots {
		slotMap[s.Name] = true
	}
	for _, expected := range []string{"ktsp", "agent", "tool"} {
		if !slotMap[expected] {
			t.Errorf("missing slot %q", expected)
		}
	}
}

func TestEngineContainer_AgentStatus_Idle(t *testing.T) {
	d := newStubDispatcher()
	ec := NewEngineContainer(d)

	result, err := ec.Handle(context.Background(), "agent", ktsp.DefaultInstance, "status", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var status map[string]any
	if err := json.Unmarshal(result, &status); err != nil {
		t.Fatalf("failed to parse status: %v", err)
	}
	if status["state"] != "idle" {
		t.Errorf("expected state 'idle', got %v", status["state"])
	}
}

func TestEngineContainer_AgentStatus_Running(t *testing.T) {
	d := newStubDispatcher()
	ec := NewEngineContainer(d)
	ec.SetRunning(5, 50)

	result, err := ec.Handle(context.Background(), "agent", ktsp.DefaultInstance, "status", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var status map[string]any
	if err := json.Unmarshal(result, &status); err != nil {
		t.Fatalf("failed to parse status: %v", err)
	}
	if status["state"] != "running" {
		t.Errorf("expected state 'running', got %v", status["state"])
	}
	if status["turn"].(float64) != 5 {
		t.Errorf("expected turn 5, got %v", status["turn"])
	}
}

func TestEngineContainer_AgentStop_NotRunning(t *testing.T) {
	d := newStubDispatcher()
	ec := NewEngineContainer(d)

	_, err := ec.Handle(context.Background(), "agent", ktsp.DefaultInstance, "stop", nil)
	if err == nil {
		t.Fatal("expected error when stopping idle engine")
	}
}

func TestEngineContainer_AgentStop_Running(t *testing.T) {
	d := newStubDispatcher()
	ec := NewEngineContainer(d)

	cancelled := false
	ec.SetRunning(3, 50)
	ec.SetCancelFunc(func() { cancelled = true })

	result, err := ec.Handle(context.Background(), "agent", ktsp.DefaultInstance, "stop", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp map[string]string
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["status"] != "stopping" {
		t.Errorf("expected status 'stopping', got %q", resp["status"])
	}
	if !cancelled {
		t.Error("expected cancel function to be called")
	}
}

func TestEngineContainer_ToolCall(t *testing.T) {
	d := newStubDispatcher()
	d.tools["read_file"] = `{"content":"hello"}`

	ec := NewEngineContainer(d)

	params, _ := json.Marshal(map[string]any{
		"tool":  "read_file",
		"input": map[string]string{"path": "test.txt"},
	})

	result, err := ec.Handle(context.Background(), "tool", ktsp.DefaultInstance, "call", params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["isError"].(bool) {
		t.Error("expected isError=false")
	}
	if resp["result"] != `{"content":"hello"}` {
		t.Errorf("unexpected result: %v", resp["result"])
	}
}

func TestEngineContainer_ToolCall_Error(t *testing.T) {
	d := newStubDispatcher()
	ec := NewEngineContainer(d)

	params, _ := json.Marshal(map[string]any{
		"tool":  "nonexistent",
		"input": json.RawMessage("{}"),
	})

	result, err := ec.Handle(context.Background(), "tool", ktsp.DefaultInstance, "call", params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if !resp["isError"].(bool) {
		t.Error("expected isError=true for unknown tool")
	}
}

func TestEngineContainer_ToolCall_MissingName(t *testing.T) {
	d := newStubDispatcher()
	ec := NewEngineContainer(d)

	params, _ := json.Marshal(map[string]any{
		"input": json.RawMessage("{}"),
	})

	_, err := ec.Handle(context.Background(), "tool", ktsp.DefaultInstance, "call", params)
	if err == nil {
		t.Fatal("expected error for missing tool name")
	}
}

func TestEngineContainer_ToolList(t *testing.T) {
	d := newStubDispatcher()
	d.toolDefs = []agentapi.ToolDef{
		{Name: "read_file", Description: "Read a file"},
		{Name: "write_file", Description: "Write a file"},
	}

	ec := NewEngineContainer(d)

	result, err := ec.Handle(context.Background(), "tool", ktsp.DefaultInstance, "list", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var defs []agentapi.ToolDef
	if err := json.Unmarshal(result, &defs); err != nil {
		t.Fatalf("failed to parse defs: %v", err)
	}
	if len(defs) != 2 {
		t.Errorf("expected 2 tool defs, got %d", len(defs))
	}
}

func TestEngineContainer_SetIdleWithResult(t *testing.T) {
	d := newStubDispatcher()
	ec := NewEngineContainer(d)

	ec.SetRunning(10, 50)
	ec.SetIdle(&agentapi.RunResult{
		Status:  agentapi.RunCompleted,
		Summary: "Done",
		Turns:   10,
	})

	if ec.State() != StateIdle {
		t.Errorf("expected idle, got %v", ec.State())
	}

	// Status should show last result
	result, err := ec.Handle(context.Background(), "agent", ktsp.DefaultInstance, "status", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, err := ParseStatusInfo(result)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if info.State != "idle" {
		t.Errorf("expected idle, got %q", info.State)
	}
	if info.Last == nil {
		t.Fatal("expected lastResult")
	}
	if info.Last.Status != "completed" {
		t.Errorf("expected completed, got %q", info.Last.Status)
	}
}

func TestEngineContainer_KtspSlot(t *testing.T) {
	d := newStubDispatcher()
	ec := NewEngineContainer(d)

	// ping
	result, err := ec.Handle(context.Background(), "ktsp", ktsp.DefaultInstance, "ping", nil)
	if err != nil {
		t.Fatalf("ktsp/ping error: %v", err)
	}
	var ping map[string]string
	if err := json.Unmarshal(result, &ping); err != nil {
		t.Fatalf("parse ping: %v", err)
	}
	if ping["status"] != "ok" {
		t.Errorf("expected ok, got %v", ping["status"])
	}
}

func TestContainerDescriptorSlots(t *testing.T) {
	slots := ContainerDescriptorSlots()
	if len(slots) != 2 {
		t.Fatalf("expected 2 slots, got %d", len(slots))
	}
	expected := map[string]bool{"agent": true, "tool": true}
	for _, s := range slots {
		if !expected[s] {
			t.Errorf("unexpected slot %q", s)
		}
	}
}
