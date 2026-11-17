// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A planner's copy read before a person forced a move never writes its stale
// state back: the stored lifecycle and gate wait win, the copy's own fields
// stay.
func TestAStaleCopyKeepsTheStoredMove(t *testing.T) {
	stale := cwmapi.Object{ID: "billing.F-001", Name: "renamed by the planner", Capabilities: map[string]map[string]any{
		cwmapi.CapLifecycle: {"state": "active"},
		api.CapGateWait:     {"gate": "ready-to-test", "to": "done"},
		"planned":           {"note": "the planner's"},
	}}
	stored := cwmapi.Object{ID: "billing.F-001", Capabilities: map[string]map[string]any{
		cwmapi.CapLifecycle: {"state": "done"},
	}}
	got := withStoredMoves(stale, stored)
	if st := got.Capabilities[cwmapi.CapLifecycle]["state"]; st != "done" {
		t.Errorf("state = %v; want done (the stored move)", st)
	}
	if _, ok := got.Capabilities[api.CapGateWait]; ok {
		t.Error("the ended gate wait came back with the stale copy")
	}
	if got.Name != "renamed by the planner" || got.Capabilities["planned"]["note"] != "the planner's" {
		t.Errorf("the planner's own change was lost: %+v", got)
	}
	if st := stale.Capabilities[cwmapi.CapLifecycle]["state"]; st != "active" {
		t.Errorf("the caller's copy was changed: %v", st)
	}
}
