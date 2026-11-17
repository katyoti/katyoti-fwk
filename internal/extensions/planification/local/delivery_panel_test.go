// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	"github.com/katyoti/katyoti-fwk/internal/test/long"
)

// THE PLANNED DELIVERY PANEL ON ITS OPEN REQUESTS (view 3
// of delivery-views-v2-mockup.html): the increment picked shows its id, priority and release, its title, « n open ·
// n done · n waits on you », its first open requests with their step dots in
// the Running tab's order, « + n more » opening its Open requests tab, « Open
// the increment › » and « Put on hold » - never its goals.
func TestPlannedDeliveryPanelShowsOpenRequests(t *testing.T) {
	long.Test(t)
	t.Parallel()
	w := runningIncrementsWorld(t)
	inc := increments(t, w.ctx)
	panelIn := func(id, lang string) (string, map[string]any) {
		t.Helper()
		def, err := inc.PanelPage(w.ctx, api.IncrementPanelInput{ID: id, Language: lang})
		if err != nil {
			t.Fatal(err)
		}
		if err := def.Validate(); err != nil {
			t.Fatalf("the panel must display: %v", err)
		}
		b, _ := json.Marshal(def.DSLTree)
		var tree map[string]any
		_ = json.Unmarshal(b, &tree)
		return string(b), tree
	}
	panel := func(id string) (string, map[string]any) {
		t.Helper()
		return panelIn(id, "")
	}

	on, tree := panel(w.built)
	for _, want := range []string{
		`"data-view":"increment-panel"`, `"content":"` + w.built + ` · P2 · 1.1.0"`, `"content":"Invoices get paid"`,
		`"content":"6 open · 1 done · 1 waits on you"`, `"data-view":"increment-panel-requests"`,
		`"kind":"stepper"`, `"text":"+ 3 more"`, `"panel":"tab:increment-open-requests"`,
		`"label":"Open the increment ›"`, `"data-control":"open-increment"`, `"navigate":{"object":"` + w.built + `"}`,
		`"label":"Put on hold"`, `"data-control":"hold"`, `"uri":"ktw-agents:increments/board-move"`,
	} {
		if !strings.Contains(on, want) {
			t.Errorf("the panel lacks %s: %s", want, on)
		}
	}
	for _, not := range []string{"Where each goal stands", "What Invoices get paid gives", `"kind":"select"`,
		`"data-request":"` + w.reqs["done"] + `"`, `"data-request":"` + w.reqs["dropped"] + `"`} {
		if strings.Contains(on, not) {
			t.Errorf("the panel shows %s: never its goals, a drop-down, a done or dropped request", not)
		}
	}
	// Its first three open requests, in the Running tab's order.
	var rows []string
	for _, l := range (runningTab{tree: map[string]any{"children": tree["children"]}}).lines() {
		if strings.HasPrefix(l, "RQ:") {
			rows = append(rows, strings.SplitN(l, ":", 3)[1])
		}
	}
	want := []string{w.reqs["running"], w.reqs["on-you"], w.reqs["failed"]}
	if strings.Join(rows, " ") != strings.Join(want, " ") {
		t.Errorf("the panel's requests:\n got %v\nwant %v (running, on you, failed: the Running tab's order, three of them)", rows, want)
	}

	// Held: Resume in place of Put on hold, and why it waits.
	held, _ := panel(w.held)
	for _, want := range []string{`"data-control":"resume"`, "ktw-agents:increments/resume", "on hold: waits on legal", `"content":"1 open · 0 done · 0 waits on you"`} {
		if !strings.Contains(held, want) {
			t.Errorf("the held increment's panel lacks %s: %s", want, held)
		}
	}
	if strings.Contains(held, `"data-control":"hold"`) || strings.Contains(held, `"data-more"`) {
		t.Errorf("the held panel offers Put on hold, or « + n more » with every open request shown: %s", held)
	}

	// In French: every word of the panel is French
	// none of its English words is left.
	fr, _ := panelIn(w.built, "fr")
	for _, want := range []string{`"content":"6 ouvertes · 1 faite · 1 à vous"`, `"text":"+ 3 de plus"`,
		`"label":"Ouvrir l'incrément ›"`, `"label":"Suspendre"`, "Le suspendre", "Pourquoi il attend", `"label":"Ouvrir `} {
		if !strings.Contains(fr, want) {
			t.Errorf("the French panel lacks %s: %s", want, fr)
		}
	}
	frHeld, _ := panelIn(w.held, "fr")
	for _, want := range []string{"suspendu : waits on legal", `"label":"Reprendre"`, `"content":"1 ouverte · 0 faite · 0 à vous"`} {
		if !strings.Contains(frHeld, want) {
			t.Errorf("the French held panel lacks %s: %s", want, frHeld)
		}
	}
	// An increment with no open request: the French empty word, singular counts.
	frNone, _ := panelIn(w.defined, "fr")
	for _, want := range []string{`"content":"0 ouverte · 0 faite · 0 à vous"`, `"content":"Aucune demande ouverte."`} {
		if !strings.Contains(frNone, want) {
			t.Errorf("the French panel of an increment with no open request lacks %s: %s", want, frNone)
		}
	}
	// In English, the counts keep « open » and « done » whatever the number.
	if none, _ := panelIn(w.defined, "en"); !strings.Contains(none, `"content":"0 open · 0 done · 0 waits on you"`) || !strings.Contains(none, "No open request.") {
		t.Errorf("the English panel of an increment with no open request: %s", none)
	}
	for _, en := range []string{" open · ", " waits on you", " more", "Open the increment", "Put on hold", "Put it on hold", "Why it waits", "on hold:", "No open request", `"Resume"`, `"label":"Open `, "ouvertes · 1 faites", "ouvertes · 0 faites"} {
		if strings.Contains(fr+frHeld+frNone, en) {
			t.Errorf("the French panel keeps the English %q", en)
		}
	}

	// The side region's empty text, and the panel's load, follow the language.
	def, err := w.sm.DeliveryPage(w.ctx, api.DeliveryPageInput{Target: "billing", Language: "fr"})
	if err != nil {
		t.Fatal(err)
	}
	page, _ := json.Marshal(def)
	for _, want := range []string{"Choisissez un incrément à gauche pour voir ses demandes ouvertes.", `"language":"fr"`,
		"Cliquez sur une version de la frise pour voir ce qu'elle livre et ce qui la retient.",
		"Chaque flux sur une frise, avec son mode de livraison",
		`"label":"Livraison planifiée"`, `"label":"En cours"`, `"label":"Incréments ouverts"`} {
		if !strings.Contains(string(page), want) {
			t.Errorf("the French Delivery view lacks %s", want)
		}
	}
	deliveryEnglish := []string{"Pick an increment on the left", "Click a release on the timeline", "Every stream on a timeline",
		`"label":"Planned delivery"`, `"label":"Running"`, `"label":"Open increments"`, "No stream yet"}
	for _, en := range deliveryEnglish {
		if strings.Contains(string(page), en) {
			t.Errorf("the French Delivery view keeps the English %q", en)
		}
	}
	// With no stream at all, its one line is French too.
	ectx, _, _ := world(t)
	def, err = streams(t, ectx).DeliveryPage(ectx, api.DeliveryPageInput{Language: "fr"})
	if err != nil {
		t.Fatal(err)
	}
	empty, _ := json.Marshal(def)
	if !strings.Contains(string(empty), "Aucun flux pour l'instant : créez-en un (streams create) pour voir ici ses versions et son backlog.") {
		t.Errorf("the French Delivery view with no stream lacks its French line: %s", empty)
	}
	for _, en := range deliveryEnglish {
		if strings.Contains(string(empty), en) {
			t.Errorf("the French Delivery view with no stream keeps the English %q", en)
		}
	}

	// The release region below the timeline: its load
	// carries the language, its title is French, and every word the release
	// level draws - its sections, the header's due date, next step and next
	// release, the legend, what needs you - is French, singular for one.
	if def.Title != "Livraison" || !strings.Contains(string(page), `"id":"${selection.release}","language":"fr"`) {
		t.Errorf("the French Delivery view: title %q, its release region's load must carry the language: %s", def.Title, page)
	}
	rel := releases(t, w.ctx)
	level := func(id, lang string) string {
		t.Helper()
		def, err := rel.RowsPage(w.ctx, api.ReleaseRowsPageInput{ID: id, Language: lang})
		if err != nil {
			t.Fatal(err)
		}
		if err := def.Validate(); err != nil {
			t.Fatalf("the release level must display: %v", err)
		}
		b, _ := json.Marshal(def)
		return string(b)
	}
	frCurrent, frLater := level(w.current, "fr"), level(w.later, "fr")
	for _, want := range []string{"Livré dans cette version (1)", `"next_label":"Terminer 1 incrément, puis la geler"`,
		`"due":"échéance 2099-10-31 · `, " jours restants", " · la prochaine version du flux", `"needs_you_text":"à vous"`,
		`{"label":"fait","word":"done"}`, `{"label":"en cours","word":"running"}`, `{"label":"en attente","word":"waiting"}`,
		`{"label":"à faire","word":"todo"}`, `{"label":"bloqué","word":"blocked"}`} {
		if !strings.Contains(frCurrent, want) {
			t.Errorf("the French release level lacks %s: %s", want, frCurrent)
		}
	}
	for _, want := range []string{"Rien n'y est encore livré.", "Prévu pour cette version (1) — il y est livré dès qu'elle est la prochaine version du flux"} {
		if !strings.Contains(frLater, want) {
			t.Errorf("the French later release's level lacks %s: %s", want, frLater)
		}
	}
	// Its gate reasons, Change release's options and what each row waits on
	//
	for _, want := range []string{`"why":"attend une version candidate"`, `"why":"ouverts : Invoices get paid"`, ` — prévu pour"`} {
		if !strings.Contains(frCurrent, want) {
			t.Errorf("the French release level lacks %s: %s", want, frCurrent)
		}
	}
	for _, en := range []string{"Ships in this release", "Nothing ships", "Planned for this release", "Finish ", "then Freeze", `"due":"due `,
		"days left", "days late", "the stream's next release", "needs you", "Ship the ",
		"needs a candidate", "no candidate delivered", `"why":"open: `, " — planned for", " — ships in", "on hold: ", `"waits":"waits on `,
		" is blocked: ", "awaits a person", "no work planned", "its proposal"} {
		if strings.Contains(frCurrent+frLater, en) {
			t.Errorf("the French release level keeps the English %q", en)
		}
	}
}
