// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	gatesapi "github.com/katyoti/katyoti-fwk/internal/extensions/gates/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	verificationapi "github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
)

// Each template that changes a module brings it up to date: build after its
// implementation and before the person's review, fix before its verification,
// feature-build in place of its documentation; the quick ones and maintain
// fold it into their one step. The others change no module.
func TestTheTemplatesThatChangeAModuleUpdateIt(t *testing.T) {
	recipes, err := loadRecipes()
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]api.Recipe{}
	for _, r := range recipes {
		byName[r.Name] = r
	}
	order := map[string][]string{
		"build":         {"analyse", "implement", "auto-review", StepModuleUpdate, "review"},
		"fix":           {"implement", "auto-review", StepModuleUpdate, "verify"},
		"feature-build": {"implement", StepModuleUpdate, "verify"},
	}
	for name, want := range order {
		r := byName[name]
		var got []string
		for _, st := range r.Steps {
			got = append(got, st.Name)
			if st.Name != StepModuleUpdate {
				continue
			}
			if st.Skill != ModuleUpdateSkill || st.Gate != GateModuleUpdated || !slices.Contains(st.After, "implement") && !slices.Contains(st.After, "auto-review") {
				t.Errorf("%s's module-update: skill %q, gate %q, after %v", name, st.Skill, st.Gate, st.After)
			}
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s: steps %v; want %v", name, got, want)
		}
		if i := slices.Index(got, StepModuleUpdate); i >= 0 && i+1 < len(r.Steps) && !slices.Contains(r.Steps[i+1].After, StepModuleUpdate) {
			t.Errorf("%s: %s comes after %v; want after the module update", name, r.Steps[i+1].Name, r.Steps[i+1].After)
		}
		if !changesModule(r) {
			t.Errorf("%s changes no module", name)
		}
	}
	for _, s := range byName["build"].Steps {
		if s.Name == "review" && !slices.Contains(s.After, StepModuleUpdate) {
			t.Errorf("build's review comes after %v; want after the module update", s.After)
		}
	}
	for _, name := range []string{"build-quick", "fix-quick", "maintain"} {
		r := byName[name]
		// The quick templates fold the module update into one step; their
		// change is reviewed automatically (auto-review) after it.
		if n := len(r.Steps); n != 1 && !(name != "maintain" && n == 2 && r.Steps[1].Name == "auto-review") && name != "build-quick" {
			t.Errorf("%s: %d steps", name, len(r.Steps))
		}
		if !strings.Contains(r.Steps[0].Instructions, ModuleUpdateSkill) || !changesModule(r) {
			t.Errorf("%s does not fold the module update into its step: %q", name, r.Steps[0].Instructions)
		}
	}
	for _, name := range []string{"analyse", "review", "decide", "question", "discover", "feature-specify", "goal-decompose"} {
		if changesModule(byName[name]) {
			t.Errorf("%s is held to the module update", name)
		}
	}
}

// A new red is a check failing now that did not fail before the run; one that
// already failed is not the request's.
func TestANewRedIsOneThatDidNotFailBefore(t *testing.T) {
	before := map[string]string{"m.AC-1": "pass", "m.AC-2": "fail"}
	out := verificationapi.VerifyOutput{Outcomes: []verificationapi.VerifyOutcome{
		{ObjectID: "m.AC-1", Status: "fail"}, // was green
		{ObjectID: "m.AC-2", Status: "fail"}, // already red
		{ObjectID: "m.AC-3", Status: "fail"}, // never run
		{ObjectID: "m.AC-4", Status: "pass"},
	}}
	if got := newReds(before, out); !slices.Equal(got, []string{"m.AC-1", "m.AC-3"}) {
		t.Errorf("new reds %v; want m.AC-1 and m.AC-3", got)
	}
}

// The end of a request holds its lock while it finishes the plan; a bus that
// delivered the module-updated gate's refusal synchronously would call
// GateRefused under that lock. A refusal of a capability's own workflow is
// left to its owner before the lock is taken: no deadlock.
func TestARefusedPlanFinishInsideAnEndDoesNotDeadlock(t *testing.T) {
	const id = "billing.RQ-001"
	unlock := lockRequest(id) // what endRequest holds
	defer unlock()
	p := cwmapi.ObjectTransitionRefusedEventPayload{ObjectID: id, Capability: api.CapPlan, Transition: "finish", To: api.PlanDone,
		Calls: []cwmapi.FunctionAnswer{{Function: gatesapi.FunctionName, Args: []any{GateModuleUpdated},
			Answer: gatesapi.GateChecklist{Gate: GateModuleUpdated, Items: []gatesapi.GateItem{{Name: "module-up-to-date", Status: gatesapi.Unmet}}}}}}
	if _, ok := gatesapi.FailingChecklist(p.Calls); !ok {
		t.Fatal("the payload carries no failing checklist: the test proves nothing")
	}
	done := make(chan error, 1)
	go func() { done <- New().GateRefused(context.Background(), p) }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("GateRefused: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("GateRefused waited on the lock its end holds: a deadlock")
	}
}
