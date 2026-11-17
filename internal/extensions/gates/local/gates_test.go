// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gates/api"
	gatescontrib "github.com/katyoti/katyoti-fwk/internal/extensions/gates/contrib"
	gateslocal "github.com/katyoti/katyoti-fwk/internal/extensions/gates/local"
	paneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	panelocal "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// NAMED GATES, on a real CWM. `ready-to-build` is declared once, with three
// requirements of its own; the `feature` type adds one from another file; a
// requirement's check calling score() is evaluated by CWM. A person's
// requirement is met by a person's confirmation, recorded. A workflow asks
// with the guard `gate('ready-to-build').passes`.

// The shipped capability names gate() host-relative; cwmapp hosts the pack
// under ktw-test, so it resolves to ktw-test.
const gateFn = "ktw-test:gates/gate-function"

// actions routes gate() to the gates service and answers score() with 3.
type actions struct{}

func (actions) Describe(_ context.Context, in actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{URI: in.URI, ReadOnly: true}, nil
}

func (actions) Run(ctx context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	if in.URI != gateFn {
		return actionsapi.RunOutput{URI: in.URI, Value: json.RawMessage(`3`)}, nil
	}
	var call cwmapi.FunctionCall
	if err := json.Unmarshal(in.Input, &call); err != nil {
		return actionsapi.RunOutput{}, err
	}
	g, err := application.UseService[api.GatesService](ctx)
	if err != nil {
		return actionsapi.RunOutput{}, err
	}
	cl, err := g.GateFunction(ctx, call)
	if err != nil {
		return actionsapi.RunOutput{}, err
	}
	raw, _ := json.Marshal(cl)
	return actionsapi.RunOutput{URI: in.URI, Value: raw}, nil
}

// seen is every CWM event of the world, as an observer receives it.
type seen struct {
	mu  sync.Mutex
	evs []cwmapi.EventEnvelope
}

func (s *seen) on(_ context.Context, ev cwmapi.EventEnvelope) {
	s.mu.Lock()
	s.evs = append(s.evs, ev)
	s.mu.Unlock()
}

// payloads decodes into T every event whose topic ends with suffix, waiting a
// little for the observer to catch up to want.
func payloads[T any](s *seen, suffix string, want int) []T {
	var out []T
	for i := 0; i < 50; i++ {
		out = nil
		s.mu.Lock()
		for _, ev := range s.evs {
			if strings.HasSuffix(ev.Topic, suffix) {
				var p T
				if json.Unmarshal(ev.Payload, &p) == nil {
					out = append(out, p)
				}
			}
		}
		s.mu.Unlock()
		if len(out) >= want && (want > 0 || i >= 5) {
			return out
		}
		time.Sleep(20 * time.Millisecond)
	}
	return out
}

var (
	ana = cwmapi.Actor{ID: "ana", Kind: cwmapi.ActorPerson}
	bot = cwmapi.Actor{ID: "bot", Kind: cwmapi.ActorAgent}
)

var pack = fstest.MapFS{
	"capabilities/lifecycle.yaml": {Data: []byte("name: lifecycle\n")},
	"capabilities/container.yaml": {Data: []byte("name: container\n")},
	"capabilities/scores.yaml":    {Data: []byte("name: scores\nfunctions:\n  - {name: score, action: 'x:score/get', returns: number}\n")},
	"object-types/module.yaml":    {Data: []byte("name: module\ncapabilities: [container]\n")},
	"object-types/feature.yaml":   {Data: []byte("name: feature\ncapabilities: [container, lifecycle, scores]\nworkflow: story\n")},
	"object-types/use-case.yaml":  {Data: []byte("name: use-case\ncapabilities: [container, lifecycle, scores]\nworkflow: story\n")},
	"object-types/story.yaml":     {Data: []byte("name: story\ncapabilities: [container, lifecycle, scores]\nworkflow: story\n")},
	"object-types/criterion.yaml": {Data: []byte("name: criterion\ncapabilities: [container]\n")},
	"workflows/story.yaml": {Data: []byte(`name: story
initial: idea
states: [{name: idea}, {name: building}]
transitions:
  - {name: start, from: [idea], to: building, gate: "gate('ready-to-build').passes", force: person}
`)},
	"gates/ready-to-build.yaml": {Data: []byte(`name: ready-to-build
label: Ready to build
requirements:
  - {name: described, check: "object.description != ''", resolve: {title: "Describe {object}"}}
  - {name: owner-agrees, person: true}
  - {name: scored, check: "score() > 2"}
`)},
	"gate-requirements/has-criteria.yaml": {Data: []byte("name: has-criteria\ngate: ready-to-build\napplies: {types: [feature]}\ncheck: \"len(children) > 0\"\nresolve: {title: 'Write the criteria of {object}', role: analyst}\n")},
	"gate-requirements/loops.yaml":        {Data: []byte("name: loops\ngate: ready-to-build\napplies: {types: [criterion]}\ncheck: \"gate('ready-to-build').passes\"\n")},
}

func world(t *testing.T) (context.Context, cwmapi.CWMService, api.GatesService, *seen) {
	t.Helper()
	events := &seen{}
	ctx, cwm := cwmapp.New(t,
		cwmapp.Pack(pack),
		gatescontrib.Pack(""),
		gateslocal.Extension(),
		panelocal.Extension(),
		cwmapi.ObserveEvents(cwmapi.EventObserver{Name: "gates-test", Handle: events.on}),
		// The in-process bus CWM publishes on (a host binary registers it).
		application.DeclareSimpleService[eventbus.EventBus](func(context.Context) (eventbus.EventBus, error) {
			return eventbus.NewMemoryEventBus(), nil
		}),
		application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) { return actions{}, nil }),
	)
	cwmapp.Module(t, ctx, cwm, "m")
	for _, o := range []cwmapi.Object{{ID: "m.f1", Type: "feature"}, {ID: "m.uc1", Type: "use-case"}, {ID: "m.s1", Type: "story"}, {ID: "m.c1", Type: "criterion"}} {
		o.Name, o.Parent = o.ID, "m"
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}
	g, err := application.UseService[api.GatesService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, cwm, g, events
}

func names(items []api.GateItem) string {
	var out []string
	for _, it := range items {
		out = append(out, it.Name+"="+it.Status)
	}
	return strings.Join(out, ",")
}

func describe(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, id string) {
	t.Helper()
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: id})
	if err != nil {
		t.Fatal(err)
	}
	o := got.Object
	o.Description = "What it does."
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "m", Object: o}); err != nil {
		t.Fatal(err)
	}
}

func TestAGateChecksEachRequirementForItsType(t *testing.T) {
	ctx, _, g, _ := world(t)
	f, err := g.Check(ctx, api.CheckInput{ID: "m.f1", Gate: "ready-to-build"})
	if err != nil || names(f.Checklist.Items) != "described=unmet,owner-agrees=unmet,scored=met,has-criteria=unmet" || f.Checklist.Passes {
		t.Fatalf("feature: %s (%v)", names(f.Checklist.Items), err)
	}
	if it := f.Checklist.Items[3]; it.Resolve["role"] != "analyst" || it.Kind != "check" {
		t.Errorf("the contributed requirement lost its resolve: %+v", it)
	}
	u, _ := g.Check(ctx, api.CheckInput{ID: "m.uc1", Gate: "ready-to-build"})
	if names(u.Checklist.Items) != "described=unmet,owner-agrees=unmet,scored=met" {
		t.Errorf("use case: %s — the feature's requirement leaked", names(u.Checklist.Items))
	}
	c, _ := g.Check(ctx, api.CheckInput{ID: "m.c1", Gate: "ready-to-build"})
	if it := c.Checklist.Items[len(c.Checklist.Items)-1]; it.Name != "loops" || it.Status != api.Unproven {
		t.Errorf("a check calling gate(): %+v — unproven, never a loop", it)
	}
	if _, err := g.Check(ctx, api.CheckInput{ID: "m.f1", Gate: "nope"}); !errors.Is(err, cwmapi.ErrNotFound) {
		t.Errorf("an undeclared gate: %v", err)
	}
}

func TestAPersonsRequirementIsConfirmedByAPerson(t *testing.T) {
	ctx, _, g, _ := world(t)
	if _, err := g.Confirm(ctx, api.ConfirmInput{ID: "m.f1", Gate: "ready-to-build", Requirement: "owner-agrees", By: bot}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Errorf("an agent confirmed: %v", err)
	}
	if _, err := g.Confirm(ctx, api.ConfirmInput{ID: "m.f1", Gate: "ready-to-build", Requirement: "described", By: ana}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Errorf("a checked requirement was confirmed by hand: %v", err)
	}
	out, err := g.Confirm(ctx, api.ConfirmInput{ID: "m.f1", Gate: "ready-to-build", Requirement: "owner-agrees", By: ana, Note: "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if it := out.Checklist.Items[1]; it.Status != api.Met || it.ConfirmedBy == nil || it.ConfirmedBy.By != ana || it.ConfirmedBy.At == "" {
		t.Errorf("not recorded: %+v", it)
	}
}

// The guard way: a refusal carries the checklist in gate()'s answer — in the
// error and in the published transition-refused — and only a person with a
// note may force it; once the requirements hold, the move passes.
func TestAGuardCallingGateRefusesWithItsChecklist(t *testing.T) {
	ctx, cwm, g, events := world(t)

	pane, err := application.UseService[paneapi.ObjectPaneService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	opts, _ := pane.TransitionOptions(ctx, paneapi.TransitionOptionsInput{ID: "m.s1"})
	tr := opts.Lifecycles[0].Transitions[0]
	if cl, ok := api.ChecklistOf(tr.Calls, "ready-to-build"); tr.Allowed || !ok || names(cl.Items) != "described=unmet,owner-agrees=unmet,scored=met" {
		t.Fatalf("option: %+v", tr)
	}

	_, err = cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: "m.s1", To: "building", By: ana})
	cl, ok := api.RefusedChecklist(err)
	if !ok || len(cl.Unmet()) != 2 || !errors.Is(err, cwmapi.ErrTransitionRefused) {
		t.Fatalf("refusal: %v", err)
	}
	refused := payloads[cwmapi.ObjectTransitionRefusedEventPayload](events, ".transition-refused", 1)
	if len(refused) != 1 {
		t.Fatalf("transition-refused events: %d, want 1", len(refused))
	}
	if fc, ok := api.FailingChecklist(refused[0].Calls); !ok || refused[0].ObjectID != "m.s1" || fc.Gate != "ready-to-build" || len(fc.Unmet()) != 2 {
		t.Errorf("the published refusal does not carry the checklist: %+v", refused[0])
	}

	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: "m.s1", To: "building", By: bot, Force: true, Note: "go"}); err == nil {
		t.Error("an agent forced past a person-forced guard")
	}
	describe(t, ctx, cwm, "m.s1")
	if _, err := g.Confirm(ctx, api.ConfirmInput{ID: "m.s1", Gate: "ready-to-build", Requirement: "owner-agrees", By: ana}); err != nil {
		t.Fatal(err)
	}
	out, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: "m.s1", To: "building", By: ana})
	if err != nil || out.Forced {
		t.Fatalf("a passed gate: %+v, %v", out, err)
	}
}
