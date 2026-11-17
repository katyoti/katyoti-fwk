// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	decisionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/decisions/api"
	decisionslocal "github.com/katyoti/katyoti-fwk/internal/extensions/decisions/local"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	inboxcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/contrib"
	inboxlocal "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/local"
	journalapi "github.com/katyoti/katyoti-fwk/internal/extensions/journal/api"
	journalcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/journal/contrib"
	journallocal "github.com/katyoti/katyoti-fwk/internal/extensions/journal/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// TestTheAnswerRule: a rejection says what to change — on a request expecting
// a verdict and on an older one expecting nothing; a request expecting text is
// answered with that text, never rejected; a verdict is approved without a
// note. A refused answer leaves the request pending.
func TestTheAnswerRule(t *testing.T) {
	ctx, _, svc := setup(t)
	filed := func(title, expects string) api.Entry {
		t.Helper()
		out, err := svc.File(ctx, api.FileInput{Title: title, RequestedBy: "task-1", BlocksTask: true, Expects: expects})
		if err != nil {
			t.Fatalf("File %q: %v", title, err)
		}
		if out.Request.Expects != expects {
			t.Fatalf("File %q: expects %q; want %q", title, out.Request.Expects, expects)
		}
		return out.Request
	}
	refused := func(e api.Entry, in api.AnswerInput, why string) {
		t.Helper()
		in.ID = e.ID
		if _, err := svc.Answer(ctx, in); !errors.Is(err, cwmapi.ErrInvalidInput) {
			t.Errorf("%s: err = %v; want invalid input", why, err)
		}
		if st := entryStatus(t, ctx, svc, e.ID); st != api.StatusPending {
			t.Errorf("%s: the request is %q; want it still pending", why, st)
		}
	}

	review := filed("Review the totals", api.ExpectsVerdict)
	refused(review, api.AnswerInput{Decision: api.StatusRejected}, "a verdict rejected without a note")
	refused(review, api.AnswerInput{Decision: api.StatusRejected, Note: "  \n "}, "a verdict rejected with a blank note")
	if _, err := svc.Answer(ctx, api.AnswerInput{ID: review.ID, Decision: api.StatusApproved}); err != nil {
		t.Fatalf("a verdict approved without a note: %v", err)
	}

	sent := filed("Send it back", api.ExpectsVerdict)
	if _, err := svc.Answer(ctx, api.AnswerInput{ID: sent.ID, Decision: api.StatusRejected, Note: "the totals are wrong"}); err != nil {
		t.Fatalf("a verdict rejected with a note: %v", err)
	}

	question := filed("Which currency?", api.ExpectsText)
	refused(question, api.AnswerInput{Decision: api.StatusApproved}, "a question answered empty")
	refused(question, api.AnswerInput{Decision: api.StatusRejected, Note: "no"}, "a question rejected")
	if _, err := svc.Answer(ctx, api.AnswerInput{ID: question.ID, Note: "euros"}); err != nil {
		t.Fatalf("a question answered: %v", err)
	}
	dismissed := filed("Which colour?", api.ExpectsText)
	if _, err := svc.Dismiss(ctx, api.DismissInput{ID: dismissed.ID}); err != nil {
		t.Fatalf("a question dismissed: %v", err)
	}

	older := filed("Deploy?", "")
	refused(older, api.AnswerInput{Decision: api.StatusRejected}, "an older request rejected without a note")
	if _, err := svc.Answer(ctx, api.AnswerInput{ID: older.ID}); err != nil {
		t.Fatalf("an older request approved without a note: %v", err)
	}

	if _, err := svc.File(ctx, api.FileInput{Title: "Odd", Expects: "pick"}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Errorf("filed expecting %q: err = %v; want invalid input", "pick", err)
	}
}

// A CHOICE — a request about a decision, answered with one of its options
// (which decides the decision, as the person) or none, with a note that sends
// it back.

// resolveURI is ktw-methods' decisions/resolve, the method an option runs.
const resolveURI = "ktw-methods:decisions/resolve"

// routeDecisions is the ActionService of the binary hosting the inbox, routing
// decisions/resolve to the decisions service in process; it counts the
// resolves asked, so a refusal is seen to come before any.
type routeDecisions struct{ runs *atomic.Int32 }

func (routeDecisions) Describe(_ context.Context, in actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{URI: in.URI}, nil
}

func (r routeDecisions) Run(ctx context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	r.runs.Add(1)
	if in.URI != resolveURI {
		return actionsapi.RunOutput{}, fmt.Errorf("no route for %s", in.URI)
	}
	d, err := application.UseService[decisionsapi.DecisionsService](ctx)
	if err != nil {
		return actionsapi.RunOutput{}, err
	}
	var x decisionsapi.ResolveInput
	if err := json.Unmarshal(in.Input, &x); err != nil {
		return actionsapi.RunOutput{}, err
	}
	out, err := d.Resolve(ctx, x)
	if err != nil {
		return actionsapi.RunOutput{}, err
	}
	raw, _ := json.Marshal(out)
	return actionsapi.RunOutput{URI: in.URI, Value: raw}, nil
}

// choiceWorld is the inbox beside ktw-methods' decisions (the type and its
// workflow as shipped), reached through the ActionService; the journal records
// who moved a decision.
func choiceWorld(t *testing.T) (context.Context, cwmapi.CWMService, api.InboxService, decisionsapi.DecisionsService, *atomic.Int32) {
	t.Helper()
	runs := new(atomic.Int32)
	ctx, cwm := cwmapp.New(t, inboxlocal.Extension(), inboxcontrib.Pack(""), cwmapp.SystemModules(t),
		cwmapp.Pack(fstest.MapFS{
			"object-types/decision.yaml": {Data: []byte("name: decision\nfolder: decisions\ncapabilities: [container, content, decision, lifecycle]\nworkflow: decision\n")},
			"workflows/decision.yaml":    {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/workflows/decision.yaml")},
		}),
		decisionslocal.Extension(),
		application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) {
			return routeDecisions{runs: runs}, nil
		}),
		application.DeclareSimpleService[eventbus.EventBus](func(context.Context) (eventbus.EventBus, error) {
			return eventbus.NewMemoryEventBus(), nil
		}),
		journalcontrib.Pack(""), journallocal.Extension(),
	)
	// The journal records on its own goroutine: drain it before the
	// workspace is removed.
	t.Cleanup(func() { appcore.CloseApplicationScope(ctx) })
	seedInbox(t, ctx, cwm)
	cwmapp.Module(t, ctx, cwm, "m")
	svc, err := application.UseService[api.InboxService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := application.UseService[decisionsapi.DecisionsService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, cwm, svc, dec, runs
}

// aChoice opens a decision with its options and files the choice about it.
func aChoice(t *testing.T, ctx context.Context, svc api.InboxService, dec decisionsapi.DecisionsService, options ...string) (decision string, entry api.Entry) {
	t.Helper()
	d, err := dec.Open(ctx, decisionsapi.OpenInput{Module: "m", Question: "Which store?", Options: options})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	out, err := svc.File(ctx, api.FileInput{Title: "Decide: which store?", RequestedBy: "planification", BlocksTask: true,
		Expects: api.ExpectsChoice, About: d.Decision.ID, Options: options})
	if err != nil {
		t.Fatalf("File the choice: %v", err)
	}
	if out.Request.Expects != api.ExpectsChoice || !slices.Equal(out.Request.Options, options) {
		t.Fatalf("filed %+v; want a choice of %v", out.Request, options)
	}
	return d.Decision.ID, out.Request
}

// decisionNow is the decision's state and what it selected, read back.
func decisionNow(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, id string) (string, []string) {
	t.Helper()
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: id})
	if err != nil {
		t.Fatalf("ObjectGet %s: %v", id, err)
	}
	life, _ := got.Object.Lifecycle()
	if life.State == "" {
		life.State = decisionsapi.StatusOpen // the workflow's initial state, not written
	}
	var selected []string
	if raw, ok := got.Object.Capabilities["decision"]["selected"].([]any); ok {
		for _, v := range raw {
			selected = append(selected, fmt.Sprint(v))
		}
	}
	return life.State, selected
}

// asPerson is a call the person makes (the GUI, their own command).
func asPerson(ctx context.Context) context.Context {
	return cause.WithCause(ctx, cause.Cause{Kind: cause.KindPerson, ID: "jdoe"})
}

// TestAChoiceIsFiledAboutADecisionWithItsOptions: a choice names the decision
// it decides and at least two distinct options; no other kind takes options.
func TestAChoiceIsFiledAboutADecisionWithItsOptions(t *testing.T) {
	ctx, _, svc, dec, _ := choiceWorld(t)
	d, err := dec.Open(ctx, decisionsapi.OpenInput{Module: "m", Question: "Which store?", Options: []string{"files", "sqlite"}})
	if err != nil {
		t.Fatal(err)
	}
	about := d.Decision.ID
	for why, in := range map[string]api.FileInput{
		"a choice about nothing":    {Expects: api.ExpectsChoice, Options: []string{"files", "sqlite"}},
		"a choice of one option":    {Expects: api.ExpectsChoice, About: about, Options: []string{"files"}},
		"a choice of no option":     {Expects: api.ExpectsChoice, About: about},
		"an option offered twice":   {Expects: api.ExpectsChoice, About: about, Options: []string{"files", "files"}},
		"an option with no label":   {Expects: api.ExpectsChoice, About: about, Options: []string{"files", " "}},
		"options on a verdict":      {Expects: api.ExpectsVerdict, Options: []string{"files", "sqlite"}},
		"options on a question":     {Expects: api.ExpectsText, Options: []string{"files", "sqlite"}},
		"options on an older kind":  {Options: []string{"files", "sqlite"}},
		"an unknown kind of answer": {Expects: "pick", About: about, Options: []string{"files", "sqlite"}},
	} {
		in.Title = "Decide"
		if _, err := svc.File(ctx, in); !errors.Is(err, cwmapi.ErrInvalidInput) {
			t.Errorf("%s: err = %v; want invalid input", why, err)
		}
	}
	out, err := svc.File(ctx, api.FileInput{Title: "Decide", Expects: api.ExpectsChoice, About: about, Options: []string{"files", "sqlite"}})
	if err != nil {
		t.Fatalf("a choice about a decision with two options: %v", err)
	}
	got, err := svc.Get(ctx, api.GetInput{ID: out.Request.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got.Entry.Expects != api.ExpectsChoice || got.Entry.About != about || !slices.Equal(got.Entry.Options, []string{"files", "sqlite"}) {
		t.Errorf("read back %+v; want a choice about %s of files, sqlite", got.Entry, about)
	}
}

// TestAChoiceIsAnsweredWithOneOfItsOptions: approved names one of the options;
// an unknown one, none, or none without a note is refused — and a refused
// answer leaves the entry pending and the decision open.
func TestAChoiceIsAnsweredWithOneOfItsOptions(t *testing.T) {
	ctx, cwm, svc, dec, runs := choiceWorld(t)
	ctx = asPerson(ctx)
	decision, e := aChoice(t, ctx, svc, dec, "files", "sqlite")
	for why, in := range map[string]api.AnswerInput{
		"approved with no choice":    {Decision: api.StatusApproved},
		"approved by default":        {Note: "files"},
		"an option nobody offered":   {Choice: "postgres"},
		"none without a note":        {Decision: api.StatusRejected},
		"none with a blank note":     {Decision: api.StatusRejected, Note: " \n "},
		"none naming an option":      {Decision: api.StatusRejected, Choice: "files", Note: "neither"},
		"a choice of another answer": {Decision: "deferred", Choice: "files"},
	} {
		in.ID = e.ID
		if _, err := svc.Answer(ctx, in); !errors.Is(err, cwmapi.ErrInvalidInput) {
			t.Errorf("%s: err = %v; want invalid input", why, err)
		}
		if st := entryStatus(t, ctx, svc, e.ID); st != api.StatusPending {
			t.Errorf("%s: the entry is %q; want it pending", why, st)
		}
		if st, sel := decisionNow(t, ctx, cwm, decision); st != decisionsapi.StatusOpen || len(sel) > 0 {
			t.Errorf("%s: the decision is %s, selected %v; want it open", why, st, sel)
		}
		if n := runs.Swap(0); n != 0 {
			t.Errorf("%s: %d resolve asked; want the inbox to refuse it first", why, n)
		}
	}
	verdict, err := svc.File(ctx, api.FileInput{Title: "Review", Expects: api.ExpectsVerdict})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Answer(ctx, api.AnswerInput{ID: verdict.Request.ID, Choice: "files"}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Errorf("a verdict answered with a choice: err = %v; want invalid input", err)
	}
}

// TestAnOptionDecidesTheDecisionAsThePerson: the option chosen resolves the
// decision through decisions/resolve — selected that option, the note its
// rationale, the person its last move's actor — and the entry records it.
func TestAnOptionDecidesTheDecisionAsThePerson(t *testing.T) {
	ctx, cwm, svc, dec, _ := choiceWorld(t)
	ctx = asPerson(ctx)
	decision, e := aChoice(t, ctx, svc, dec, "files", "sqlite")
	out, err := svc.Answer(ctx, api.AnswerInput{ID: e.ID, Choice: "sqlite", Note: "the queries need it"})
	if err != nil {
		t.Fatalf("an option chosen: %v", err)
	}
	if out.Request.Status != api.StatusApproved || out.Request.Choice != "sqlite" || out.Request.ResolvedBy != "jdoe" {
		t.Errorf("the entry %+v; want approved, sqlite, by jdoe", out.Request)
	}
	if st, sel := decisionNow(t, ctx, cwm, decision); st != decisionsapi.StatusApplies || !slices.Equal(sel, []string{"sqlite"}) {
		t.Errorf("the decision is %s, selected %v; want it to apply, sqlite", st, sel)
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: decision})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Object.Body, "sqlite. the queries need it") {
		t.Errorf("the decision's prose %q; want the note as its rationale", got.Object.Body)
	}
	journal, err := application.UseService[journalapi.JournalService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	var last cwmapi.LifecycleEntry
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		moves, err := journal.Transitions(ctx, journalapi.TransitionsInput{ID: decision})
		if err == nil && len(moves.Entries) > 0 {
			if last = moves.Entries[len(moves.Entries)-1]; last.To == decisionsapi.StatusApplies {
				break
			}
		}
	}
	if last.To != decisionsapi.StatusApplies || last.By != (cwmapi.Actor{ID: "jdoe", Kind: cwmapi.ActorPerson}) {
		t.Errorf("the decision's last move %+v; want to applies by the person jdoe", last)
	}
}

// TestNoneSendsTheDecisionBack: none is rejected with a note; the decision is
// left as it is, to be prepared again.
func TestNoneSendsTheDecisionBack(t *testing.T) {
	ctx, cwm, svc, dec, _ := choiceWorld(t)
	ctx = asPerson(ctx)
	decision, e := aChoice(t, ctx, svc, dec, "files", "sqlite")
	out, err := svc.Answer(ctx, api.AnswerInput{ID: e.ID, Decision: api.StatusRejected, Note: "neither: weigh a hosted store"})
	if err != nil {
		t.Fatalf("none with a note: %v", err)
	}
	if out.Request.Status != api.StatusRejected || out.Request.Choice != "" || out.Request.DecisionNote != "neither: weigh a hosted store" {
		t.Errorf("the entry %+v; want rejected with the note, no choice", out.Request)
	}
	if st, sel := decisionNow(t, ctx, cwm, decision); st != decisionsapi.StatusOpen || len(sel) > 0 {
		t.Errorf("the decision is %s, selected %v; want it untouched, open", st, sel)
	}
}

// TestADecidedDecisionRefusesTheChoice: a decision decided elsewhere first
// answers who decided it; the entry stays pending (what moved the decision
// closes it).
func TestADecidedDecisionRefusesTheChoice(t *testing.T) {
	ctx, cwm, svc, dec, _ := choiceWorld(t)
	ctx = asPerson(ctx)
	decision, e := aChoice(t, ctx, svc, dec, "files", "sqlite")
	if _, err := dec.Resolve(ctx, decisionsapi.ResolveInput{ID: decision, Selected: []string{"files"}, Actor: cwmapi.Actor{ID: "jdoe", Kind: cwmapi.ActorPerson}}); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Answer(ctx, api.AnswerInput{ID: e.ID, Choice: "sqlite"})
	if !errors.Is(err, cwmapi.ErrTypeRuleViolation) || !strings.Contains(err.Error(), "already decided") {
		t.Fatalf("a choice on a decided decision: err = %v; want already decided", err)
	}
	if st := entryStatus(t, ctx, svc, e.ID); st != api.StatusPending {
		t.Errorf("the entry is %q; want it pending", st)
	}
	if _, sel := decisionNow(t, ctx, cwm, decision); !slices.Equal(sel, []string{"files"}) {
		t.Errorf("the decision selected %v; want files, as decided first", sel)
	}
}

// TestAnAgentCannotChoose: an agent's session never answers the person's
// choice, whatever by says; nothing moves.
func TestAnAgentCannotChoose(t *testing.T) {
	ctx, cwm, svc, dec, _ := choiceWorld(t)
	decision, e := aChoice(t, ctx, svc, dec, "files", "sqlite")
	agent := cause.WithCause(ctx, cause.Cause{Kind: cause.KindAgent, ID: "run-1"})
	if _, err := svc.Answer(agent, api.AnswerInput{ID: e.ID, Choice: "files", By: "jdoe"}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("an agent's choice: err = %v; want invalid input", err)
	}
	if st := entryStatus(t, ctx, svc, e.ID); st != api.StatusPending {
		t.Errorf("the entry is %q; want it pending", st)
	}
	if st, sel := decisionNow(t, ctx, cwm, decision); st != decisionsapi.StatusOpen || len(sel) > 0 {
		t.Errorf("the decision is %s, selected %v; want it open", st, sel)
	}
}

// TestAChoiceNeedsTheActionService: without the ActionService the decision
// cannot be decided, and the answer says so; the entry stays pending.
func TestAChoiceNeedsTheActionService(t *testing.T) {
	ctx, _, svc := setup(t)
	out, err := svc.File(ctx, api.FileInput{Title: "Decide", Expects: api.ExpectsChoice, About: "sys-inbox.inbox", Options: []string{"files", "sqlite"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Answer(ctx, api.AnswerInput{ID: out.Request.ID, Choice: "files"}); err == nil || !strings.Contains(err.Error(), "no ActionService") {
		t.Errorf("a choice with no ActionService: err = %v; want it to say so", err)
	}
	if st := entryStatus(t, ctx, svc, out.Request.ID); st != api.StatusPending {
		t.Errorf("the entry is %q; want it pending", st)
	}
}
