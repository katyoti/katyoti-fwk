// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// AN INCREMENT'S GOALS TAB DRAWS EACH DELIVERY GOAL AS THE MOCK-UP'S GOAL CARD
// (request-rows-mockup.html section 4): a header of
// fixed columns 44 / share / 260 - the key, the goal's words in full, the
// progress (requests n / m done over the bar split by state in the shared
// palette, its swatch legend) -; « Became » and its chips; the open requests
// as the shared dense row in the groups' order; the done ones only in one
// rule fold with their count; a delivery constraint a dashed card with its
// tag and no bar.
func TestGoalsTabDrawsEachGoalAsTheMockupGoalCard(t *testing.T) {
	js := func(n any) string { b, _ := json.Marshal(n); return string(b) }
	steps := []api.StepChip{{Name: "intake", State: "done"}, {Name: "implement", State: "todo"}}
	p := paneData{lang: "en", requests: map[string]paneRequest{}}
	var ids []string
	for _, rq := range []paneRequest{
		{ID: "m.RQ-1", Bucket: "todo"},
		{ID: "m.RQ-2", Bucket: "done"},
		{ID: "m.RQ-3", Bucket: "waits", On: []string{"m.RQ-1"}},
		{ID: "m.RQ-4", Bucket: "run"},
		{ID: "m.RQ-5", Bucket: "done"},
		{ID: "m.RQ-6", Bucket: "wait"},
		{ID: "m.RQ-7", Bucket: "block"},
		{ID: "m.RQ-8", Ended: "dropped"},
		{ID: "m.RQ-9", Bucket: "done"},
	} {
		rq.Title, rq.Steps = "Request "+rq.ID, steps
		p.requests[rq.ID] = rq
		ids = append(ids, rq.ID)
	}
	goal := paneGoalLine{Key: "G1", Text: "An object's sources are the contributed source connectors, every one wherever sources are chosen", Analysed: true,
		Became: []api.HeldLine{{ID: "feeds.G-002"}, {ID: "feeds.F-011"}}, Requests: ids}
	card := p.goalCard(context.Background(), goal)
	raw := js(card)

	// One card per goal, its key; not dashed.
	if card["kind"] != "card" || card["data-goal"] != "G1" || card["dashed"] != nil || card["p"] != 0 {
		t.Fatalf("the goal is not one plain card keyed G1 padded by its parts: %s", raw)
	}
	// The header: fixed columns, the key, the words in full, the progress.
	var tree struct {
		Children []struct {
			Kind     string           `json:"kind"`
			Children []map[string]any `json:"children"`
		} `json:"children"`
	}
	_ = json.Unmarshal([]byte(raw), &tree)
	if len(tree.Children) != 3 {
		t.Fatalf("the card has %d parts, want header, Became, body: %s", len(tree.Children), raw)
	}
	head := tree.Children[0].Children[0]
	if head["kind"] != "grid" || !reflect.DeepEqual(head["widths"], []any{44.0, 0.0, 260.0}) || head["gap"] != 14.0 {
		t.Errorf("the header is not the grid 44 / share / 260 gap 14: %s", js(head))
	}
	hk := js(head)
	for _, want := range []string{`"content":"G1"`, `"data-goal-key":"G1"`, `"content":"` + goal.Text + `"`, `"content":"requests"`, `"content":"3"`, `"content":" / 8 done"`} {
		if !strings.Contains(hk, want) {
			t.Errorf("the header lacks %s: %s", want, hk)
		}
	}
	if strings.Contains(hk, "lineClamp") {
		t.Errorf("the goal's words are clamped; want them in full: %s", hk)
	}
	// The bar: one count only (the header's), split by state in the shared
	// palette, its legend swatches.
	var gauges []map[string]any
	var walk func(n any)
	walk = func(n any) {
		switch v := n.(type) {
		case map[string]any:
			if v["kind"] == "gauge" {
				gauges = append(gauges, v)
			}
			for _, c := range v {
				walk(c)
			}
		case []any:
			for _, c := range v {
				walk(c)
			}
		}
	}
	var whole any
	_ = json.Unmarshal([]byte(raw), &whole)
	walk(whole)
	if len(gauges) != 1 {
		t.Fatalf("the card draws %d bars, want 1: %s", len(gauges), raw)
	}
	g := gauges[0]
	if g["legend"] != "swatch" || g["valueText"] != "" || g["size"] != "md" {
		t.Errorf("the bar is not a md bar with a swatch legend and no count of its own: %s", js(g))
	}
	var got []string
	for _, s := range g["segments"].([]any) {
		sm := s.(map[string]any)
		got = append(got, js(sm["value"])+" "+sm["label"].(string)+" "+sm["color"].(string))
	}
	want := []string{"3 done green.6", "1 running orange.6", "1 on you grape.6", "1 waiting violet.7", "1 to do dark.2", "1 blocked red.6"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the bar's segments %v; want %v (the goal's requests by state, the row stripe's palette, the dropped one counted nowhere)", got, want)
	}
	// One palette: each segment takes its row group's stripe colour.
	for _, b := range paneBuckets {
		if b.group != "" && bucketTone(b.group) != rowStripe[b.group] {
			t.Errorf("the bucket %s is drawn %s, its row stripe %s", b.key, bucketTone(b.group), rowStripe[b.group])
		}
	}
	// Became and its chips.
	became := js(tree.Children[1])
	if !strings.Contains(became, `"content":"Became"`) || !strings.Contains(became, `"data-open":"feeds.G-002"`) || !strings.Contains(became, `"data-open":"feeds.F-011"`) {
		t.Errorf("the Became line lacks its chips: %s", became)
	}
	// The body: the open requests as the shared dense row in the groups'
	// order, then the one fold of the done ones.
	body := tree.Children[2].Children
	var rows []string
	for _, n := range body {
		if r, ok := n["data-request"].(string); ok {
			if n["data-dense"] != true || n["data-state"] == nil {
				t.Errorf("the row of %s is not the shared dense row: %s", r, js(n))
			}
			rows = append(rows, r)
		}
	}
	if strings.Join(rows, ",") != "m.RQ-4,m.RQ-6,m.RQ-7,m.RQ-3,m.RQ-1" {
		t.Errorf("the open rows %v; want running, on you, blocked, waiting, to do: m.RQ-4, m.RQ-6, m.RQ-7, m.RQ-3, m.RQ-1", rows)
	}
	last := body[len(body)-1]
	if last["kind"] != "accordion" || last["look"] != "rule" || last["defaultOpen"] != nil {
		t.Fatalf("the done ones are not one closed rule fold last: %s", js(last))
	}
	panels := last["panels"].([]any)
	fold := panels[0].(map[string]any)
	if len(panels) != 1 || fold["label"] != "3 done · 1 dropped" || fold["data-goal-done"] != "G1" {
		t.Errorf("the fold: %s; want one fold « 3 done · 1 dropped » data-goal-done G1", js(panels))
	}
	fc := js(fold["content"])
	for _, id := range []string{"m.RQ-2", "m.RQ-5", "m.RQ-9", "m.RQ-8"} {
		if !strings.Contains(fc, `"data-open":"`+id+`"`) {
			t.Errorf("the fold lacks the chip of %s: %s", id, fc)
		}
		if strings.Contains(js(body[:len(body)-1]), `"`+id+`"`) {
			t.Errorf("%s is drawn as a row; want it only in the fold", id)
		}
	}

	// A delivery constraint: a dashed card, a gray key, its tag, no bar.
	c := p.goalCard(context.Background(), paneGoalLine{Key: "G6", Text: "The former display keeps working", Analysed: true, Constraint: true})
	cr := js(c)
	if c["dashed"] != true || c["data-goal"] != "G6" || strings.Contains(cr, `"kind":"gauge"`) {
		t.Errorf("the constraint is not a dashed card with no bar: %s", cr)
	}
	for _, want := range []string{`"text":"delivery constraint"`, `"content":"checked when the increment ends"`, `"bg":"var(--mantine-color-gray-light)"`} {
		if !strings.Contains(cr, want) {
			t.Errorf("the constraint lacks %s: %s", want, cr)
		}
	}

	// In French: the fold's ended words, the bar's legend and the row groups
	// in the GUI's language, one word for a request on a person - the
	// mock-up's « à vous », the row group's.
	fr := paneData{lang: "fr", requests: p.requests}
	frCard := js(fr.goalCard(context.Background(), goal))
	for _, want := range []string{`"label":"3 faites · 1 abandonnées"`, `"label":"à vous"`, `"label":"à faire"`} {
		if !strings.Contains(frCard, want) {
			t.Errorf("the French goal card lacks %s: %s", want, frCard)
		}
	}
	for _, leak := range []string{"dropped", `"label":"on you"`, "awaiting a person"} {
		if strings.Contains(frCard, leak) {
			t.Errorf("the French goal card says %q: %s", leak, frCard)
		}
	}

	// A goal whose only requests ended (dropped, split): no « no request
	// yet », no empty bar; the fold says how they ended.
	p.requests["m.RQ-10"] = paneRequest{ID: "m.RQ-10", Title: "Split one", Ended: "split"}
	for _, pd := range []paneData{p, {lang: "fr", requests: p.requests}} {
		d := js(pd.goalCard(context.Background(), paneGoalLine{Key: "G3", Text: "Given up", Analysed: true, Requests: []string{"m.RQ-8", "m.RQ-10"}}))
		if strings.Contains(d, goalWord(pd.lang, "no_request")) || strings.Contains(d, `"kind":"gauge"`) {
			t.Errorf("a goal whose requests all ended says « no request yet » or draws a bar (%s): %s", pd.lang, d)
		}
		fold := map[string]string{"en": `"label":"0 done · 1 dropped · 1 split"`, "fr": `"label":"0 faites · 1 abandonnées · 1 scindées"`}[pd.lang]
		if !strings.Contains(d, fold) || !strings.Contains(d, goalWord(pd.lang, "no_open")) {
			t.Errorf("a goal whose requests all ended lacks %s and « %s » (%s): %s", fold, goalWord(pd.lang, "no_open"), pd.lang, d)
		}
	}

	// A goal with no request yet: an empty bar and its words.
	e := js(p.goalCard(context.Background(), paneGoalLine{Key: "G2", Text: "Not yet served", Analysed: true, Became: []api.HeldLine{{ID: "m.F-1"}}}))
	if !strings.Contains(e, `"content":"no request yet"`) || !strings.Contains(e, `"max":1`) {
		t.Errorf("a goal with no request lacks its empty bar and words: %s", e)
	}
}
