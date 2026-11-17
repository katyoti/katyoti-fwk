// SPDX-License-Identifier: Apache-2.0

// Package enginetest is what a machine's tests use to check the state it saves:
// ReadBack gives a state as a restart reads it.
package enginetest

import (
	"encoding/json"
	"testing"

	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// ReadBack is st saved in a checkpoint and read back, as the task service's
// run store does across a restart (encoding/json both ways: State.Data comes
// back as JSON values — maps, []any, float64 — for the machine to type again).
// A test in the task service holds the two together.
func ReadBack(t testing.TB, st engine.State) *engine.State {
	t.Helper()
	b, err := json.Marshal(engine.Checkpoint{State: st})
	if err != nil {
		t.Fatalf("save state %s: %v", st.Name, err)
	}
	var cp engine.Checkpoint
	if err := json.Unmarshal(b, &cp); err != nil {
		t.Fatalf("read back state %s: %v", st.Name, err)
	}
	return &cp.State
}
