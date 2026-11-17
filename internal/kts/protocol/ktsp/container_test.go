// SPDX-License-Identifier: Apache-2.0

package ktsp

import (
	"context"
	"encoding/json"
	"testing"
)

// --- Slot ---

func TestSlotRegisterAndHandle(t *testing.T) {
	s := NewSlot("test", DefaultInstance, "test slot", nil)
	s.Add(RawMethod("echo", MethodOpts{}, func(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
		return params, nil // echo back
	}))

	input, _ := json.Marshal(map[string]string{"msg": "hello"})
	result, err := s.Handle(context.Background(), "echo", input)
	if err != nil {
		t.Fatal(err)
	}

	var out map[string]string
	if err := json.Unmarshal(result, &out); err != nil {
		t.Fatal(err)
	}
	if out["msg"] != "hello" {
		t.Errorf("msg = %q, want %q", out["msg"], "hello")
	}
}

func TestSlotMethodNotFound(t *testing.T) {
	s := NewSlot("test", DefaultInstance, "test slot", nil)
	_, err := s.Handle(context.Background(), "missing", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	rpcErr, ok := err.(*RPCError)
	if !ok {
		t.Fatalf("expected *RPCError, got %T", err)
	}
	if rpcErr.Code != CodeMethodNotFound {
		t.Errorf("Code = %d, want %d", rpcErr.Code, CodeMethodNotFound)
	}
}

func TestSlotInfo(t *testing.T) {
	s := NewSlot("workspace", DefaultInstance, "File operations", nil)
	s.Add(RawMethod("read", MethodOpts{}, func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) { return nil, nil }))
	s.Add(RawMethod("write", MethodOpts{}, func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) { return nil, nil }))

	info := s.Info()
	if info.Name != "workspace" {
		t.Errorf("Name = %q, want %q", info.Name, "workspace")
	}
	if len(info.Methods) != 2 {
		t.Errorf("Methods count = %d, want 2", len(info.Methods))
	}
}

// SlotInfo.SlotType mirrors typeName after the Phase-A++ collapse —
// the categorical marker IS the wire-routing key.
func TestSlotInfoSlotType_MirrorsTypeName(t *testing.T) {
	s := NewSlot("skill", DefaultInstance, "Skill capability contract", nil)
	s.Add(RawMethod("render", MethodOpts{}, func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) { return nil, nil }))

	info := s.Info()
	if info.SlotType != SlotType("skill") {
		t.Errorf("SlotType = %q, want %q", info.SlotType, "skill")
	}
	if string(info.SlotType) != info.Name {
		t.Errorf("SlotType (%q) and Name (%q) must mirror each other", info.SlotType, info.Name)
	}
}

// --- Endpoint ---

func TestEndpointKTSPSlot(t *testing.T) {
	c := NewEndpoint("test-svc", "Test service", "1.0.0")

	// ktsp slot should exist and respond to ping
	result, err := c.Handle(context.Background(), "ktsp", DefaultInstance, "ping", nil)
	if err != nil {
		t.Fatal(err)
	}
	var ping map[string]string
	if err := json.Unmarshal(result, &ping); err != nil {
		t.Fatal(err)
	}
	if ping["status"] != "ok" {
		t.Errorf("ping status = %q, want %q", ping["status"], "ok")
	}
}

func TestEndpointSlots(t *testing.T) {
	c := NewEndpoint("file-svc", "File service", "1.0.0")

	ws := NewSlot("workspace", DefaultInstance, "File operations", nil)
	ws.Add(RawMethod("read", MethodOpts{}, func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) { return nil, nil }))
	c.AddSlot(ws)

	jrnl := NewSlot("journal", DefaultInstance, "Event journal", nil)
	jrnl.Add(RawMethod("emit", MethodOpts{}, func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) { return nil, nil }))
	c.AddSlot(jrnl)

	// List slots via ktsp.slots
	result, err := c.Handle(context.Background(), "ktsp", DefaultInstance, "slots", nil)
	if err != nil {
		t.Fatal(err)
	}
	var slots []SlotInfo
	if err := json.Unmarshal(result, &slots); err != nil {
		t.Fatal(err)
	}
	// Should have 3 slots: journal, ktsp, workspace (sorted)
	if len(slots) != 3 {
		t.Fatalf("slot count = %d, want 3", len(slots))
	}
	if slots[0].Name != "journal" {
		t.Errorf("slots[0] = %q, want %q", slots[0].Name, "journal")
	}
	if slots[1].Name != "ktsp" {
		t.Errorf("slots[1] = %q, want %q", slots[1].Name, "ktsp")
	}
	if slots[2].Name != "workspace" {
		t.Errorf("slots[2] = %q, want %q", slots[2].Name, "workspace")
	}
}

func TestEndpointDescribeSlot(t *testing.T) {
	c := NewEndpoint("test-svc", "Test", "1.0.0")

	ws := NewSlot("workspace", DefaultInstance, "File operations", nil)
	ws.Add(RawMethod("read", MethodOpts{}, func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) { return nil, nil }))
	c.AddSlot(ws)

	params, _ := json.Marshal(map[string]string{"slot": "workspace"})
	result, err := c.Handle(context.Background(), "ktsp", DefaultInstance, "describe", params)
	if err != nil {
		t.Fatal(err)
	}
	var info SlotInfo
	if err := json.Unmarshal(result, &info); err != nil {
		t.Fatal(err)
	}
	if info.Name != "workspace" {
		t.Errorf("Name = %q, want %q", info.Name, "workspace")
	}
	if len(info.Methods) != 1 {
		t.Errorf("Methods count = %d, want 1", len(info.Methods))
	}
}

func TestEndpointDescribeContainer(t *testing.T) {
	c := NewEndpoint("test-svc", "A test service", "2.0.0")

	// describe with no slot param returns container info
	result, err := c.Handle(context.Background(), "ktsp", DefaultInstance, "describe", nil)
	if err != nil {
		t.Fatal(err)
	}
	var info ContainerInfo
	if err := json.Unmarshal(result, &info); err != nil {
		t.Fatal(err)
	}
	if info.Name != "test-svc" {
		t.Errorf("Name = %q, want %q", info.Name, "test-svc")
	}
	if info.Version != "2.0.0" {
		t.Errorf("Version = %q, want %q", info.Version, "2.0.0")
	}
}

func TestEndpointSlotNotFound(t *testing.T) {
	c := NewEndpoint("test-svc", "Test", "1.0.0")
	_, err := c.Handle(context.Background(), "nonexistent", DefaultInstance, "method", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	rpcErr, ok := err.(*RPCError)
	if !ok {
		t.Fatalf("expected *RPCError, got %T", err)
	}
	if rpcErr.Code != CodeSlotNotFound {
		t.Errorf("Code = %d, want %d", rpcErr.Code, CodeSlotNotFound)
	}
}

func TestEndpointCannotOverrideKTSP(t *testing.T) {
	c := NewEndpoint("test-svc", "Test", "1.0.0")
	fake := NewSlot("ktsp", DefaultInstance, "fake ktsp", nil)
	fake.Add(RawMethod("hack", MethodOpts{}, func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		return json.Marshal("hacked")
	}))
	c.AddSlot(fake)

	// ktsp.hack should not exist — the fake slot was rejected
	_, err := c.Handle(context.Background(), "ktsp", DefaultInstance, "hack", nil)
	if err == nil {
		t.Fatal("expected error — ktsp override should be rejected")
	}
}

func TestContainerInfoSerialization(t *testing.T) {
	info := ContainerInfo{
		Name:        "doc-tools",
		Description: "Document analysis and creation",
		Version:     "1.0.0",
		Slots: []SlotInfo{
			{Name: "doc-analyze", Description: "Analyze documents"},
		},
		Tags: []string{"documents", "analysis"},
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}

	var decoded ContainerInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Name != "doc-tools" {
		t.Errorf("Name = %q, want %q", decoded.Name, "doc-tools")
	}
	if len(decoded.Slots) != 1 {
		t.Errorf("Slots count = %d, want 1", len(decoded.Slots))
	}
}

// ── Named-instance dispatch (bus-server-side-instance-dispatch) ────

// instanceEchoSlot builds a slot whose `which` method returns the
// supplied identifier — used to prove that two instances of the same
// slot type really dispatch to their own bound handlers. The slot
// itself owns its instance (Phase A rename).
func instanceEchoSlot(t *testing.T, instance, identifier string) *Slot {
	t.Helper()
	s := NewSlot("echo-svc", instance, "echo service for instance dispatch tests", nil)
	s.Add(RawMethod("which", MethodOpts{}, func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		return json.Marshal(identifier)
	}))
	return s
}

func TestEndpointInstanceDispatch_DefaultBinding(t *testing.T) {
	c := NewEndpoint("test-svc", "Test", "1.0.0")
	c.AddSlot(instanceEchoSlot(t, DefaultInstance, "default-impl"))

	// Explicit DefaultInstance dispatch.
	got, err := c.Handle(context.Background(), "echo-svc", DefaultInstance, "which", nil)
	if err != nil {
		t.Fatalf("Handle(default): %v", err)
	}
	var s string
	_ = json.Unmarshal(got, &s)
	if s != "default-impl" {
		t.Errorf("Handle(default) → %q, want %q", s, "default-impl")
	}

	// Empty instance is now rejected — the strict-explicit contract
	// surfaces missing instance as ErrInvalidRequest rather than
	// silently routing to DefaultInstance.
	_, err = c.Handle(context.Background(), "echo-svc", "", "which", nil)
	if err == nil {
		t.Fatal("expected error for empty instance")
	}
	rpcErr, ok := err.(*RPCError)
	if !ok {
		t.Fatalf("expected *RPCError, got %T", err)
	}
	if rpcErr.Code != CodeInvalidRequest {
		t.Errorf("empty-instance code = %d, want CodeInvalidRequest", rpcErr.Code)
	}
}

func TestEndpointInstanceDispatch_MultipleInstances(t *testing.T) {
	c := NewEndpoint("test-svc", "Test", "1.0.0")
	c.AddSlot(instanceEchoSlot(t, "alice", "alice"))
	c.AddSlot(instanceEchoSlot(t, "bob", "bob"))
	c.AddSlot(instanceEchoSlot(t, DefaultInstance, "default"))

	cases := []struct {
		instance string
		want     string
	}{
		{"alice", "alice"},
		{"bob", "bob"},
		{DefaultInstance, "default"},
	}
	for _, tc := range cases {
		got, err := c.Handle(context.Background(), "echo-svc", tc.instance, "which", nil)
		if err != nil {
			t.Fatalf("Handle(%q): %v", tc.instance, err)
		}
		var s string
		_ = json.Unmarshal(got, &s)
		if s != tc.want {
			t.Errorf("Handle(%q) → %q, want %q", tc.instance, s, tc.want)
		}
	}

	// Listing instances must return all three sorted.
	names := c.Instances("echo-svc")
	wantNames := []string{"alice", "bob", "default"}
	if len(names) != len(wantNames) {
		t.Fatalf("Instances len = %d, want %d (%v)", len(names), len(wantNames), names)
	}
	for i := range names {
		if names[i] != wantNames[i] {
			t.Errorf("Instances[%d] = %q, want %q", i, names[i], wantNames[i])
		}
	}
}

func TestEndpointInstanceDispatch_MissingInstance(t *testing.T) {
	c := NewEndpoint("test-svc", "Test", "1.0.0")
	c.AddSlot(instanceEchoSlot(t, "alice", "alice"))

	// Unknown instance — typed ErrSlotNotFound with `slot(instance)` form.
	_, err := c.Handle(context.Background(), "echo-svc", "carol", "which", nil)
	if err == nil {
		t.Fatal("expected error for missing instance")
	}
	rpcErr, ok := err.(*RPCError)
	if !ok {
		t.Fatalf("expected *RPCError, got %T", err)
	}
	if rpcErr.Code != CodeSlotNotFound {
		t.Errorf("Code = %d, want %d", rpcErr.Code, CodeSlotNotFound)
	}

	// Unknown slot type entirely — bare slot name in the error.
	_, err = c.Handle(context.Background(), "missing-svc", "alice", "which", nil)
	if err == nil {
		t.Fatal("expected error for missing slot type")
	}
}
