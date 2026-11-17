// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// The shipped recipes load, one per kind at least; a malformed recipe is
// refused with the reason.
func TestRecipes(t *testing.T) {
	recipes, err := loadRecipes()
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, r := range recipes {
		kinds[r.Kind] = true
	}
	for _, k := range api.Kinds {
		if !kinds[k] {
			t.Errorf("no recipe for %s", k)
		}
	}
	bad := map[string]api.Recipe{
		"unknown kind":          {Name: "x", Kind: "wish", Steps: []api.RecipeStep{{Name: "a"}}},
		"no steps":              {Name: "x", Kind: api.KindFix},
		"waits on a later":      {Name: "x", Kind: api.KindFix, Steps: []api.RecipeStep{{Name: "a", After: []string{"b"}}, {Name: "b"}}},
		"waits on no step":      {Name: "x", Kind: api.KindFix, Steps: []api.RecipeStep{{Name: "a", After: []string{"z"}}}},
		"two steps one name":    {Name: "x", Kind: api.KindFix, Steps: []api.RecipeStep{{Name: "a"}, {Name: "a"}}},
		"shares over 1":         {Name: "x", Kind: api.KindFix, Steps: []api.RecipeStep{{Name: "a", Share: 0.7}, {Name: "b", Share: 0.7}}},
		"person asks nothing":   {Name: "x", Kind: api.KindFix, Steps: []api.RecipeStep{{Name: "a", Actor: api.ActorPerson}}},
		"agent interacts":       {Name: "x", Kind: api.KindFix, Steps: []api.RecipeStep{{Name: "a", Interaction: api.InteractionValidate}}},
		"agent decides":         {Name: "x", Kind: api.KindFix, Steps: []api.RecipeStep{{Name: "a", Role: "reviewer", Interaction: api.InteractionDecide}}},
		"agent provides":        {Name: "x", Kind: api.KindFix, Steps: []api.RecipeStep{{Name: "a", Role: "reviewer", Interaction: api.InteractionProvide}}},
		"asked agent decides":   {Name: "x", Kind: api.KindFix, Steps: []api.RecipeStep{{Name: "a", Role: "reviewer", PersonWhenAsked: true, Interaction: api.InteractionDecide}}},
		"unknown class":         {Name: "x", Kind: api.KindFix, Steps: []api.RecipeStep{{Name: "a", Class: "polish"}}},
		"neither type nor kind": {Name: "x", Steps: []api.RecipeStep{{Name: "a"}}},
		"intake is reserved":    {Name: "x", Kind: api.KindFix, Steps: []api.RecipeStep{{Name: api.StepIntake}}},
		"unknown isolation":     {Name: "x", Kind: api.KindFix, Steps: []api.RecipeStep{{Name: "a", Isolation: "worktree"}}},
	}
	for name, r := range bad {
		if err := checkRecipe(r); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// The recipe is chosen by type and state first — the stage to reach — then by
// the request's kind.
func TestRecipeFor(t *testing.T) {
	obj := func(typ, state, kind string) cwmapi.Object {
		o := cwmapi.Object{ID: "m.x", Type: typ}
		if typ == api.RequestType { // a request object has no lifecycle: its kind chooses
			_ = o.SetCapability(api.CapRequest, api.RequestFields{Kind: kind})
			return o
		}
		if state != "" {
			o.SetLifecycle(cwmapi.Lifecycle{State: state})
		}
		if kind != "" {
			_ = o.SetCapability(api.CapRequest, api.RequestFields{Kind: kind})
		}
		return o
	}
	cases := []struct {
		o    cwmapi.Object
		want string
	}{
		{obj("goal", "accepted", ""), "goal-decompose"},
		{obj("goal", "", api.KindBuild), "goal-follow"},
		{obj("goal", "accepted", api.KindBuild), "goal-decompose"}, // its rung before goal-follow // a goal a delta added: its features build it
		{obj("feature", "accepted", ""), "feature-specify"},
		{obj("feature", "ready", ""), "feature-build"},
		{obj("feature", "accepted", api.KindFix), "feature-specify"}, // its rung before its kind
		{obj(api.RequestType, "new", api.KindDecide), "decide"},
		{obj(api.RequestType, "", ""), "build"},
	}
	for _, c := range cases {
		r, err := recipeFor(c.o)
		if err != nil || r.Name != c.want {
			t.Errorf("%s in %s: %s, %v; want %s", c.o.Type, lifeState(c.o), r.Name, err, c.want)
		}
	}
	if _, err := recipeFor(obj("feature", "specified", "")); err == nil {
		t.Error("a specified feature (its work is a person's: make it ready) got a recipe")
	}
}
