// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/decisions/api"
	decisionslocal "github.com/katyoti/katyoti-fwk/internal/extensions/decisions/local"
	journalcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/journal/contrib"
	journallocal "github.com/katyoti/katyoti-fwk/internal/extensions/journal/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// setupJournal is setup with the journal recording CWM's events, so a
// decision's last move (who decided, when) can be read back.
func setupJournal(t *testing.T) (context.Context, cwmapi.CWMService, api.DecisionsService) {
	t.Helper()
	ctx, cwm := cwmapp.New(t, decisionPack(t), decisionslocal.Extension(),
		application.DeclareSimpleService[eventbus.EventBus](func(context.Context) (eventbus.EventBus, error) {
			return eventbus.NewMemoryEventBus(), nil
		}),
		journalcontrib.Pack(""), journallocal.Extension(), cwmapp.SystemModules(t),
	)
	// The journal records on its own goroutine: drain it before the
	// workspace is removed.
	t.Cleanup(func() { appcore.CloseApplicationScope(ctx) })
	cwmapp.Module(t, ctx, cwm, "m")
	svc, err := application.UseService[api.DecisionsService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, cwm, svc
}

// page is the decision's panel as the GUI receives it: decoded JSON.
func page(t *testing.T, ctx context.Context, svc api.DecisionsService, id, lang string) map[string]any {
	t.Helper()
	def, err := svc.Panel(ctx, api.PanelInput{ID: id, Language: lang})
	if err != nil {
		t.Fatalf("Panel %s: %v", id, err)
	}
	if err := def.Validate(); err != nil {
		t.Fatalf("the panel is off the page grammar: %v", err)
	}
	raw, _ := json.Marshal(def.DSLTree)
	var tree map[string]any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatal(err)
	}
	return tree
}

// nodesOf walks a page tree and returns every node of a kind.
func nodesOf(tree map[string]any, kind string) []map[string]any {
	var out []map[string]any
	var walk func(n map[string]any)
	walk = func(n map[string]any) {
		if n["kind"] == kind {
			out = append(out, n)
		}
		for _, key := range []string{"children", "aside"} {
			kids, _ := n[key].([]any)
			for _, k := range kids {
				if m, ok := k.(map[string]any); ok {
					walk(m)
				}
			}
		}
	}
	walk(tree)
	return out
}

// choices is what each form of the page resolves: the label each submit
// names, after checking it runs decisions/resolve on the decision as the
// person, with the reason typed.
func choices(t *testing.T, tree map[string]any, id string) []string {
	t.Helper()
	var labels []string
	for _, f := range nodesOf(tree, "form") {
		submit, _ := f["submit"].(map[string]any)
		args, _ := submit["args"].(map[string]any)
		if submit["uri"] != "ktw-methods:decisions/resolve" || args["id"] != id || args["actor"] != "${person}" || args["rationale"] != "${value.reason}" {
			t.Fatalf("a form does not resolve %s as the person with the reason: %v", id, submit)
		}
		sel, _ := args["selected"].([]any)
		if len(sel) != 1 {
			t.Fatalf("a form chooses one option: %v", args["selected"])
		}
		labels = append(labels, sel[0].(string))
	}
	return labels
}

func text(tree map[string]any) string {
	raw, _ := json.Marshal(tree)
	return string(raw)
}

var prepared = []api.OptionDetail{
	{Label: "Not listed", Means: "The rail lists only work universes.", Pros: []string{"No failing panel", "One entry"}, Cons: []string{"A new entry to find"}, Cost: "Small", Consequence: "OD-001's GUI text is rewritten."},
	{Label: "Listed, made useful", Means: "Its own views replace the tree.", Pros: []string{"Visible"}, Cons: []string{"A special case in every panel"}, Cost: "Medium"},
	{Label: "As today", Means: "Unchanged.", Cost: "None", Consequence: "The failing panel stays."},
}

// TestAPreparedDecisionShowsOneCardAndOneFormPerOption: prepared, a decision
// shows its proposal, its challenge (none, then the points) and one card per
// option whose form resolves its exact label; a second preparation replaces
// the first whole.
func TestAPreparedDecisionShowsOneCardAndOneFormPerOption(t *testing.T) {
	ctx, cwm, svc := setup(t)
	d := open(t, ctx, svc, "Should the admin universe be listed?")
	if _, err := svc.Prepare(ctx, api.PrepareInput{ID: d.ID, Options: prepared, Actor: cwmapi.Actor{ID: "specifier", Kind: "agent"},
		Proposal: api.Proposal{Option: "Not listed", Why: "The rail answers where do I work."}}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	tree := page(t, ctx, svc, d.ID, "fr")
	if got := choices(t, tree, d.ID); !slices.Equal(got, []string{"Not listed", "Listed, made useful", "As today"}) {
		t.Fatalf("one form per option, each its exact label: %v", got)
	}
	if cards := nodesOf(tree, "card"); len(cards) != 3 {
		t.Fatalf("one card per option: %d", len(cards))
	}
	all := text(tree)
	for _, want := range []string{"Proposition", "The rail answers where do I work.", "proposée", "Proposition contestée", "Pas encore contestée.",
		"Ce que cela signifie", "The rail lists only work universes.", "No failing panel", "A special case in every panel", "Pour", "Contre", "A new entry to find", "Coût", "Medium", "Si elle est choisie", "OD-001's GUI text is rewritten.", "Choisir cette option"} {
		if !strings.Contains(all, want) {
			t.Fatalf("the panel misses %q:\n%s", want, all)
		}
	}

	if _, err := svc.Challenge(ctx, api.ChallengeInput{ID: d.ID, Actor: cwmapi.Actor{ID: "reviewer", Kind: "agent"},
		Points: []string{"The fresh install has no work universe", "Who opens the chat?"}}); err != nil {
		t.Fatalf("Challenge: %v", err)
	}
	all = text(page(t, ctx, svc, d.ID, "en"))
	for _, want := range []string{"Challenged proposal", "The fresh install has no work universe", "Who opens the chat?", "Challenged by reviewer"} {
		if !strings.Contains(all, want) {
			t.Fatalf("the challenge misses %q:\n%s", want, all)
		}
	}

	// Prepared again: the labels and the sections are replaced whole.
	if _, err := svc.Prepare(ctx, api.PrepareInput{ID: d.ID, Options: prepared[:2], Actor: cwmapi.Actor{ID: "specifier", Kind: "agent"},
		Proposal: api.Proposal{Option: "Listed, made useful"}}); err != nil {
		t.Fatalf("Prepare again: %v", err)
	}
	obj, st := stored(t, ctx, cwm, d.ID)
	if strings.Count(obj.Body, "## Options") != 1 || strings.Contains(obj.Body, "### As today") {
		t.Fatalf("a second preparation replaces the first:\n%s", obj.Body)
	}
	if opts, _ := st["options"].([]any); len(opts) != 2 {
		t.Fatalf("options after the second preparation: %v", st["options"])
	}
	if got := choices(t, page(t, ctx, svc, d.ID, "en"), d.ID); !slices.Equal(got, []string{"Not listed", "Listed, made useful"}) {
		t.Fatalf("forms after the second preparation: %v", got)
	}

	// The proposal names one of the labels; a label is short and one line.
	if _, err := svc.Prepare(ctx, api.PrepareInput{ID: d.ID, Options: prepared, Actor: who, Proposal: api.Proposal{Option: "Elsewhere"}}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("a proposal that is not an option: %v", err)
	}
	long := append([]api.OptionDetail{{Label: strings.Repeat("a long sentence ", 8)}}, prepared...)
	if _, err := svc.Prepare(ctx, api.PrepareInput{ID: d.ID, Options: long, Actor: who, Proposal: api.Proposal{Option: "As today"}}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("a long label: %v", err)
	}
}

// TestALabelOnlyDecisionShowsPlainCardsWithButtons: a decision whose options
// have no prose section shows its labels as cards, each with its form.
func TestALabelOnlyDecisionShowsPlainCardsWithButtons(t *testing.T) {
	ctx, _, svc := setup(t)
	d := open(t, ctx, svc, "Which store?", "files", "sqlite")
	tree := page(t, ctx, svc, d.ID, "en")
	if got := choices(t, tree, d.ID); !slices.Equal(got, []string{"files", "sqlite"}) {
		t.Fatalf("a form per label: %v", got)
	}
	if cards := nodesOf(tree, "card"); len(cards) != 2 {
		t.Fatalf("a card per label: %d", len(cards))
	}
	if all := text(tree); !strings.Contains(all, "Not prepared yet") || strings.Contains(all, "Preparation") {
		t.Fatalf("an unprepared decision without a plan: %s", all)
	}
}

// TestAnUnpreparedDecisionShowsItsPrepareStepDetails: a decision prepared
// before options had prose sections keeps its analysis in its plan's prepare
// step; the panel shows it above the cards.
func TestAnUnpreparedDecisionShowsItsPrepareStepDetails(t *testing.T) {
	ctx, cwm, svc := setup(t)
	d := open(t, ctx, svc, "Should the admin universe be listed?", "Not listed", "As today")
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: d.ID})
	if err != nil {
		t.Fatal(err)
	}
	o := got.Object
	if err := o.SetCapability("plan", map[string]any{"steps": []any{
		map[string]any{"step": "intake", "details": "routed to univers"},
		map[string]any{"step": "decide/prepare", "details": "## Options\n\n| | Cost |\n|---|---|\n| **1. Not listed** | Small |\n\n## Recommendation: option 1"},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "m", Object: o}); err != nil {
		t.Fatal(err)
	}
	tree := page(t, ctx, svc, d.ID, "fr")
	all := text(tree)
	if !strings.Contains(all, "Préparation") || !strings.Contains(all, "Recommendation: option 1") || strings.Contains(all, "routed to univers") {
		t.Fatalf("the prepare step's details are not shown:\n%s", all)
	}
	if got := choices(t, tree, d.ID); !slices.Equal(got, []string{"Not listed", "As today"}) {
		t.Fatalf("the cards keep their forms: %v", got)
	}
}

// TestADecidedDecisionIsReadOnly: decided, the panel has no form and says
// what was chosen, by whom, when and why.
func TestADecidedDecisionIsReadOnly(t *testing.T) {
	ctx, _, svc := setupJournal(t)
	d := open(t, ctx, svc, "Which store?", "files", "sqlite")
	if _, err := svc.Resolve(ctx, api.ResolveInput{ID: d.ID, Actor: who, Selected: []string{"sqlite"}, Rationale: "It survives a restart."}); err != nil {
		t.Fatal(err)
	}
	var all string
	deadline := time.Now().Add(5 * time.Second)
	for {
		tree := page(t, ctx, svc, d.ID, "en")
		if forms := nodesOf(tree, "form"); len(forms) != 0 {
			t.Fatalf("a decided decision has no form: %d", len(forms))
		}
		all = text(tree)
		if strings.Contains(all, "Decided by jdoe on ") || time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	for _, want := range []string{"Decided by jdoe on " + time.Now().UTC().Format("2006-01-02"), "sqlite", "chosen", "Why", "It survives a restart."} {
		if !strings.Contains(all, want) {
			t.Fatalf("the decided panel misses %q:\n%s", want, all)
		}
	}
}

// TestAPersonDecidesThemselves: under an agent's cause, a resolve naming a
// person is refused, whatever actor says; the agent naming itself may take
// an agent's decision.
func TestAPersonDecidesThemselves(t *testing.T) {
	ctx, _, svc := setup(t)
	d := open(t, ctx, svc, "Which store?", "files", "sqlite")
	agentCtx := cause.WithCause(ctx, cause.Cause{Kind: cause.KindAgent, ID: "run-1"})
	_, err := svc.Resolve(agentCtx, api.ResolveInput{ID: d.ID, Actor: who, Selected: []string{"files"}})
	if !errors.Is(err, cwmapi.ErrInvalidInput) || !strings.Contains(err.Error(), "run-1") {
		t.Fatalf("a person's resolve from an agent's session: %v", err)
	}
	mine, err := svc.Open(ctx, api.OpenInput{Module: "m", Question: "Which cache?", Options: []string{"memory", "disk"}, Owner: api.OwnerAgent, OwnerReason: "technical and reversible"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Resolve(agentCtx, api.ResolveInput{ID: mine.Decision.ID, Actor: cwmapi.Actor{ID: "agent-1", Kind: "agent"}, Selected: []string{"memory"}, Rationale: "one process"}); err != nil {
		t.Fatalf("an agent resolving as itself: %v", err)
	}
}

// TestASecondResolveSaysWhoDecided: a decision decides once; the second
// resolve answers who decided it and when.
func TestASecondResolveSaysWhoDecided(t *testing.T) {
	ctx, _, svc := setupJournal(t)
	d := open(t, ctx, svc, "Which store?", "files", "sqlite")
	if _, err := svc.Resolve(ctx, api.ResolveInput{ID: d.ID, Actor: who, Selected: []string{"files"}}); err != nil {
		t.Fatal(err)
	}
	var err error
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, err = svc.Resolve(ctx, api.ResolveInput{ID: d.ID, Actor: cwmapi.Actor{ID: "other", Kind: "person"}, Selected: []string{"sqlite"}})
		if err == nil || strings.Contains(err.Error(), "by jdoe") || time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !errors.Is(err, cwmapi.ErrTypeRuleViolation) || !strings.Contains(err.Error(), "already decided by jdoe on "+time.Now().UTC().Format("2006-01-02")) || !strings.Contains(err.Error(), "files") {
		t.Fatalf("a second resolve: %v", err)
	}
}
