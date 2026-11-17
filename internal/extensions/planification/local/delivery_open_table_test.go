// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// DELIVERY'S « OPEN INCREMENTS » AS ONE TABLE (view 1A of
// delivery-views-v2-mockup.html): the tab is the search box (id, title) and
// the stream and held filters over a region
// reading the table's own page - one line per open increment in no release
// (id, title, held tag, description, stream; priority, Hold… or Resume, ships
// in at the end of the line, menus and buttons, no inline drop-down), sorted by
// priority then rank, the first lines then « Afficher plus »; no board.
func TestOpenIncrementsIsOneTable(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	sm, rel, inc := streams(t, ctx), releases(t, ctx), increments(t, ctx)
	product, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: "Product", StreamFields: api.StreamFields{VersionScheme: api.SchemeSemver, ReleaseMode: api.ModeManual}})
	if err != nil {
		t.Fatal(err)
	}
	p := product.Stream.ID
	next := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.1.0", Date: "2027-01-01", Stream: p})
	later := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.2.0", Date: "2027-02-01", Stream: p})
	open := func(title, description, priority string, stream bool) string {
		t.Helper()
		if !stream {
			// One in no stream is older data.
			return strayIncrement(t, ctx, cwm, "billing", title)
		}
		out, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: p, Title: title, Description: description, By: person})
		if err != nil {
			t.Fatal(err)
		}
		id := out.Increment.ID
		if priority != "" {
			if r, err := inc.BoardMove(ctx, api.BoardMoveInput{ID: id, Column: priority, By: person}); err != nil || !r.OK {
				t.Fatalf("priority %s: %+v %v", priority, r, err)
			}
		}
		return id
	}
	low := open("Reminders", "Remind the late payers.", "P3", true)
	top := open("Invoices by mail", "Send each invoice by mail, the PDF attached.", "P0", true)
	mid := open("Payment links", "", "P1", true)
	held := open("Bank feeds", "Read the bank's statements.", "P1", true)
	loose := open("A loose idea", "", "", false)
	shipped := open("Already shipping", "", "P0", true)
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: shipped, In: next.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.PlanFor(ctx, api.PlanForInput{ID: mid, In: later.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	if r, err := inc.BoardMove(ctx, api.BoardMoveInput{ID: held, Column: "held", Reason: "waits for the bank", By: person}); err != nil || !r.OK {
		t.Fatalf("held: %+v %v", r, err)
	}

	tabIn := func(lang string) (string, any) {
		t.Helper()
		def, err := sm.DeliveryPage(ctx, api.DeliveryPageInput{Target: "billing", Language: lang})
		if err != nil {
			t.Fatal(err)
		}
		if err := def.Validate(); err != nil {
			t.Fatalf("the Delivery view must display: %v", err)
		}
		var tree struct {
			Tabs []struct {
				ID      string          `json:"id"`
				Content json.RawMessage `json:"content"`
			} `json:"tabs"`
		}
		raw, _ := json.Marshal(def.DSLTree)
		_ = json.Unmarshal(raw, &tree)
		for _, tb := range tree.Tabs {
			if tb.ID == "open" {
				return string(tb.Content), def.Data
			}
		}
		t.Fatal("no Open increments tab")
		return "", nil
	}
	tab := func() (string, any) { t.Helper(); return tabIn("") }

	on, data := tab()
	if strings.Contains(on, `"kind":"grid-board"`) {
		t.Error("the Open increments tab is still the board")
	}
	for _, want := range []string{`"kind":"chips"`, `"key":"open_stream"`, `"value":"all"`, `"label":"All streams"`, `"value":"` + p + `"`,
		`"key":"open_held"`, `"value":"not-held"`, `"label":"Not held"`, `"value":"held"`, `"label":"Held"`, "sorted by priority, then rank",
		`"id":"open-increments"`, `"method":"ktw-agents:increments/open-page"`, `"stream":"${selection.open_stream}"`, `"held":"${selection.open_held}"`,
		`"shown":"${selection.open_shown}"`, `"load":{"method"`,
		`"kind":"search"`, `"key":"open_search"`, `"placeholder":"Search id, title…"`, `"selection":"${selection}"`} {
		if !strings.Contains(on, want) {
			t.Errorf("the Open increments tab lacks %s", want)
		}
	}
	// The search box comes first in the toolbar, before the stream and held
	// chips (mock-up view 1A).
	toolbar := openToolbar(t, on)
	if s, c := strings.Index(toolbar, `"key":"open_search"`), strings.Index(toolbar, `"key":"open_stream"`); s < 0 || c < 0 || s > c {
		t.Errorf("the search box is not first in the toolbar, before the chips: %s", toolbar)
	}
	if fr, _ := tabIn("fr"); !strings.Contains(fr, `"placeholder":"Chercher id, titre…"`) {
		t.Errorf("the French tab lacks its search placeholder: %s", fr)
	}
	sel, _ := json.Marshal(data)
	for _, want := range []string{`"open_search":""`, `"open_stream":"all"`, `"open_held":"not-held"`, `"open_shown":25`} {
		if !strings.Contains(string(sel), want) {
			t.Errorf("the page's selection %s lacks %s", sel, want)
		}
	}

	page := func(in api.OpenIncrementsInput) string {
		t.Helper()
		def, err := inc.OpenPage(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		if err := def.Validate(); err != nil {
			t.Fatalf("the table must display: %v", err)
		}
		b, _ := json.Marshal(def.DSLTree)
		return string(b)
	}
	// Not held, every stream: by priority then rank; held and shipping ones out.
	rows := page(api.OpenIncrementsInput{Stream: "all", Held: "not-held", Shown: 25})
	// Each increment is the planification card: a list
	// of cards, two by row, each with its tone edge, its controls at its foot.
	for _, want := range []string{`"kind":"list"`, `"layout":"grid"`, `"columns":2`, `"nodeField":"card"`, `"toneField":"tone"`, `"tone":"blue"`, `"data-view":"card-actions"`,
		"Send each invoice by mail, the PDF attached.", "Remind the late payers.", `"text":"Product"`, `"text":"No stream"`,
		`"data-control":"priority"`, `"choice":"P0|P1|P2|P3"`, `"uri":"ktw-agents:requests/assess"`,
		`"data-control":"hold"`, "Hold…", `"data-control":"ships-in"`, `"uri":"ktw-agents:increments/board-ship"`,
		`"label":"1.1.0 (next)"`, `"label":"— No release"`, `"value":"out"`, `"label":"1.2.0"`, `"data-control":"stream"`} {
		if !strings.Contains(rows, want) {
			t.Errorf("the table lacks %s", want)
		}
	}
	// The title text alone carries data-title (the search matches id and
	// title only; a GUI probe reads it, never the description).
	if !strings.Contains(rows, `"content":"Invoices by mail","data-title":"`+top+`"`) ||
		strings.Contains(rows, `"content":"Send each invoice by mail, the PDF attached.","data-title"`) {
		t.Errorf("the title text, and it alone, carries data-title: %s", rows)
	}
	if strings.Contains(rows, `"kind":"select"`) {
		t.Error("a line carries an inline drop-down: priority and release are menus (ops.RQ-307)")
	}
	for _, not := range []string{`"data-increment":"` + held + `"`, `"data-increment":"` + shipped + `"`} {
		if strings.Contains(rows, not) {
			t.Errorf("the not-held table shows %s", not)
		}
	}
	order := []int{strings.Index(rows, `"data-increment":"`+top+`"`), strings.Index(rows, `"data-increment":"`+mid+`"`),
		strings.Index(rows, `"data-increment":"`+loose+`"`), strings.Index(rows, `"data-increment":"`+low+`"`)}
	for n := range order {
		if order[n] < 0 || (n > 0 && order[n] < order[n-1]) {
			t.Fatalf("the lines are not P0, P1, P2 (the loose one's default), P3: %v", order)
		}
	}

	// Held: only the held one, its tag and Resume.
	h := page(api.OpenIncrementsInput{Stream: "all", Held: "held", Shown: 25})
	if !strings.Contains(h, `"data-increment":"`+held+`"`) || !strings.Contains(h, `"data-held":"`+held+`"`) || !strings.Contains(h, `"data-control":"resume"`) ||
		!strings.Contains(h, "ktw-agents:increments/resume") || strings.Contains(h, `"data-increment":"`+top+`"`) {
		t.Errorf("the held table: %s", h)
	}
	// A stream's, and none: the increments in no stream.
	if s := page(api.OpenIncrementsInput{Stream: "none", Held: "not-held", Shown: 25}); !strings.Contains(s, `"data-increment":"`+loose+`"`) || strings.Contains(s, `"data-increment":"`+top+`"`) {
		t.Errorf("the no-stream table: %s", s)
	}
	if s := page(api.OpenIncrementsInput{Stream: p, Held: "not-held", Shown: 25}); strings.Contains(s, `"data-increment":"`+loose+`"`) || !strings.Contains(s, `"data-increment":"`+top+`"`) {
		t.Errorf("the stream's table: %s", s)
	}
	// The first lines, then « … n more » and Show more asking the next page.
	two := page(api.OpenIncrementsInput{Stream: "all", Held: "not-held", Shown: 2})
	if strings.Contains(two, `"data-increment":"`+loose+`"`) || !strings.Contains(two, "… 2 more · 25 per page") ||
		!strings.Contains(two, `"select":{"key":"open_shown","value":27}`) {
		t.Errorf("two lines then Show more: %s", two)
	}
	if fr := page(api.OpenIncrementsInput{Stream: "all", Held: "not-held", Shown: 2, Language: "fr"}); !strings.Contains(fr, "Afficher plus") || !strings.Contains(fr, "Suspendre…") {
		t.Errorf("the French table: %s", fr)
	}
	if all := page(api.OpenIncrementsInput{Stream: "all", Held: "not-held", Shown: 25}); strings.Contains(all, "open-increments-more") {
		t.Error("Show more with every line shown")
	}

	// The search: part of a title, another case, keeps
	// only its lines; part of an id too.
	has := func(s, id string) bool { return strings.Contains(s, `"data-increment":"`+id+`"`) }
	if s := page(api.OpenIncrementsInput{Stream: "all", Held: "not-held", Search: "INVOICE", Shown: 25}); !has(s, top) || has(s, mid) || has(s, low) || has(s, loose) {
		t.Errorf("a search on part of a title keeps only its line: %s", s)
	}
	if s := page(api.OpenIncrementsInput{Stream: "all", Held: "not-held", Search: " " + strings.ToLower(low) + " ", Shown: 25}); !has(s, low) || has(s, top) || has(s, mid) {
		t.Errorf("a search on an id keeps only its line: %s", s)
	}
	// It comes before the paging: « … n more » follows it (« m » is in three
	// not-held titles - Invoices by mail, Payment links, Reminders).
	if s := page(api.OpenIncrementsInput{Stream: "all", Held: "not-held", Search: "m", Shown: 1}); !has(s, top) || !strings.Contains(s, "… 2 more · 25 per page") {
		t.Errorf("the search narrows the lines before the paging: %s", s)
	}
	// It keeps the other filters: a held line stays out of the not-held table.
	if s := page(api.OpenIncrementsInput{Stream: "all", Held: "not-held", Search: "bank", Shown: 25}); has(s, held) || strings.Count(s, "No open increment here.") != 1 || !strings.Contains(s, `"empty":"No open increment here."`) || strings.Contains(s, "open-increments-more") {
		t.Errorf("a search matching nothing shows the empty text once - the list's own - and no more-row: %s", s)
	}
}

// openToolbar is the Open increments tab's toolbar node, as JSON.
func openToolbar(t *testing.T, tab string) string {
	t.Helper()
	var root any
	if err := json.Unmarshal([]byte(tab), &root); err != nil {
		t.Fatal(err)
	}
	var find func(n any) any
	find = func(n any) any {
		switch v := n.(type) {
		case map[string]any:
			if v["data-view"] == "open-increments-toolbar" {
				return v
			}
			for _, c := range v {
				if f := find(c); f != nil {
					return f
				}
			}
		case []any:
			for _, c := range v {
				if f := find(c); f != nil {
					return f
				}
			}
		}
		return nil
	}
	n := find(root)
	if n == nil {
		t.Fatal("no open-increments-toolbar in the tab")
	}
	b, _ := json.Marshal(n)
	return string(b)
}
