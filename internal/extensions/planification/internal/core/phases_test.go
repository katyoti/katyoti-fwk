// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// THE SIX PHASES EACH HAVE THEIR WORK — every shipped template step declares
// its phase; Inputs and Release are used; the work a release's gate files for
// its change log takes the release template, in the Release phase.
func TestEveryShippedStepDeclaresItsPhase(t *testing.T) {
	recipes, err := loadRecipes()
	if err != nil {
		t.Fatal(err)
	}
	used := map[string]bool{}
	for _, r := range recipes {
		for _, st := range r.Steps {
			if api.PhaseIndex(st.Phase) < 0 {
				t.Errorf("%s/%s declares no phase (%q)", r.Name, st.Name, st.Phase)
			}
			used[st.Phase] = true
		}
	}
	for _, p := range []string{api.PhaseInputs, api.PhasePreparation, api.PhaseBuild, api.PhaseVerify, api.PhaseRelease} {
		if !used[p] {
			t.Errorf("no shipped template has a step in %s", p)
		}
	}
	r, err := selectRecipeOf(recipes, requestOn(t, api.KindMaintain, ""), api.ReleaseType)
	if err != nil || r.Name != "release-notes" || r.Steps[0].Phase != api.PhaseRelease {
		t.Fatalf("a maintain request meeting a release: %s (%v); want release-notes, in release", r.Name, err)
	}
	if r, _ := selectRecipeOf(recipes, requestOn(t, api.KindMaintain, ""), ""); r.Name != "maintain" {
		t.Errorf("a maintain request meeting nothing: %s; want maintain", r.Name)
	}
}
