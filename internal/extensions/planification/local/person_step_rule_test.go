// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"regexp"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// ONE PERSON STEP, ONE ACTION: every person step of every
// shipped recipe asks exactly one action of the closed set — its one
// interaction maps to it: validate and challenge → validate, decide → decide,
// provide → answer — and none folds a test by hand into a judgement (a hand
// test is a deliverable's own item in « À faire », before the validation).
func TestEveryPersonStepAsksOneAction(t *testing.T) {
	t.Parallel()
	ctx, _, _ := world(t)
	got, err := plannings(t, ctx).ListRecipes(ctx, api.ListAllInput{})
	if err != nil {
		t.Fatal(err)
	}
	action := map[string]string{
		api.InteractionValidate: api.ActionValidate, api.InteractionChallenge: api.ActionValidate,
		api.InteractionDecide: api.ActionDecide, api.InteractionProvide: api.ActionAnswer,
	}
	handTest := regexp.MustCompile(`(?i)\btest(ed|ing)?\b[^.;]*\bby hand\b|\bmanual(ly)?\s+test|\bhand[- ]test`)
	people := 0
	for _, r := range got.Recipes {
		for _, s := range r.Steps {
			if s.Actor != api.DoerPerson {
				continue
			}
			people++
			if action[s.Interaction] == "" {
				t.Errorf("%s/%s: a person's step asking %q — not one action of the closed set", r.Name, s.Name, s.Interaction)
			}
			if handTest.MatchString(s.Instructions) {
				t.Errorf("%s/%s asks a test by hand inside its %s: %q", r.Name, s.Name, s.Interaction, s.Instructions)
			}
		}
	}
	if people == 0 {
		t.Fatal("no recipe has a person's step: nothing was checked")
	}
}
