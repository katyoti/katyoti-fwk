// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// THE INCREMENT AND REQUEST VIEWS READ WHAT HOLDS WORK FROM THE ONE READER
// : on the one K1-K6 state, its
// requests filed in one increment, every line that says what holds a request
// - the increment summary's waits (a blocked plan's line), each demand's why
// (what the deck's cards and the increment's lists read) - names the hold's
// K-number, kind and holder exactly as planning check names that request's;
// and the request view offers to retry a stopped step only when the check
// puts the stop on the person (past the agents' cap, an incident) - never for
// a step an agent retries, an outside party's hold or a wait on other work,
// whose expectation names its holder and asks nothing.
func TestTheIncrementAndRequestViewsNameAHoldExactlyWhereTheCheckDoes(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	in := increment(t, ctx, "Invoices reach their customers")
	w := fillOneReaderWorldIn(t, ctx, req, in.ID)
	// A plan blocked: the increment's summary says what holds it.
	if _, err := plansOf(t, ctx).Block(ctx, api.BlockInput{ID: w.Outside, Note: "the screen waits on its runner", By: session}); err != nil {
		t.Fatal(err)
	}
	blockers := checkPutsOnThePerson(t, ctx, w)
	head := func(b api.Blocker) string { return api.BlockerClass(b.Kind) + " " + b.Kind + " by " + b.Actor }
	// A hold as the views word it starts with its K-number, its kind and
	// " by ": who moves it follows, then why.
	said := regexp.MustCompile(`K[1-6] (answer|act|ready|held|outside|incident) by `)
	// names says every hold a line words is b's.
	names := func(line string, b api.Blocker) bool {
		for _, at := range said.FindAllStringIndex(line, -1) {
			if !strings.HasPrefix(line[at[0]:], head(b)) {
				return false
			}
		}
		return true
	}

	// The increment's summary: a request's line names its hold as the check.
	sum, err := increments(t, ctx).Summary(ctx, api.GetInput{ID: in.ID})
	if err != nil {
		t.Fatal(err)
	}
	for work, b := range blockers {
		for _, line := range sum.Waits {
			if strings.HasPrefix(line, b.Title+" ") && !names(line, b) {
				t.Errorf("the summary says %q; the check holds %s as %q", line, work, head(b))
			}
		}
	}
	if b := blockers[w.Outside]; !slices.ContainsFunc(sum.Waits, func(l string) bool {
		return strings.HasPrefix(l, b.Title+" ") && strings.Contains(l, head(b))
	}) {
		t.Errorf("the summary's waits %q never name the blocked %s's hold %q", sum.Waits, w.Outside, head(b))
	}

	// The demands: why a request is not moving is the hold the check gives
	// it.
	ds, err := req.Demands(ctx, api.DemandsInput{Module: "billing"})
	if err != nil {
		t.Fatal(err)
	}
	why := map[string]string{}
	var walk func([]api.Demand)
	walk = func(ds []api.Demand) {
		for _, d := range ds {
			why[d.ID] = d.Why
			walk(d.Parts)
		}
	}
	walk(ds.Demands)
	for work, b := range blockers {
		if !names(why[work], b) {
			t.Errorf("%s: its demand says %q; the check holds it as %q", work, why[work], head(b))
		}
	}
	for _, work := range []string{w.Ready, w.Incident, w.Failed, w.FailedOnce, w.OnWork} {
		if !strings.Contains(why[work], head(blockers[work])) {
			t.Errorf("%s: its demand says %q; want the check's hold %q", work, why[work], head(blockers[work]))
		}
	}

	// The request view: retry only for a stop the check puts on the person.
	views, err := application.UseService[api.ViewService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, work := range []string{w.Incident, w.Failed, w.FailedOnce, w.Outside, w.OnWork} {
		b := blockers[work]
		onPerson := w.onThePerson()[work]
		def, err := views.Request(ctx, api.RequestViewInput{ID: work})
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(def.DSLTree)
		e := expectedItem(t, string(raw))
		_, retry := e["todo"]
		if retry != onPerson {
			t.Errorf("%s: the request view offers a retry: %v; the check puts it on the person: %v (%s, %q)", work, retry, onPerson, head(b), e["meta"])
		}
		if !onPerson && !strings.Contains(e["meta"].(string), b.Actor+" ("+api.BlockerClass(b.Kind)+" "+b.Kind+")") {
			t.Errorf("%s: the request view expects %q; want it to name the check's holder %s", work, e["meta"], head(b))
		}
	}

	// The deck's Needs a person: a stopped step is the person's card - with
	// its retry - only when the check puts it on the person; a step an agent
	// retries, an outside party's hold or a wait on other work is no card.
	def, err := views.Deck(ctx, api.DeckInput{})
	if err != nil {
		t.Fatal(err)
	}
	cards := needsCards(t, def.DSLTree)
	for _, work := range []string{w.Incident, w.Failed, w.FailedOnce, w.Outside, w.OnWork} {
		c, listed := cards[work]
		_, retry := c["todo"]
		if onPerson := w.onThePerson()[work]; listed != onPerson || retry != onPerson {
			t.Errorf("%s: Needs a person lists it: %v, offers a retry: %v; the check puts it on the person: %v (%s)", work, listed, retry, onPerson, head(blockers[work]))
		}
	}
}

// needsCards are the cards of the deck's Needs a person tab, by request.
func needsCards(t *testing.T, tree any) map[string]map[string]any {
	t.Helper()
	raw, _ := json.Marshal(tree)
	var deck struct {
		Tabs []struct {
			ID      string `json:"id"`
			Content any    `json:"content"`
		} `json:"tabs"`
	}
	if err := json.Unmarshal(raw, &deck); err != nil {
		t.Fatal(err)
	}
	cards := map[string]map[string]any{}
	var walk func(any)
	walk = func(n any) {
		switch v := n.(type) {
		case map[string]any:
			if v["kind"] == "lanes" {
				for _, c := range v["data"].([]any) {
					card := c.(map[string]any)
					cards[card["id"].(string)] = card
				}
				return
			}
			for _, x := range v {
				walk(x)
			}
		case []any:
			for _, x := range v {
				walk(x)
			}
		}
	}
	for _, tab := range deck.Tabs {
		if tab.ID == "needs" {
			walk(tab.Content)
			return cards
		}
	}
	t.Fatal("the deck has no Needs a person tab")
	return nil
}

// expectedItem is the request view's « What is expected » item.
func expectedItem(t *testing.T, view string) map[string]any {
	t.Helper()
	at := strings.Index(view, `"title":"What is expected"`)
	if at < 0 {
		t.Fatalf("the request view expects nothing: %s", view)
	}
	start := strings.LastIndex(view[:at], "{")
	end := strings.Index(view[at:], "}") + at + 1
	var e map[string]any
	if err := json.Unmarshal([]byte(view[start:end]), &e); err != nil {
		t.Fatalf("the expected item %s: %v", view[start:end], err)
	}
	return e
}
