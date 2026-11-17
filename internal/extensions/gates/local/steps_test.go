// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gates/api"
	gatescontrib "github.com/katyoti/katyoti-fwk/internal/extensions/gates/contrib"
	gateslocal "github.com/katyoti/katyoti-fwk/internal/extensions/gates/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// THE STEPS A GATE'S REQUIREMENTS ADD. ready-to-test declares two steps of
// its own at the same priority and one requirement without a step; a feature
// adds a security step first, a story adds one only stories get. ready-to-plan
// has no requirement at all: types opt in. broken holds a misspelt step.
var stepsPack = fstest.MapFS{
	"capabilities/container.yaml": {Data: []byte("name: container\n")},
	"object-types/module.yaml":    {Data: []byte("name: module\ncapabilities: [container]\n")},
	"object-types/feature.yaml":   {Data: []byte("name: feature\ncapabilities: [container]\n")},
	"object-types/story.yaml":     {Data: []byte("name: story\ncapabilities: [container]\n")},
	"gates/ready-to-test.yaml": {Data: []byte(`name: ready-to-test
requirements:
  - {name: zeta, check: "true", resolve: {step: {name: review, interaction: validate, priority: 2}}}
  - {name: alpha, check: "true", resolve: {step: {name: test, role: tester, skill: k-test, priority: 2}}}
  - {name: no-step, check: "true", resolve: {title: "Plan {object}"}}
`)},
	"gates/ready-to-plan.yaml": {Data: []byte("name: ready-to-plan\n")},
	"gates/broken.yaml": {Data: []byte(`name: broken
requirements:
  - {name: bad, check: "true", resolve: {step: {name: x, rol: tester}}}
`)},
	"gate-requirements/secure.yaml":  {Data: []byte("name: secure\ngate: ready-to-test\napplies: {types: [feature]}\ncheck: \"true\"\nresolve: {step: {name: threat-model, role: security, priority: 1}}\n")},
	"gate-requirements/stories.yaml": {Data: []byte("name: stories\ngate: ready-to-test\napplies: {types: [story]}\ncheck: \"true\"\nresolve: {step: {name: story-only, role: tester}}\n")},
}

func stepsWorld(t *testing.T) (context.Context, api.GatesService) {
	t.Helper()
	ctx, cwm := cwmapp.New(t,
		cwmapp.Pack(stepsPack),
		gatescontrib.Pack(""),
		gateslocal.Extension(),
		application.DeclareSimpleService[eventbus.EventBus](func(context.Context) (eventbus.EventBus, error) {
			return eventbus.NewMemoryEventBus(), nil
		}),
	)
	cwmapp.Module(t, ctx, cwm, "m")
	for _, o := range []cwmapi.Object{{ID: "m.f1", Type: "feature"}, {ID: "m.s1", Type: "story"}} {
		o.Name, o.Parent = o.ID, "m"
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}
	g, err := application.UseService[api.GatesService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, g
}

func stepNames(steps []api.AddedStep) string {
	var out []string
	for _, s := range steps {
		out = append(out, s.Requirement+":"+s.Step.Name)
	}
	return strings.Join(out, ",")
}

// The steps of the requirements that apply to the object, by priority then
// requirement name; the requirement without a step adds none, the story's
// requirement adds nothing to a feature; no check is needed to read them.
func TestAGatesRequirementsAddTheirStepsInPriorityOrder(t *testing.T) {
	ctx, g := stepsWorld(t)
	f, err := g.Steps(ctx, api.StepsInput{ID: "m.f1", Gate: "ready-to-test"})
	if err != nil || f.Gate != "ready-to-test" || stepNames(f.Steps) != "secure:threat-model,alpha:test,zeta:review" {
		t.Fatalf("feature: %s (%v)", stepNames(f.Steps), err)
	}
	if s := f.Steps[1]; s.Step.Role != "tester" || s.Step.Skill != "k-test" || s.Provider == "" {
		t.Errorf("the gate's own step lost its fields or its provider: %+v", s)
	}
	if s := f.Steps[2]; s.Step.Interaction != api.InteractionValidate || s.Step.Role != "" {
		t.Errorf("a person's step: %+v", s)
	}
	s, err := g.Steps(ctx, api.StepsInput{ID: "m.s1", Gate: "ready-to-test"})
	if err != nil || stepNames(s.Steps) != "stories:story-only,alpha:test,zeta:review" {
		t.Errorf("story: %s (%v) — the feature's step leaked, or the story's was lost", stepNames(s.Steps), err)
	}
}

// A gate without requirements for a type passes, by design (types opt in),
// and adds no step; an undeclared gate is not found.
func TestAGateWithoutRequirementsPassesAndAddsNoStep(t *testing.T) {
	ctx, g := stepsWorld(t)
	c, err := g.Check(ctx, api.CheckInput{ID: "m.f1", Gate: "ready-to-plan"})
	if err != nil || !c.Checklist.Passes || len(c.Checklist.Items) != 0 {
		t.Errorf("a gate with no requirement must pass: %+v (%v)", c.Checklist, err)
	}
	st, err := g.Steps(ctx, api.StepsInput{ID: "m.f1", Gate: "ready-to-plan"})
	if err != nil || st.Steps == nil || len(st.Steps) != 0 {
		t.Errorf("no requirement, no step (an empty list): %+v (%v)", st, err)
	}
	if _, err := g.Steps(ctx, api.StepsInput{ID: "m.f1", Gate: "nope"}); !errors.Is(err, cwmapi.ErrNotFound) {
		t.Errorf("an undeclared gate: %v", err)
	}
}

// A malformed step is an error naming its requirement, never dropped: its
// item in the checklist reads unproven, so the gate cannot pass on it.
func TestAMalformedStepFailsAndItsItemReadsUnproven(t *testing.T) {
	ctx, g := stepsWorld(t)
	if _, err := g.Steps(ctx, api.StepsInput{ID: "m.f1", Gate: "broken"}); !errors.Is(err, cwmapi.ErrInvalidInput) || !strings.Contains(err.Error(), `"bad"`) || !strings.Contains(err.Error(), `unknown field "rol"`) {
		t.Errorf("a misspelt step must be refused naming the requirement and the key: %v", err)
	}
	c, err := g.Check(ctx, api.CheckInput{ID: "m.f1", Gate: "broken"})
	if err != nil || c.Checklist.Passes || len(c.Checklist.Items) != 1 || c.Checklist.Items[0].Status != api.Unproven {
		t.Errorf("the malformed step's item must read unproven: %+v (%v)", c.Checklist, err)
	}
}
