// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// THE DELIVERY ACTIONS ANSWER IN THE GUI'S LANGUAGE (
// F-093): BoardMove, BoardShip and SetStream given language fr answer in
// French - their toast and its Undo, which carries the language on - and the
// Open increments table and the panel pass the language in their actions'
// args.
func TestTheDeliveryActionsAnswerInTheGUIsLanguage(t *testing.T) {
	t.Parallel()
	ctx, _, _ := world(t)
	sm, rel, inc := streams(t, ctx), releases(t, ctx), increments(t, ctx)
	stream := func(name string) string {
		t.Helper()
		out, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: name, StreamFields: api.StreamFields{VersionScheme: api.SchemeSemver, ReleaseMode: api.ModeManual}})
		if err != nil {
			t.Fatal(err)
		}
		return out.Stream.ID
	}
	p, q := stream("Product"), stream("Platform")
	next := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.1.0", Date: "2027-01-01", Stream: p})
	create := func(title string, in string) string {
		t.Helper()
		out, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: streamIn(t, ctx, "billing"), Title: title, By: person})
		if err != nil {
			t.Fatal(err)
		}
		if in != "" {
			if _, err := sm.AimIncrement(ctx, api.StreamInInput{ID: out.Increment.ID, Stream: in}); err != nil {
				t.Fatal(err)
			}
		}
		return out.Increment.ID
	}
	english := []string{"stays in its stream", "already on hold", "Say why", "is on hold", "is not a priority", "resumed at",
		"ships in no release", "taken out of", "back in Open increments", "already ships", "ships in", "see Planned delivery",
		"is now in the", "stream", "Undo"}
	// answers runs every action once on fresh increments in lang and gives
	// each answer (message and Undo).
	answers := func(lang string) []render.Result {
		t.Helper()
		free, loose := create("Reminders "+lang, p), create("A loose idea "+lang, "")
		var rs []render.Result
		do := func(r render.Result, err error) {
			t.Helper()
			if err != nil {
				t.Fatal(err)
			}
			rs = append(rs, r)
		}
		do(inc.BoardMove(ctx, api.BoardMoveInput{ID: free, Row: "none", Column: "P1", By: person, Language: lang}))
		do(inc.BoardMove(ctx, api.BoardMoveInput{ID: free, Column: "P9", By: person, Language: lang}))
		do(inc.BoardMove(ctx, api.BoardMoveInput{ID: free, Column: "held", By: person, Language: lang}))
		do(inc.BoardMove(ctx, api.BoardMoveInput{ID: free, Column: "P0", By: person, Language: lang}))
		do(inc.BoardMove(ctx, api.BoardMoveInput{ID: free, Column: "held", Reason: "waits for the bank", By: person, Language: lang}))
		do(inc.BoardMove(ctx, api.BoardMoveInput{ID: free, Column: "held", Reason: "again", By: person, Language: lang}))
		do(inc.BoardMove(ctx, api.BoardMoveInput{ID: free, Column: "P1", By: person, Language: lang}))
		do(inc.BoardShip(ctx, api.BoardShipInput{ID: free, In: "out", By: person, Language: lang}))
		do(inc.BoardShip(ctx, api.BoardShipInput{ID: free, In: next.ID, By: person, Language: lang}))
		do(inc.BoardShip(ctx, api.BoardShipInput{ID: free, In: next.ID, By: person, Language: lang}))
		do(inc.BoardShip(ctx, api.BoardShipInput{ID: free, In: "out", By: person, Language: lang}))
		do(inc.BoardShip(ctx, api.BoardShipInput{ID: loose, In: p, By: person, Language: lang}))
		do(inc.SetStream(ctx, api.IncrementStreamInput{ID: loose, Stream: q, By: person, Language: lang}))
		return rs
	}

	// In French: no English left; each Undo is « Annuler » and asks
	// its answer in French too.
	for _, r := range answers("fr") {
		for _, w := range english {
			if strings.Contains(r.Message, w) {
				t.Errorf("the French answer %q keeps the English %q", r.Message, w)
			}
		}
		if r.Undo != nil && (r.Undo.Label != "Annuler" || r.Undo.Args["language"] != "fr") {
			t.Errorf("the French answer's Undo: %+v; want Annuler asking fr", r.Undo)
		}
	}
	fr := answers("fr")
	for n, want := range map[int]string{0: "reste dans son flux", 2: "Dites pourquoi il est suspendu", 3: " : P2 → P0", 4: "est suspendu : waits for the bank",
		6: "repris en P1", 8: ": voir Livraison planifiée", 10: ": il est de nouveau dans Incréments ouverts", 12: "est maintenant dans le flux Platform"} {
		if !strings.Contains(fr[n].Message, want) {
			t.Errorf("French answer %d %q lacks %q", n, fr[n].Message, want)
		}
	}
	// The builders pass the language: the table's hold and ships-in, the panel's hold.
	marshal := func(def render.Def, err error) string {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(def.DSLTree)
		return string(b)
	}
	free := create("Panel one", p)
	table := marshal(inc.OpenPage(ctx, api.OpenIncrementsInput{Stream: "all", Held: "not-held", Shown: 25, Language: "fr"}))
	if n := strings.Count(table, `"language":"fr"`); n < 2 {
		t.Errorf("the French table's actions carry the language %d times; want hold and ships-in on each line: %s", n, table)
	}
	if panel := marshal(inc.PanelPage(ctx, api.IncrementPanelInput{ID: free, Language: "fr"})); !strings.Contains(panel, `"language":"fr"`) {
		t.Errorf("the French panel's hold lacks the language: %s", panel)
	}
}
