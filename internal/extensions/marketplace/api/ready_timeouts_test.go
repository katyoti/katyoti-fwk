// SPDX-License-Identifier: Apache-2.0

package api

import "testing"

// The Ready screen's reads - the doctor, the programs' plan and the key sets
// (and their import, which reads the keys again) - start each wired sidecar's
// config check, which on a fresh machine takes far more than the transport's
// few seconds: each declares its request timeout, which every caller (the GUI
// included) applies.
func TestTheReadyScreensReadsDeclareTheirTimeout(t *testing.T) {
	for _, name := range []string{"Doctor", "PlanPrereqs", "KeySets", "ImportKeySets"} {
		m, ok := SetupServiceDef.Methods[name]
		if !ok {
			t.Fatalf("no method %s", name)
		}
		if m.TimeoutS < 120 {
			t.Errorf("%s declares a %ds timeout; a cold machine needs minutes", name, m.TimeoutS)
		}
	}
}
