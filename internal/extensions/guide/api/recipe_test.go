// SPDX-License-Identifier: Apache-2.0

package guideapi

import "testing"

func TestParseRecipeReadsStepsAndTellsATourFromARecipe(t *testing.T) {
	raw := []byte(`
name: Add a capability to my universe
hashtags: [marketplace]
steps:
  - id: open
    say: Open the Marketplace.
    target: {guide: ribbon.marketplace}
    await: click
  - id: done
    say: Done.
    check: ktw-univers:guide-checks/sidecar-wired
`)
	r, err := ParseRecipe(raw)
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "Add a capability to my universe" || len(r.Steps) != 2 {
		t.Fatalf("parsed %+v", r)
	}
	if r.Steps[0].Target == nil || r.Steps[0].Target.Guide != "ribbon.marketplace" || r.Steps[0].ID != "open" {
		t.Fatalf("target = %+v", r.Steps[0].Target)
	}
	if r.IsTour() {
		t.Error("a recipe with a check is not a tour")
	}
	if got := r.Checks(); len(got) != 1 || got[0] != "ktw-univers:guide-checks/sidecar-wired" {
		t.Errorf("checks = %v", got)
	}
	if r.Steps[1].EffectiveAwait() != AwaitManual {
		t.Errorf("a step with no await is manual, got %q", r.Steps[1].EffectiveAwait())
	}

	r.Steps[1].Check = ""
	if !r.IsTour() {
		t.Error("a recipe with no check is a tour")
	}
}

func TestParseRecipeRefusesBrokenYAML(t *testing.T) {
	if _, err := ParseRecipe([]byte("steps: [")); err == nil {
		t.Fatal("broken YAML parsed")
	}
}

// Text picks a language string by string and falls back to the English.
func TestRecipeTextFallsBackPerString(t *testing.T) {
	r := Recipe{Name: "Add", Description: "Adds.", Steps: []Step{{ID: "a", Say: "First."}, {ID: "b", Say: "Second."}}}
	tr := Translations{"fr": {Name: "Ajouter", Steps: map[string]string{"a": "Premier.", "zz": "ignored"}}}
	fr := r.Text(tr, "fr")
	if fr.Name != "Ajouter" || fr.Description != "Adds." || fr.Steps["a"] != "Premier." || fr.Steps["b"] != "Second." || len(fr.Steps) != 2 {
		t.Errorf("fr = %+v", fr)
	}
	if en := r.Text(tr, "en"); en.Name != "Add" || en.Steps["a"] != "First." {
		t.Errorf("en = %+v", en)
	}
}

// A step names a control {control:<anchor>} or {control:<anchor>|<name>};
// anything else opening `{control` is counted malformed.
func TestControlRefs(t *testing.T) {
	refs, bad := ControlRefs("Open {control:objtab.panel:actions}, then {control:page.action|ktw-demo:signoff/approve} and {control:Bad One}.")
	if bad != 1 || len(refs) != 2 {
		t.Fatalf("refs %v, malformed %d", refs, bad)
	}
	if refs[0] != (ControlRef{Anchor: "objtab.panel:actions"}) || refs[1] != (ControlRef{Anchor: "page.action", Name: "ktw-demo:signoff/approve"}) {
		t.Fatalf("refs %v", refs)
	}
	if refs, bad := ControlRefs("No control here, only *a term*."); len(refs) != 0 || bad != 0 {
		t.Fatalf("plain text: %v %d", refs, bad)
	}
}
