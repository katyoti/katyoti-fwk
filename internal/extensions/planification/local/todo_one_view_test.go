// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// « À FAIRE » IS THE PREVIOUS VIEW, EVERY WAITING ITEM IN IT
// (the person's choice 2026-11-02): planification's
// « À faire » page keeps every element it had -
// its tiles, a section per action « <Action> — n », the release › increment
// lines, its cards (p md, the title, the id / module / request badges, the
// question, « Open <request> ») - and lists the inbox's human tasks too, each
// a card of the same page: its kind, what it is about and that object's
// title, its question, a choice's options before the pick, who asks when it
// is about nothing, the inbox's own answer forms, its element id; they count.
func TestÀFaireIsThePreviousViewWithEveryWaitingItem(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindBuild, Review: api.ReviewPerson,
		Deliverables: []api.Deliverable{{Title: "it builds", Check: "go version"}}})
	toReview(t, ctx, r.ID)
	views := views(t, ctx)
	page := func() string {
		t.Helper()
		def, err := views.PersonQueuePage(ctx, api.PersonQueueInput{Language: "en"})
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(def)
		return string(raw)
	}
	count := func() int {
		t.Helper()
		c, err := views.PersonQueueCount(ctx, api.PersonQueueInput{})
		if err != nil {
			t.Fatal(err)
		}
		return c.Count
	}
	countOff := count()

	// Two human tasks in the inbox: a choice about the request, a validation about nothing.
	put := func(key, title string, ask map[string]any) {
		t.Helper()
		ask["title"] = title
		if _, err := schedOf(t, ctx).Put(ctx, schedapi.PutInput{Todo: schedapi.Todo{
			Key: key, Module: "billing", Target: "billing", Title: title, For: schedapi.ForPerson, Start: "manual", Priority: "P1",
			State: schedapi.StateReady, Owner: inboxapi.Owner, Work: map[string]any{"task": inboxapi.HumanTaskStrategy, inboxapi.WorkAsk: ask},
		}}); err != nil {
			t.Fatal(err)
		}
	}
	put("ht-choose", "Which space?", map[string]any{"kind": inboxapi.KindChoose, "about": r.ID, "question": "Where the pages go",
		"options": []any{"ARCHI", "ARCHI-TEST"}})
	put("ht-approve", "Validate the export", map[string]any{"kind": inboxapi.KindApprove, "question": "The export of 12 pages", "asked_by": "agent:reviewer"})
	// A bare one - no about, no question, no asker (defect 1).
	put("ht-bare", "Test the export", map[string]any{"kind": inboxapi.KindTestByHand})
	// A permission, its decision and rule in the person's language (defect 2).
	put("ht-perm", "May it push?", map[string]any{"kind": inboxapi.KindPermission, "about": r.ID, "question": "git push to origin", "options": []any{"git push *"}})
	// A gate's confirmation, filed in the inbox by a planification asker no step carries.
	gate, err := inboxOf(t, ctx).File(ctx, inboxapi.FileInput{Title: "Confirm: change log", RequestedBy: "planification:gate:ready-to-release/change-log@1",
		Expects: inboxapi.ExpectsVerdict, About: r.ID})
	if err != nil {
		t.Fatal(err)
	}

	on := page()
	// Every element of the previous view, kept.
	for _, want := range []string{
		`"kind":"kpi"`, `"label":"All"`, "Validate — ", "Decide — ", "In no release › ", `"p":"md"`, `"fw":600`,
		`"mono":true`, `"text":"` + r.ID + `"`, `"text":"billing"`, "Open " + r.ID, `"kind":"heading"`,
	} {
		if !strings.Contains(on, want) {
			t.Errorf("the previous view lacks %q", want)
		}
	}
	// The inbox's questions, cards of the same page with what was missing.
	for _, want := range []string{
		"Which space?", "Where the pages go", `"text":"Choice"`, // its question and its kind
		`"color":"dimmed","content":"Print invoices","kind":"text","size":"sm"`, // the title of what it is about
		"The options", "- ARCHI", "- ARCHI-TEST", // its options before the pick
		"ktw-agents:todos/report", `"id":"answer:billing.TD`, // answered in place by the inbox's forms
		`"id":"inbox:billing.TD`,                                                   // its element id
		"Validate the export", "The export of 12 pages", "Asked by agent:reviewer", // about nothing: who asks
		`"id":"approve:billing.TD`,                                           // a validation's buttons
		`"data-section":"decide"`, `"data-tile":"validate"`, `"data-count":`, // the walks' addresses
		"Confirm: change log", `"id":"inbox:` + gate.Request.ID + `"`, `"id":"approve:` + gate.Request.ID + `"`, // a gate's confirmation, its verdict buttons
	} {
		if !strings.Contains(on, want) {
			t.Errorf("an inbox card lacks %q", want)
		}
	}
	if strings.Index(on, "Which space?") < strings.Index(on, "Decide — ") {
		t.Error("the choice is not under Decide")
	}
	if got := count(); got != countOff+5 {
		t.Errorf("count = %d; want %d (the four human tasks and the confirmation counted)", got, countOff+5)
	}

	// Defect 1: a card that says nothing says what its action asks, who asks
	// it (else its module) and since when - never its title alone.
	bare := cardText(t, on, "Test the export")
	for _, want := range []string{"What no agent can test", "Asked by billing", "since 20"} {
		if !strings.Contains(bare, want) {
			t.Errorf("the bare card lacks %q: %s", want, bare)
		}
	}
	// Defect 3: putting on hold is a compact button asking why in a modal,
	// not a form open on every card.
	if !strings.Contains(on, `"id":"hold:`) || !strings.Contains(on, `"id":"hold-why:`) {
		t.Error("the hold is not a compact button with its modal")
	}
	if strings.Count(on, `"root":"PutOnHold"`) != strings.Count(on, `"id":"hold-why:`) {
		t.Error("a hold form is open on a card, outside its modal")
	}
	// Defect 2: in French no English is left in the answer forms.
	def, err := views.PersonQueuePage(ctx, api.PersonQueueInput{Language: "fr"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(def)
	fr := string(raw)
	for _, english := range []string{"Your choice", "Allow or deny the act", "The rule an always keeps", "Why it waits", "Your decision", "Your note", "Your answer", "What you saw", "What no agent can test"} {
		if strings.Contains(fr, english) {
			t.Errorf("the French page carries %q", english)
		}
	}
	// A hint never only repeats its field's label.
	for _, twice := range []string{`"description":"Votre choix"`, `"description":"Votre réponse"`, `"description":"Pourquoi ?"`} {
		if strings.Contains(fr, twice) {
			t.Errorf("a French field's hint repeats its label: %s", twice)
		}
	}
	for _, french := range []string{"Autoriser ou refuser l'acte", "La règle qu'un « toujours » garde", "Pourquoi il attend", "Demandé par billing", "Ce qu'aucun agent ne peut tester"} {
		if !strings.Contains(fr, french) {
			t.Errorf("the French page lacks %q", french)
		}
	}
}

// cardText is the JSON of the card on a page whose title is title.
func cardText(t *testing.T, page, title string) string {
	t.Helper()
	at := strings.Index(page, `"content":"`+title+`"`)
	if at < 0 {
		t.Fatalf("no card titled %q", title)
	}
	start := strings.LastIndex(page[:at], `{"children"`)
	end := strings.Index(page[at:], `"kind":"card"`)
	if start < 0 || end < 0 {
		return page[at:]
	}
	return page[start : at+end]
}

// TestTheChatsCardIsÀFairesCard: one frame
// for one human task - the card a chat shows for a question
// (inboxapi.HumanTaskCard, which planification sets) is exactly the card « À
// faire » draws for it, answer area included; answered, it says who answered
// and what in place of its answer area, with nothing left to answer.
func TestTheChatsCardIsÀFairesCard(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindFix})
	if _, err := schedOf(t, ctx).Put(ctx, schedapi.PutInput{Todo: schedapi.Todo{
		Key: "ht-choose", Module: "billing", Target: "billing", Title: "Which space?", For: schedapi.ForPerson, Start: "manual", Priority: "P1",
		State: schedapi.StateReady, Owner: inboxapi.Owner, Work: map[string]any{"task": inboxapi.HumanTaskStrategy, inboxapi.WorkAsk: map[string]any{
			"kind": inboxapi.KindChoose, "title": "Which space?", "about": r.ID, "question": "Where the pages go", "options": []any{"ARCHI", "ARCHI-TEST"}}},
	}}); err != nil {
		t.Fatal(err)
	}
	if inboxapi.HumanTaskCard == nil {
		t.Fatal("planification sets no human task card for the chat")
	}
	waiting, err := application.UseService[inboxapi.WaitingService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	out, err := waiting.List(ctx, inboxapi.WaitingInput{Language: "en", Answers: true})
	if err != nil {
		t.Fatal(err)
	}
	var item inboxapi.WaitingItem
	for _, g := range out.Groups {
		for _, it := range g.Items {
			if it.Title == "Which space?" {
				item = it
			}
		}
	}
	if item.Key == "" {
		t.Fatal("the choice is not in the waiting list")
	}
	def, err := views(t, ctx).PersonQueuePage(ctx, api.PersonQueueInput{Language: "en"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(def.DSLTree)
	var tree any
	_ = json.Unmarshal(raw, &tree)
	inPage := nodeByID(tree, item.Key)
	if inPage == nil {
		t.Fatalf("« À faire » has no card %s", item.Key)
	}
	// Both as the host receives them: through JSON - the page's forms come
	// resolved, so the chat's card is compared
	// resolved the same way.
	var chat any
	rawChat, _ := json.Marshal(inboxapi.HumanTaskCard(ctx, item, "", "en"))
	_ = json.Unmarshal(rawChat, &chat)
	if chat, err = render.ResolveComponents(chat, render.ComponentResolver{}); err != nil {
		t.Fatal(err)
	}
	inChat, _ := json.Marshal(chat)
	want, _ := json.Marshal(inPage)
	if string(inChat) != string(want) {
		t.Errorf("the chat's card is not « À faire »'s:\nchat     %s\nÀ faire  %s", inChat, want)
	}
	answered, _ := json.Marshal(inboxapi.HumanTaskCard(ctx, item, "answered by jdoe · ARCHI", "en"))
	if a := string(answered); !strings.Contains(a, "answered by jdoe · ARCHI") || strings.Contains(a, "ktw-agents:todos/report") || !strings.Contains(a, "Which space?") {
		t.Errorf("the answered card does not keep what it asked and say who answered, with nothing left to answer: %s", a)
	}
}

// nodeByID is the node of a tree with that id.
func nodeByID(v any, id string) map[string]any {
	switch n := v.(type) {
	case []any:
		for _, c := range n {
			if got := nodeByID(c, id); got != nil {
				return got
			}
		}
	case map[string]any:
		if n["id"] == id {
			return n
		}
		for _, c := range n {
			if got := nodeByID(c, id); got != nil {
				return got
			}
		}
	}
	return nil
}

// failingWaiting is an inbox whose waiting list cannot be read.
type failingWaiting struct{ inboxapi.WaitingService }

func (failingWaiting) List(context.Context, inboxapi.WaitingInput) (inboxapi.WaitingOutput, error) {
	return inboxapi.WaitingOutput{}, errors.New("the inbox's index is locked")
}

// TestÀFaireSaysWhenTheInboxsQuestionsCannotBeRead:
// when the inbox's waiting list cannot be read « À faire » still lists its
// own items and says, at its top, that the inbox's questions are not listed
// and why - never silently fewer.
func TestÀFaireSaysWhenTheInboxsQuestionsCannotBeRead(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindBuild, Review: api.ReviewPerson,
		Deliverables: []api.Deliverable{{Title: "it builds", Check: "go version"}}})
	toReview(t, ctx, r.ID)
	application.PutService[inboxapi.WaitingService](application.AppFromContext(ctx), "default", failingWaiting{})
	page := func(lang string) string {
		t.Helper()
		def, err := views(t, ctx).PersonQueuePage(ctx, api.PersonQueueInput{Language: lang})
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(def)
		return string(raw)
	}
	on := page("en")
	if !strings.Contains(on, "The inbox's questions could not be read: the inbox's index is locked") || !strings.Contains(on, "Validate — ") {
		t.Errorf("the page does not say the inbox's questions were not read, or lost its own items: %s", on)
	}
	if fr := page("fr"); !strings.Contains(fr, "Les questions de la boîte de réception n'ont pas pu être lues") {
		t.Error("the French page does not say it in French")
	}
	q, err := views(t, ctx).PersonQueue(ctx, api.PersonQueueInput{})
	if err != nil || q.Unread == "" || q.Count == 0 {
		t.Errorf("the queue = %+v, %v; want its own items and why the inbox's were not read", q, err)
	}
}

// TestÀFaireKeepsWhatTheInboxsPageShowed: « À faire »
// - the previous view, its layout untouched - also
// shows what the inbox's page showed: (1) on a card, by when (late in red),
// the proof, who may answer, who took it, what its answer settles; (2) the
// header « « À faire » · <the person> » with how many wait; (3) a compact
// chip per kind waiting - overdue included - under the tiles, its count the
// inbox's, a click keeping only that kind (every count kept), the chosen chip
// showing all again; (4) the inbox's notices under « For information », each
// with « Dismiss ».
func TestÀFaireKeepsWhatTheInboxsPageShowed(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindBuild, Review: api.ReviewPerson,
		Deliverables: []api.Deliverable{{Title: "it builds", Check: "go version"}}})
	toReview(t, ctx, r.ID)
	views := views(t, ctx)
	page := func(in api.PersonQueueInput) string {
		t.Helper()
		def, err := views.PersonQueuePage(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(def)
		return string(raw)
	}
	put := func(td schedapi.Todo, ask map[string]any) {
		t.Helper()
		td.Module, td.Target, td.For, td.Start, td.Priority, td.State, td.Owner = "billing", "billing", schedapi.ForPerson, "manual", "P1", schedapi.StateReady, inboxapi.Owner
		td.Work = map[string]any{"task": inboxapi.HumanTaskStrategy, inboxapi.WorkAsk: ask}
		if due, ok := ask["due"]; ok {
			td.Work[inboxapi.WorkDue] = due // as the human task puts it
		}
		if _, err := schedOf(t, ctx).Put(ctx, schedapi.PutInput{Todo: td}); err != nil {
			t.Fatal(err)
		}
	}
	put(schedapi.Todo{Key: "ht-late", Title: "Approve the release notes"}, map[string]any{"kind": inboxapi.KindApprove, "title": "Approve the release notes",
		"about": r.ID, "question": "Read the notes", "due": "2026-01-02T00:00:00Z", "proof": "passkey", "answerable_by": "any",
		"satisfies": []any{r.ID + "/review"}})
	put(schedapi.Todo{Key: "ht-choose", Title: "Which space?"}, map[string]any{"kind": inboxapi.KindChoose, "title": "Which space?", "about": r.ID,
		"question": "Where the pages go", "options": []any{"ARCHI", "ARCHI-TEST"}})
	// One another member took: its card says who.
	taken, err := schedOf(t, ctx).Put(ctx, schedapi.PutInput{Todo: schedapi.Todo{Key: "ht-taken", Title: "Review the as-is chapter", Module: "billing", Target: "billing",
		For: schedapi.ForPerson, Start: "manual", Priority: "P1", State: schedapi.StateReady, Owner: inboxapi.Owner,
		Work: map[string]any{"task": inboxapi.HumanTaskStrategy, inboxapi.WorkAsk: map[string]any{"kind": inboxapi.KindApprove,
			"title": "Review the as-is chapter", "about": r.ID, "question": "Read chapter 2"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := schedOf(t, ctx).Pull(ctx, schedapi.PullInput{ID: taken.Todo.ID, By: cwmapi.Actor{ID: "marie", Kind: "person"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := inboxOf(t, ctx).File(ctx, inboxapi.FileInput{Title: "Your demand became a request", RequestedBy: "planification:demand@1", Expects: inboxapi.ExpectsNotice}); err != nil {
		t.Fatal(err)
	}
	on := page(api.PersonQueueInput{Language: "en"})
	// (1) The card's fields.
	late := cardText(t, on, "Approve the release notes")
	for _, want := range []string{"overdue 2026-01-02", `"color":"red"`, "proof: passkey", "answerable by any", "settles: " + r.ID + "/review"} {
		if !strings.Contains(late, want) {
			t.Errorf("(1) the late card lacks %q: %s", want, late)
		}
	}
	// Who took a question.
	if taken := cardText(t, on, "Review the as-is chapter"); !strings.Contains(taken, `"text":"taken by marie"`) {
		t.Errorf("(1) the card another member took does not say who: %s", taken)
	}
	// (2) The header.
	q, err := views.PersonQueue(ctx, api.PersonQueueInput{Language: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(on, `"id":"todo-header"`) || !strings.Contains(on, "« À faire » · ") || !strings.Contains(on, fmt.Sprintf("%d waiting", q.Count)) {
		t.Errorf("(2) no header « « À faire » · <person> » with %d waiting", q.Count)
	}
	if strings.Index(on, `"id":"todo-header"`) > strings.Index(on, `"kind":"kpi"`) {
		t.Error("(2) the header is not above the tiles")
	}
	// (3) A chip per kind, its count the inbox's, under the tiles; a click keeps that kind.
	if len(q.Kinds) == 0 || q.Overdue != 1 {
		t.Fatalf("(3) the queue counts kinds %+v, overdue %d; want the kinds waiting and the late one", q.Kinds, q.Overdue)
	}
	for _, k := range q.Kinds {
		if !strings.Contains(on, fmt.Sprintf(`"data-count":%d,"data-group-filter":"%s"`, k.Count, k.Kind)) {
			t.Errorf("(3) no chip « %s » counting %d", k.Kind, k.Count)
		}
	}
	if !strings.Contains(on, `"data-group-filter":"overdue"`) || strings.Index(on, `"id":"kinds"`) < strings.Index(on, `"kind":"kpi"`) {
		t.Error("(3) no overdue chip, or the chips are not under the tiles")
	}
	only := page(api.PersonQueueInput{Language: "en", Kind: inboxapi.KindChoose})
	if !strings.Contains(only, "Which space?") || strings.Contains(only, "Approve the release notes") || strings.Contains(only, "Validate — ") {
		t.Error("(3) the choose chip does not keep only the choices")
	}
	if !strings.Contains(only, `"data-count":1,"data-group-filter":"choose"`) || !strings.Contains(only, `"args":{"kind":""}`) {
		t.Error("(3) under the choose chip its count is lost or it does not show all again")
	}
	if lateOnly := page(api.PersonQueueInput{Language: "en", Kind: "overdue"}); !strings.Contains(lateOnly, "Approve the release notes") || strings.Contains(lateOnly, "Which space?") {
		t.Error("(3) the overdue chip does not keep only the late ones")
	}
	// (4) The inbox's notices under « For information », with Dismiss.
	notice := cardText(t, on, "Your demand became a request")
	if !strings.Contains(notice, "ktw-agents:inbox/dismiss") || strings.Index(on, "For information — ") > strings.Index(on, "Your demand became a request") {
		t.Errorf("(4) the inbox's notice is not under « For information » with Dismiss: %s", notice)
	}
	// In French.
	fr := page(api.PersonQueueInput{Language: "fr"})
	for _, want := range []string{"en attente", "Par genre", "en retard depuis le 2026-01-02", "preuve : passkey", "peut répondre : any", "règle : ", "pris par marie"} {
		if !strings.Contains(fr, want) {
			t.Errorf("the French page lacks %q", want)
		}
	}
}

// TestÀFairesCardSpeaksThePersonsLanguageAndHoldsInItsRow
// : a question's kind shows in
// the person's language even when its to-do carries the English label it was
// asked with; and « Put on hold » is one more button in the card's row of
// buttons, after its answers - not alone in the middle of the card.
func TestÀFairesCardSpeaksThePersonsLanguageAndHoldsInItsRow(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindFix})
	if _, err := schedOf(t, ctx).Put(ctx, schedapi.PutInput{Todo: schedapi.Todo{
		Key: "ht-test", Module: "billing", Target: "billing", Title: "Test the export", For: schedapi.ForPerson, Start: "manual", Priority: "P1",
		State: schedapi.StateReady, Owner: inboxapi.Owner, Work: map[string]any{"task": inboxapi.HumanTaskStrategy, inboxapi.WorkLabel: "Test by hand",
			inboxapi.WorkAsk: map[string]any{"kind": inboxapi.KindTestByHand, "title": "Test the export", "about": r.ID, "question": "Open it"}},
	}}); err != nil {
		t.Fatal(err)
	}
	def, err := views(t, ctx).PersonQueuePage(ctx, api.PersonQueueInput{Language: "fr"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(def.DSLTree)
	if s := string(raw); strings.Contains(s, `"text":"Test by hand"`) || !strings.Contains(s, `"text":"Test à la main"`) {
		t.Errorf("the kind badge is not in French: %s", cardText(t, s, "Test the export"))
	}
	var tree any
	_ = json.Unmarshal(raw, &tree)
	var card map[string]any
	walkCards(tree, func(n map[string]any) {
		if id, _ := n["id"].(string); strings.HasPrefix(id, "inbox:billing.TD") {
			card = n
		}
	})
	if card == nil {
		t.Fatal("no card for the test by hand")
	}
	todo := strings.TrimPrefix(card["id"].(string), "inbox:")
	row := rowOf(card, "all_good:"+todo)
	if row == nil || nodeByID(row, "hold:inbox:"+todo) == nil {
		t.Errorf("« Mettre en attente » is not in the row of « Tout est bon » / « Quelque chose a échoué »: %v", row)
	}
	kids, _ := card["children"].([]any)
	for _, k := range kids {
		if m, _ := k.(map[string]any); m != nil && m["id"] == "hold:inbox:"+todo {
			t.Error("« Mettre en attente » stands alone in the card")
		}
	}
}

// walkCards calls f on every node of a tree.
func walkCards(v any, f func(map[string]any)) {
	switch n := v.(type) {
	case []any:
		for _, c := range n {
			walkCards(c, f)
		}
	case map[string]any:
		f(n)
		for _, c := range n {
			walkCards(c, f)
		}
	}
}

// rowOf is the hbox of a tree holding, among its children, the node of that id.
func rowOf(v any, id string) map[string]any {
	var row map[string]any
	walkCards(v, func(n map[string]any) {
		if n["kind"] != "hbox" {
			return
		}
		kids, _ := n["children"].([]any)
		for _, k := range kids {
			if m, _ := k.(map[string]any); m != nil && m["id"] == id {
				row = n
			}
		}
	})
	return row
}
