// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"testing"
)

func TestPermissionBehavior_String(t *testing.T) {
	cases := []struct {
		in   PermissionBehavior
		want string
	}{
		{PermissionAllow, "allow"},
		{PermissionAsk, "ask"},
		{PermissionDeny, "deny"},
		{PermissionPassthrough, "passthrough"},
		{PermissionBehavior(99), "unknown"},
	}
	for _, c := range cases {
		if got := c.in.String(); got != c.want {
			t.Errorf("PermissionBehavior(%d).String() = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPermissionBehavior_ZeroValueIsAskFailClosed(t *testing.T) {
	// Ask is intentionally the zero value (fail-closed per spec FR-01).
	// A tool that returns PermissionResult{} without setting Behavior must
	// prompt the user, never auto-allow.
	var p PermissionBehavior
	if p != PermissionAsk {
		t.Fatalf("zero PermissionBehavior should equal PermissionAsk (fail-closed), got %v", p)
	}
}

func TestPermissionResult_ZeroValueIsAskFailClosed(t *testing.T) {
	r := PermissionResult{}
	if r.Behavior != PermissionAsk {
		t.Fatalf("zero PermissionResult.Behavior should be PermissionAsk (fail-closed), got %v", r.Behavior)
	}
	if r.Message != "" {
		t.Fatalf("zero PermissionResult.Message should be empty, got %q", r.Message)
	}
	if r.ModifiedInput != nil {
		t.Fatalf("zero PermissionResult.ModifiedInput should be nil, got %v", r.ModifiedInput)
	}
}

func TestPermissionResult_ModifiedInputRoundtrip(t *testing.T) {
	raw := json.RawMessage(`{"path":"/etc/hosts"}`)
	r := PermissionResult{Behavior: PermissionAllow, ModifiedInput: raw}
	if string(r.ModifiedInput) != string(raw) {
		t.Fatalf("ModifiedInput roundtrip failed: got %s, want %s", r.ModifiedInput, raw)
	}
}

func TestInterruptBehavior_String(t *testing.T) {
	cases := []struct {
		in   InterruptBehavior
		want string
	}{
		{InterruptBlock, "block"},
		{InterruptCancel, "cancel"},
		{InterruptBehavior(99), "unknown"},
	}
	for _, c := range cases {
		if got := c.in.String(); got != c.want {
			t.Errorf("InterruptBehavior(%d).String() = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestInterruptBehavior_DefaultIsBlock(t *testing.T) {
	// Block is intentionally the zero value — long-running tools must opt
	// into Cancel via WithCancelOnInterrupt().
	var i InterruptBehavior
	if i != InterruptBlock {
		t.Fatalf("zero InterruptBehavior should equal InterruptBlock, got %v", i)
	}
}

// ── DefaultDimensionExtractor ──────────────────────────────────────────

func TestDefaultDimensionExtractor_FlatStringFields(t *testing.T) {
	in := json.RawMessage(`{"workspace_path":"src/main.go","absolute_path":"/etc/hosts"}`)
	got := DefaultDimensionExtractor(in)
	if got["workspace_path"] != "src/main.go" || got["absolute_path"] != "/etc/hosts" {
		t.Errorf("got %v, want both string fields", got)
	}
}

func TestDefaultDimensionExtractor_SkipsNonStringFields(t *testing.T) {
	in := json.RawMessage(`{"path":"foo","offset":10,"is_partial":true,"nested":{"a":1},"items":[1,2]}`)
	got := DefaultDimensionExtractor(in)
	if got["path"] != "foo" {
		t.Errorf("path missing or wrong: %v", got)
	}
	for _, key := range []string{"offset", "is_partial", "nested", "items"} {
		if _, has := got[key]; has {
			t.Errorf("non-string key %q should have been skipped, got %v", key, got)
		}
	}
}

func TestDefaultDimensionExtractor_EmptyInput(t *testing.T) {
	got := DefaultDimensionExtractor(nil)
	if len(got) != 0 {
		t.Errorf("nil input should yield empty map, got %v", got)
	}
	got = DefaultDimensionExtractor(json.RawMessage(""))
	if len(got) != 0 {
		t.Errorf("empty bytes should yield empty map, got %v", got)
	}
}

func TestDefaultDimensionExtractor_MalformedJSON_FailSoft(t *testing.T) {
	got := DefaultDimensionExtractor(json.RawMessage(`{not json`))
	if len(got) != 0 {
		t.Errorf("malformed JSON should yield empty map (no error), got %v", got)
	}
}

func TestDefaultDimensionExtractor_EmptyObject(t *testing.T) {
	got := DefaultDimensionExtractor(json.RawMessage(`{}`))
	if len(got) != 0 {
		t.Errorf("empty object should yield empty map, got %v", got)
	}
}
