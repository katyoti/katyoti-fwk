// SPDX-License-Identifier: Apache-2.0

package ledger

import (
	"reflect"
	"testing"

	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine/enginetest"
)

// A ledger's saved state reads back whole after a restart: its snapshot, every
// nested value, and its version. Its numbers are JSON's (float64), as every
// patch it merged made them — the one difference: an int of its initial
// snapshot reads back a float64, the same number to whoever reads the ledger.
func TestALedgersStateReadsBackWhole(t *testing.T) {
	saved := &ledgerState{Version: 12, Snapshot: map[string]any{
		"title": "plan", "done": false, "count": 3.0, "none": nil,
		"steps": []any{map[string]any{"id": "s1", "ok": true}, "free"},
		"meta":  map[string]any{"owner": map[string]any{"name": "a"}, "empty": map[string]any{}},
	}}
	got := ensureState(enginetest.ReadBack(t, engine.State{Name: "listening", Data: saved}), map[string]any{"initial": true})
	if !reflect.DeepEqual(got, saved) {
		t.Errorf("read back %#v\nwant %#v", got, saved)
	}
}
