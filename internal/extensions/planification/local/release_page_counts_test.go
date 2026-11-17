// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A RELEASE PAGE SAYS WHERE IT STANDS AT A GLANCE (feature):
// each increment and request once - a request serving what two of its
// increments hold counts once -, the Increments gauge counts the increments
// its list shows (those riding along apart), the person's count equals the
// list of what awaits a person, one wording for it, and no id as a label:
// every object by its title, an id only behind a click or a hover.
func TestAReleasePageCountsEachThingOnceAndNamesItByItsTitle(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	sm, rel, inc := streams(t, ctx), releases(t, ctx), increments(t, ctx)
	st, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: "Product", StreamFields: api.StreamFields{VersionScheme: api.SchemeSemver, ReleaseMode: api.ModeManual}})
	if err != nil {
		t.Fatal(err)
	}
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.1.0", Stream: st.Stream.ID})
	in := func(title, uc string) string {
		t.Helper()
		id := increment(t, ctx, title).ID
		if _, err := sm.AimIncrement(ctx, api.StreamInInput{ID: id, Stream: st.Stream.ID}); err != nil {
			t.Fatal(err)
		}
		if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: id, In: r.ID}); err != nil {
			t.Fatal(err)
		}
		if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: uc, Increment: id}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	mail, csv := usecase(t, ctx, cwm, "Mail the invoice"), usecase(t, ctx, cwm, "Export to CSV")
	in("Invoices by mail", mail)
	in("Exports", csv)
	shared := file(t, ctx, req, api.FileInput{Title: "One sender for every channel", Module: "billing", Kind: api.KindBuild})
	if _, err := req.Assess(ctx, api.AssessInput{ID: shared.ID, Serves: mail, By: intake}); err != nil {
		t.Fatal(err)
	}
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: shared.ID, To: csv, Kind: api.EdgeServes}); err != nil {
		t.Fatal(err)
	}
	// A done increment of the stream in no release rides along.
	along := increment(t, ctx, "Faster startup").ID
	if _, err := sm.AimIncrement(ctx, api.StreamInInput{ID: along, Stream: st.Stream.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.Done(ctx, api.IncrementMoveInput{ID: along, By: person, Force: true, Note: forcedNote}); err != nil {
		t.Fatal(err)
	}

	def, err := releasePane(ctx, rel, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := def.Validate(); err != nil {
		t.Fatalf("the release's pane must display: %v", err)
	}
	b, _ := json.Marshal(def.DSLTree)
	raw := string(b)

	// Three requests: each increment's own plan, and the one they share -
	// counted once (summing the increments' counts made it four).
	if !strings.Contains(raw, `"label":"Requests"`) || !strings.Contains(raw, `"valueText":"0/3"`) {
		t.Errorf("the request two increments share is not counted once (want Requests 0/3): %s", gaugeTexts(raw))
	}
	if !strings.Contains(raw, `"label":"Increments","max":2`) && !strings.Contains(raw, `"valueText":"0/2"`) {
		t.Errorf("the Increments gauge does not count the two it carries: %s", gaugeTexts(raw))
	}
	if !strings.Contains(raw, `"text":"Riding along"`) || !strings.Contains(raw, "Faster startup") {
		t.Error("the increment riding along is not listed apart")
	}
	for _, mixed := range []string{"Waiting on you", "Needs you", "needs you"} {
		if strings.Contains(raw, mixed) {
			t.Errorf("the page says %q: one wording, « Awaiting a person »", mixed)
		}
	}
	// No id as a label: in a text, a heading, a badge's text or a row's title
	// or meta (an id stays in a row's id, a navigate target or a tooltip).
	id := regexp.MustCompile(`[a-z-]+\.(IN|RQ|RL|SM|UC|F|G)-\d+`)
	var walk func(n any)
	walk = func(n any) {
		switch v := n.(type) {
		case map[string]any:
			// Every object is named by its title: an id shows only in its own
			// id badge (mono) or after a title in brackets.
			for _, k := range []string{"content", "text", "title", "meta", "label"} {
				if s, ok := v[k].(string); ok && id.MatchString(s) && v["mono"] != true && !idBesideTitle(id, s) {
					t.Errorf("an id shows as a label: %s %q", k, s)
				}
			}
			for k, c := range v {
				if k != "navigate" && k != "action" {
					walk(c)
				}
			}
		case []any:
			for _, c := range v {
				walk(c)
			}
		}
	}
	var tree any
	_ = json.Unmarshal(b, &tree)
	walk(tree)
}

// gaugeTexts is what a page's gauges say, for a failure's message.
func gaugeTexts(raw string) string {
	return strings.Join(regexp.MustCompile(`"label":"[^"]+"|"valueText":"[^"]+"`).FindAllString(raw, -1), " ")
}
