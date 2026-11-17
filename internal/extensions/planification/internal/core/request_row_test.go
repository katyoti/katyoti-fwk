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

// EVERY LIST OF REQUESTS DRAWS THE ONE REQUEST ROW (
// request-rows-mockup.html section 3): four fixed columns - the state stripe,
// id and kind, title over its meta line, step dots with « n/m · step » - the
// dense row in a Running card and a goal; an increment's open requests in
// groups Running, On you, Blocked, Waiting, To do, their counts the head's,
// the done ones folded last.
func TestEveryRequestListDrawsTheSharedRequestRow(t *testing.T) {
	steps := func(states ...string) []api.StepChip {
		var out []api.StepChip
		for i, s := range states {
			out = append(out, api.StepChip{Name: []string{"intake", "analyse", "implement", "auto-review", "module-update", "review"}[i], State: s})
		}
		return out
	}
	js := func(n any) string { b, _ := json.Marshal(n); return string(b) }
	cells := func(row node) []any {
		t.Helper()
		item, _ := row["item"].(node)
		if item == nil {
			t.Fatalf("the row has no item: %s", js(row))
		}
		if item["kind"] != "grid" {
			t.Fatalf("the row is not a grid: %s", js(item))
		}
		kids, _ := json.Marshal(item["children"])
		var out []any
		_ = json.Unmarshal(kids, &out)
		return out
	}

	// The columns: 4px stripe, id, the title sharing the rest, the steps; a
	// line before the title and before the steps; the card padded by its cells.
	held := paneRequest{ID: "feeds.RQ-047", Title: "Every source scope carries a connection field", Bucket: "run", Kind: "build",
		Steps: steps("done", "done", "done", "running", "todo", "todo")}
	row := requestRow("en", held, openStep{Step: "auto-review", Who: "claude-b", Since: "08:38", Label: "auto-review · claude-b"}, rowOpts{})
	grid := row["item"].(node)
	if !reflect.DeepEqual(grid["widths"], []int{4, 150, 0, 210}) || !reflect.DeepEqual(grid["dividers"], []int{2, 3}) || grid["gap"] != 0 {
		t.Errorf("the row's columns: widths %v dividers %v gap %v; want [4 150 0 210], [2 3], 0", grid["widths"], grid["dividers"], grid["gap"])
	}
	if row["cardPadding"] != 0 || row["variant"] != "cards" || row["open"] == nil {
		t.Errorf("the row is not an opening card padded by its cells: %s", js(row))
	}
	c := cells(row)
	if len(c) != 4 {
		t.Fatalf("the row has %d cells, want 4: %s", len(c), js(c))
	}
	raw := js(row)
	for _, want := range []string{
		`"data-stripe":"running"`, `"bg":"orange.6"`, // the stripe
		`"content":"feeds.RQ-047"`, `"mono":true`, `"data-kind":"build"`, // id and kind
		`"lineClamp":2`, `"content":"feeds"`, `"content":"auto-review"`, `"content":"held by claude-b since 08:38"`, // title and meta
		`"variant":"dots"`, `"content":"4/6 ·"`, // the steps
		`"data-request":"feeds.RQ-047"`, `"data-state":"running"`, `"data-step-who":"auto-review · claude-b"`, `"data-group":"running"`,
	} {
		if !strings.Contains(raw, want) {
			t.Errorf("the held row lacks %s: %s", want, raw)
		}
	}

	// The meta line, case by case.
	for _, cs := range []struct {
		name string
		rq   paneRequest
		st   openStep
		has  []string
		not  []string
	}{
		{"on you", paneRequest{ID: "ui.RQ-101", Bucket: "wait", Steps: steps("done", "on-you")}, openStep{Step: "analyse", Ready: true, OnYou: true},
			[]string{`"data-stripe":"on-you"`, `"bg":"grape.6"`, `"content":"analyse"`, `"content":"on you"`}, nil},
		{"ready", paneRequest{ID: "feeds.F-027", Bucket: "todo", Kind: "feature", Steps: steps("done", "todo")}, openStep{Step: "review", Ready: true},
			[]string{`"data-stripe":"to-do"`, `"bg":"dark.2"`, `"content":"review"`, `"content":"ready"`, `"data-kind":"feature"`, `"color":"gray"`}, []string{"not started"}},
		{"not started", paneRequest{ID: "pane.RQ-013", Bucket: "todo", Steps: steps("todo", "todo", "todo")}, openStep{Step: "intake", Ready: true},
			[]string{`"content":"not started"`, `"content":"intake"`, `"content":"0/3 ·"`}, nil},
		{"waits on", paneRequest{ID: "feeds.F-032", Bucket: "waits", On: []string{"feeds.F-030", "wiki.F-020", "view.F-073"}, Steps: steps("done", "waiting")}, openStep{Step: "implement", Waits: true},
			[]string{`"data-stripe":"waiting"`, `"bg":"violet.7"`, `"content":"waits on"`, `"data-open":"feeds.F-030"`, `"data-open":"wiki.F-020"`, `"data-open":"view.F-073"`,
				`"data-step-who":"waits on feeds.F-030 wiki.F-020 view.F-073"`, `"content":"1/2 ·"`, `"content":"waiting"`}, []string{`"content":"waiting on"`, `"content":"waits"`}},
		{"blocked", paneRequest{ID: "m.RQ-9", Bucket: "todo", Steps: steps("done", "failed")}, openStep{Step: "analyse", Ready: true},
			[]string{`"data-stripe":"blocked"`, `"bg":"red.6"`}, nil},
		{"nothing left", paneRequest{ID: "m.RQ-8", Bucket: "idle", Steps: steps("done")}, openStep{},
			[]string{`"content":"nothing left - it should end"`, `"data-step-who":"nothing left - it should end"`}, []string{`"content":"ready"`}},
	} {
		r := js(requestRow("en", cs.rq, cs.st, rowOpts{}))
		for _, h := range cs.has {
			if !strings.Contains(r, h) {
				t.Errorf("%s: the row lacks %s: %s", cs.name, h, r)
			}
		}
		for _, n := range cs.not {
			if strings.Contains(r, n) {
				t.Errorf("%s: the row says %s: %s", cs.name, n, r)
			}
		}
	}

	// The dense row: its own widths, one title line, no kind tag, the module
	// left out; the Running card's and the Planned delivery panel's row is it.
	dense := requestRow("en", held, openStep{Step: "auto-review", Who: "claude-b"}, rowOpts{dense: true})
	if dw := dense["item"].(node)["widths"]; !reflect.DeepEqual(dw, []int{4, 140, 0, 190}) {
		t.Errorf("the dense row's widths %v, want [4 140 0 190]", dw)
	}
	if d := js(dense); !strings.Contains(d, `"lineClamp":1`) || strings.Contains(d, `"data-kind"`) || strings.Contains(d, `"content":"feeds"`) || strings.Contains(d, "4/6") ||
		!strings.Contains(d, `"content":"claude-b"`) {
		t.Errorf("the dense row: %s", d)
	}
	if js(runningRequest("en", held, openStep{Step: "auto-review", Who: "claude-b"})) != js(dense) {
		t.Error("the Running card's request is not the shared dense row")
	}
	under := func(r node) int { return len(cells(r)[3].(map[string]any)["children"].([]any)) }
	if named := requestRow("en", held, openStep{Step: "auto-review", Who: "claude-b"}, rowOpts{dense: true, named: true}); under(named) != 2 || under(dense) != 1 {
		t.Errorf("the goal's dense row names its step under the dots, the Running card's draws the dots alone: %s / %s", js(named), js(dense))
	}

	// The goal's dense row of a request waiting on others: « waits on » and
	// its chips, and under the dots its open step then « waits » (section 4:
	// « implement waits »), never « waiting waits ».
	waitsOn := paneRequest{ID: "wiki.F-020", Bucket: "waits", On: []string{"feeds.RQ-047"}, Steps: steps("done", "waiting", "todo", "todo")}
	goalRow := requestRow("en", waitsOn, openStep{Step: "implement", Waits: true}, rowOpts{dense: true, named: true})
	gu, _ := json.Marshal(cells(goalRow)[3].(map[string]any)["children"].([]any)[1])
	if g := string(gu); !strings.Contains(g, `"content":"implement"`) || !strings.Contains(g, `"content":"waits"`) || strings.Contains(g, `"content":"waiting"`) {
		t.Errorf("the goal's waiting row under its dots: %s; want « implement waits »", g)
	}
	if g := js(goalRow); !strings.Contains(g, `"content":"waits on"`) || !strings.Contains(g, `"data-open":"feeds.RQ-047"`) {
		t.Errorf("the goal's waiting row's meta line: %s; want « waits on » and its chip", g)
	}

	// The open requests in their groups, counted as the head, the done ones
	// folded last.
	p := paneData{requests: map[string]paneRequest{}}
	for _, rq := range []paneRequest{
		{ID: "m.RQ-1", Bucket: "todo", Steps: steps("todo")},
		{ID: "m.RQ-2", Bucket: "run", Steps: steps("running")},
		{ID: "m.RQ-3", Bucket: "waits", On: []string{"m.RQ-1"}, Steps: steps("done", "waiting")},
		{ID: "m.RQ-4", Bucket: "wait", Steps: steps("on-you")},
		{ID: "m.RQ-5", Bucket: "block", Steps: steps("blocked")},
		{ID: "m.RQ-6", Bucket: "done", Steps: steps("done")},
		{ID: "m.RQ-7", Bucket: "idle", Steps: steps("done")},
		{ID: "m.RQ-10", Bucket: "run", Steps: steps("running")},
	} {
		p.requests[rq.ID] = rq
		p.order = append(p.order, rq.ID)
	}
	list := js(p.openRequestList(context.Background()))
	var tree struct {
		Children []struct {
			Kind   string           `json:"kind"`
			View   string           `json:"data-view"`
			Look   string           `json:"look"`
			Panels []map[string]any `json:"panels"`
			Def    []string         `json:"defaultOpen"`
		} `json:"children"`
	}
	_ = json.Unmarshal([]byte(list), &tree)
	var groups []string
	sum, folded := 0, false
	for _, ch := range tree.Children {
		switch ch.View {
		case "increment-open-groups":
			if ch.Look != "rule" || len(ch.Def) != len(ch.Panels) {
				t.Errorf("the groups are not rule folds all open: %s", list)
			}
			for _, pn := range ch.Panels {
				groups = append(groups, pn["data-group"].(string))
				sum += int(pn["badge"].(float64))
			}
		case doneRequestsFold:
			folded = ch.Look == "rule" && len(ch.Def) == 0 && len(ch.Panels) == 1 && ch.Panels[0]["label"] == "1 done" && ch.Panels[0]["color"] == nil
		}
	}
	if strings.Join(groups, ",") != "running,on-you,blocked,waiting,to-do" {
		t.Errorf("the groups %v; want running, on-you, blocked, waiting, to-do", groups)
	}
	if sum != p.counts().open() || sum != 7 {
		t.Errorf("the groups count %d, the head %d; want 7 each", sum, p.counts().open())
	}
	if !folded {
		t.Errorf("the done requests are not one quiet fold, folded, last: %s", list)
	}
	if i, j := strings.Index(list, `"data-request":"m.RQ-7"`), strings.Index(list, `"data-group":"to-do"`); i < 0 || j < 0 || i < j {
		t.Errorf("nothing-left is not under To do: %s", list)
	}
}
