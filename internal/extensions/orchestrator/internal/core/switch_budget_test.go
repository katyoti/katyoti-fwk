// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// ,: a switch, an attach, a detach and a create wait
// for the serving member's restart (up to servingRestartWait) plus the hub's
// dial, so SwitchProject, AttachProject, DetachProject and CreateProject
// declare a call budget covering both - undeclared, the bus
// default (5 s) reported a switch that worked as refused.
func TestSwitchDeclaresABudgetCoveringTheRestart(t *testing.T) {
	want := servingRestartWait + 15*time.Second
	for _, m := range []string{"SwitchProject", "AttachProject", "DetachProject", "CreateProject"} {
		def, ok := api.OrchestratorServiceDef.Methods[m]
		if !ok {
			t.Fatalf("%s is not declared in OrchestratorServiceDef.Methods", m)
		}
		if got := time.Duration(def.TimeoutS) * time.Second; got < want {
			t.Errorf("%s declares a budget of %s; want at least %s", m, got, want)
		}
	}
}
