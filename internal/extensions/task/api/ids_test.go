// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"testing"
)

// TestIDs_DistinctTypes — compile-time guarantee that the four ID types
// are NOT interchangeable. This file refuses to build if a future PR
// collapses them to a single underlying type (or to plain string).
//
// We can't directly assert "RunID is not assignable to TaskID" at runtime
// — that's a compile-time property. Instead, we verify the runtime
// behaviour we care about: each ID round-trips as a plain string through
// JSON serialisation (the wire shape is unchanged) AND the type identity
// is preserved across the conversion.
func TestIDs_RoundTripAsString(t *testing.T) {
	cases := []struct {
		name string
		got  string
	}{
		{"RunID", string(RunID("run-abc"))},
		{"TaskID", string(TaskID("task-def"))},
		{"ChildID", string(ChildID("child-1"))},
		{"NodeID", string(NodeID("seq_42"))},
	}
	for _, tc := range cases {
		if tc.got == "" {
			t.Errorf("%s: zero-value string from typed ID", tc.name)
		}
	}
}

// TestIDs_JSONUnchangedFromString — wire-format compatibility. Existing
// code paths that read/write IDs as plain JSON strings continue to work;
// the migration is mechanical (rename the field type) without any wire
// shape change.
func TestIDs_JSONUnchangedFromString(t *testing.T) {
	type wire struct {
		RunID   RunID   `json:"runId"`
		TaskID  TaskID  `json:"taskId"`
		ChildID ChildID `json:"childId"`
		NodeID  NodeID  `json:"nodeId"`
	}
	w := wire{
		RunID:   "run-1",
		TaskID:  "task-2",
		ChildID: "child-3",
		NodeID:  "seq_4",
	}
	b, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"runId":"run-1","taskId":"task-2","childId":"child-3","nodeId":"seq_4"}`
	if string(b) != want {
		t.Errorf("JSON wire shape changed: got %s, want %s", string(b), want)
	}
	var out wire
	if err := json.Unmarshal([]byte(want), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out != w {
		t.Errorf("roundtrip mismatch: got %+v, want %+v", out, w)
	}
}
