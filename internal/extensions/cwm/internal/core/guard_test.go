// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/events"
)

// GUARDS (b7): a refused guard says what its functions answered, in the error
// and in a published `transition-refused`; a transition says who may force it
// (anyone, or a person with a note) and a forced move keeps the answers;
// ObjectEvaluate lends the guard's vocabulary to a service with expressions
// of its own.

var ana = api.Actor{ID: "ana", Kind: api.ActorPerson}

func guardFixture(t *testing.T, guard string) (*Service, stdctx.Context, *fnActions, *captured) {
	t.Helper()
	acts := newFnActions()
	svc, ctx := fnFixture(t, guard, acts)
	bus := eventbus.NewMemoryEventBus()
	svc.Events = events.New("t", "u", "w", bus, nil)
	var c captured
	_ = bus.Subscribe(api.ObjectTransitionRefusedSubject("t", "u", "w", api.SubjectToken("m.t")), c.on)
	return svc, ctx, acts, &c
}

func TestObjectEvaluateAnswersEachExpression(t *testing.T) {
	svc, ctx, acts, _ := guardFixture(t, "")
	out, err := svc.ObjectEvaluate(ctx, api.ObjectEvaluateInput{ID: "m.t", By: ana,
		Expressions: []string{"score() == 3", "gate('ready')", "score() +", "score() + 1", "by.id"}})
	if err != nil || len(out.Results) != 5 {
		t.Fatalf("evaluate: %+v, %v", out, err)
	}
	r := out.Results
	if r[0].Value != true || r[1].Value != true || r[2].Error == "" || r[3].Value != 4 && r[3].Value != 4.0 || r[4].Value != "ana" {
		t.Fatalf("values: %+v", r)
	}
	if a, ok := api.AnswerOf(r[3].Calls, "score"); !ok || a.Answer != 3.0 {
		t.Errorf("the fourth expression's calls: %+v — score() answered once, for both", r[3].Calls)
	}
	if a, ok := api.AnswerOf(r[1].Calls, "gate", "ready"); !ok || a.Answer != true || len(r[1].Calls) != 1 {
		t.Errorf("gate('ready')'s calls: %+v", r[1].Calls)
	}
	if n := acts.count("x:score/get"); n != 1 {
		t.Errorf("score() asked %d times by one evaluation, want once", n)
	}
	if _, err := svc.ObjectEvaluate(ctx, api.ObjectEvaluateInput{ID: "m.t"}); !errors.Is(err, api.ErrInvalidInput) {
		t.Errorf("no expressions: %v", err)
	}
}

func TestARefusedGuardSaysWhatItsFunctionsAnswered(t *testing.T) {
	svc, ctx, acts, c := guardFixture(t, "score() > 5")
	_, err := svc.ObjectTransition(ctx, api.ObjectTransitionInput{ID: "m.t", To: "accepted", By: ana, DryRun: true})
	var refused *api.TransitionRefusedError
	if !errors.As(err, &refused) || !errors.Is(err, api.ErrTransitionRefused) {
		t.Fatalf("a dry run refuses as the move would: %v", err)
	}
	if len(c.all()) != 0 {
		t.Error("a dry run published a refusal — a question is not a refused move")
	}
	err = fnMove(svc, ctx)
	if !errors.As(err, &refused) || refused.Transition != "accept" || refused.From != "proposed" || refused.Guard != "score() > 5" {
		t.Fatalf("refusal: %v", err)
	}
	if a, ok := api.AnswerOf(refused.Calls, "score"); !ok || a.Answer != 3.0 || !strings.Contains(err.Error(), "score(): 3") {
		t.Errorf("the refusal must carry and say score()'s answer: %+v — %v", refused.Calls, err)
	}
	evs := c.all()
	if len(evs) != 1 {
		t.Fatalf("transition-refused events: %d, want 1", len(evs))
	}
	p := evs[0].(events.ObjectTransitionRefusedPayload)
	if p.ObjectID != "m.t" || p.Transition != "accept" || p.By != ana || p.Workflow != "flow" || len(p.Calls) != 1 {
		t.Errorf("payload: %+v", p)
	}
	if len(acts.seen) == 0 || acts.seen[0].By == nil || *acts.seen[0].By != ana || acts.seen[0].From != "proposed" || acts.seen[0].To != "accepted" {
		t.Errorf("a function in a guard is told the move: %+v", acts.seen)
	}
}

func TestForcePersonTakesAPersonAndANote(t *testing.T) {
	svc, ctx, _, c := guardFixture(t, "score() > 5")
	svc.Config.CatalogTypes.Workflows[0].Transitions[0].Force = api.ForcePerson
	bot := api.Actor{ID: "bot", Kind: api.ActorAgent}
	for _, in := range []api.ObjectTransitionInput{
		{ID: "m.t", To: "accepted", By: bot, Force: true, Note: "go"},
		{ID: "m.t", To: "accepted", By: ana, Force: true},
	} {
		if _, err := svc.ObjectTransition(ctx, in); !errors.Is(err, api.ErrTransitionRefused) || !strings.Contains(err.Error(), "a person and a note") {
			t.Errorf("%s forced (note %q): %v", in.By.Kind, in.Note, err)
		}
	}
	// A bus of its own for the forced move (the refusals above went to c's).
	bus := eventbus.NewMemoryEventBus()
	svc.Events = events.New("t", "u", "w", bus, nil)
	var moved captured
	_ = bus.Subscribe(api.ObjectTransitionedSubject("t", "u", "w", api.SubjectToken("m.t")), moved.on)
	out, err := svc.ObjectTransition(ctx, api.ObjectTransitionInput{ID: "m.t", To: "accepted", By: ana, Force: true, Note: "demo tomorrow"})
	if err != nil || !out.Forced {
		t.Fatalf("a person with a note: %+v, %v", out, err)
	}
	// The forced move's event carries what the guard's functions answered:
	// the journal keeps it with the move.
	evs := moved.all()
	if len(evs) != 1 {
		t.Fatalf("transitioned events: %d, want 1", len(evs))
	}
	last := evs[0].(events.ObjectTransitionedPayload)
	if a, ok := api.AnswerOf(last.Calls, "score"); !last.Forced || !ok || a.Answer != 3.0 {
		t.Errorf("a forced move carries what the guard's functions answered: %+v", last)
	}
	if n := len(c.all()); n != 2 {
		t.Errorf("transition-refused events: %d, want the two refused forcings", n)
	}

	svc.Config.CatalogTypes.Workflows[0].Transitions[0].Force = api.ForceNone
	svc2, ctx2, _, _ := guardFixture(t, "score() > 5")
	if _, err := svc2.ObjectTransition(ctx2, api.ObjectTransitionInput{ID: "m.t", To: "accepted", By: ana, Force: true, Note: "x"}); !strings.Contains(err.Error(), "does not allow forcing") {
		t.Errorf("a transition without force: %v", err)
	}
}

func TestAForceRuleReadsTrueOrWho(t *testing.T) {
	for raw, want := range map[string]api.ForceRule{"true": api.ForceAnyone, "false": api.ForceNone, "person": api.ForcePerson, "anyone": api.ForceAnyone} {
		var tr api.WorkflowTransition
		if err := config.DecodeBytes([]byte("name: x\nto: y\nforce: "+raw+"\n"), marshal.YAML, &tr); err != nil || tr.Force != want {
			t.Errorf("yaml force: %s → %q, %v; want %q", raw, tr.Force, err, want)
		}
		var j api.WorkflowTransition
		js := raw
		if raw == "person" || raw == "anyone" {
			js = `"` + raw + `"`
		}
		if err := json.Unmarshal([]byte(`{"name":"x","to":"y","force":`+js+`}`), &j); err != nil || j.Force != want {
			t.Errorf("json force: %s → %q, %v; want %q", raw, j.Force, err, want)
		}
	}
	var tr api.WorkflowTransition
	if err := config.DecodeBytes([]byte("name: x\nto: y\nforce: maybe\n"), marshal.YAML, &tr); err == nil {
		t.Error("force: maybe was accepted")
	}
}

// TestARefusalReasonIsOneShortLine: a gate answering many unmet
// items gives a reason of one line, at most ~300 characters, naming the gate,
// the unmet count and the first labels, with no HTML escaping — the full
// answer stays on the refusal's Calls.
func TestARefusalReasonIsOneShortLine(t *testing.T) {
	items := make([]any, 0, 50)
	items = append(items, map[string]any{"name": "ok", "label": "already met", "status": "met"})
	items = append(items, map[string]any{"name": "review", "label": "delivery review <not> run", "status": "unmet",
		"reason": "delivery review <not> run " + strings.Repeat("a long reason\nover lines ", 20)})
	for i := 0; i < 41; i++ {
		items = append(items, map[string]any{"name": "f", "label": "m.F-0" + string(rune('0'+i%10)) + " not done", "status": "unproven"})
	}
	log := &callLog{}
	log.add("gate", []any{"usable-delivered"}, map[string]any{"gate": "usable-delivered", "passes": false, "items": items}, nil)
	log.add("score", nil, 3.0, nil)
	reason := "the guard \"gate('usable-delivered')\" does not hold" + calledFunctions(map[string]any{callLogKey: log})

	if n := len([]rune(reason)); n > 300 {
		t.Errorf("the reason is %d characters, want at most 300: %s", n, reason)
	}
	if strings.ContainsAny(reason, "\n\r") || strings.Contains(reason, "\\"+"u003c") {
		t.Errorf("the reason must be one unescaped line: %q", reason)
	}
	for _, want := range []string{`gate("usable-delivered")`, "42 unmet", "review: delivery review <not> run", "f: m.F-00 not done", "score(): 3"} {
		if !strings.Contains(reason, want) {
			t.Errorf("the reason must name %q: %s", want, reason)
		}
	}
	if strings.Contains(reason, "already met") {
		t.Errorf("a met item is not named: %s", reason)
	}
}
