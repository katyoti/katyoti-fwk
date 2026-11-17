// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	objectpaneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// THE REQUEST PANEL — a request whose end was refused (a deliverable no check
// can verify) is in Needs a person saying what is missing in plain words, with
// the actions that resolve it; a click on its card opens the request panel,
// which says the same, offers Verify by hand on the deliverable and a form
// revising the deliverables, and shows its steps; verifying the deliverable by
// hand from there ends the request.
func TestARefusedEndSaysWhatIsMissingAndEndsFromTheView(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices", Module: "billing", Kind: api.KindBuild,
		Deliverables: []api.Deliverable{{Title: "they are sent", ByHand: "a person checks it; no agent can"}}})
	toReview(t, ctx, r.ID)
	review := stepOf(t, ctx, r.ID, "review")
	if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: review.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "fine", Verdict: api.VerdictApproved, By: person}}); err != nil {
		t.Fatal(err)
	}
	if st := planState(t, ctx, r.ID); st != api.PlanBlocked {
		t.Fatalf("its end: %s; want refused (blocked)", st)
	}
	missing := "“they are sent” has no check: verify it by hand or give it a check"

	needs := deckTab(t, ctx, "needs")
	for _, want := range []string{
		`"column":"refused"`, missing, `"refused":true`, // the card says what is missing
		`"label":"Verify or revise…"`, `"open":{"label":"Open the request","navigate":{"object":"${item.id}","panel":"request"}}`, // its actions; a click opens the request panel (the GUI names it panel:request)
		`"uri":"ktw-agents:requests/check"`, `"when":"checks"`, // Run checks only where a check can run
	} {
		if !strings.Contains(needs, want) {
			t.Errorf("Needs a person lacks %s", want)
		}
	}
	if strings.Contains(needs, `"checks":true`) {
		t.Error("a deliverable with no check offers Run checks")
	}

	views, err := application.UseService[api.ViewService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	def, err := views.Request(ctx, api.RequestViewInput{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := render.ValidatePage(def.DSLTree); err != nil {
		t.Fatalf("the request panel is off the page grammar: %v", err)
	}
	raw, _ := json.Marshal(def.DSLTree)
	page := string(raw)
	for _, want := range []string{
		"What is expected", missing, // what a person is expected to do
		`"uri":"ktw-agents:requests/verify"`, `"deliverable":"they are sent"`, `"evidence":"${value.evidence}"`, `"VerifyByHand"`, // Verify by hand: a form asking what was seen
		`"kind":"form"`, `"uri":"ktw-agents:requests/revise"`, `"deliverables":"${value.deliverables}"`, `"title":"they are sent"`, // the form revising them, filled
		"Its steps", "intake — success", // its steps
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the request panel lacks %s", want)
		}
	}

	// The panel's action: a person verifies the deliverable by hand — it ends.
	if _, err := req.Verify(ctx, api.VerifyInput{ID: r.ID, Deliverable: "they are sent", Evidence: "seen in the outbox", By: person}); err != nil {
		t.Fatal(err)
	}
	if st := planState(t, ctx, r.ID); st != api.PlanDone {
		t.Fatalf("after its deliverable was verified by hand: %s; want done", st)
	}
	def, err = views.Request(ctx, api.RequestViewInput{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(def.DSLTree)
	if strings.Contains(string(raw), "What is expected") || !strings.Contains(string(raw), "verified by "+person.ID) {
		t.Errorf("an ended request still expects something, or does not say who verified it: %s", raw)
	}
}

// A failed step says what failed and offers to try it again; a card of every
// other tab opens its request; the request panel is declared on requests.
func TestAFailedStepSaysWhatFailedAndEveryCardOpensItsRequest(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Ship them", Module: "billing", Kind: api.KindBuild})
	failed := stepOf(t, ctx, r.ID, api.StepIntake)
	failPastTheCap(t, ctx, failed.ID, "no SMTP") // past the cap: the person\'s
	needs := deckTab(t, ctx, "needs")
	for _, want := range []string{`"column":"failed"`, "Its step intake failed: no SMTP", `"todo":"` + failed.ID + `"`, `"uri":"ktw-agents:work/retry"`} {
		if !strings.Contains(needs, want) {
			t.Errorf("Needs a person lacks %s", want)
		}
	}
	for _, tab := range []string{"by-state", "by-phase", "requests", "deliveries", "history"} {
		if c := deckTab(t, ctx, tab); !strings.Contains(c, `"panel":"request"`) {
			t.Errorf("the %s tab's cards do not open their request", tab)
		}
	}
	pane, err := application.UseService[objectpaneapi.ObjectPaneService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	det, err := pane.Detail(ctx, objectpaneapi.DetailInput{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range det.Panels {
		if p.ID == "tab:request" && p.Source == "ktw-agents:views/request" {
			return
		}
	}
	t.Errorf("no request panel on a request: %+v", det.Panels)
}

// deckTab is the content of one tab of the every-module deck, as JSON.
func deckTab(t *testing.T, ctx context.Context, id string) string {
	t.Helper()
	views, err := application.UseService[api.ViewService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	switch id {
	case api.DeckTabDeliveries, api.DeckTabHistory, api.DeckTabGroups:
		// Loaded when the tab opens.
		def, err := views.DeckTab(ctx, api.DeckTabInput{Tab: id})
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(def.DSLTree)
		return string(raw)
	}
	def, err := views.Deck(ctx, api.DeckInput{})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range def.DSLTree.(map[string]any)["tabs"].([]map[string]any) {
		if p["id"] == id {
			raw, _ := json.Marshal(p["content"])
			return string(raw)
		}
	}
	t.Fatalf("no %s tab", id)
	return ""
}

// A request blocked for another cause than a deliverable says that cause, not
// a refused end.
func TestABlockedRequestSaysItsCause(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices", Module: "billing", Kind: api.KindBuild,
		Deliverables: []api.Deliverable{{Title: "they are sent", ByHand: "a person checks it; no agent can"}}})
	if _, err := plansOf(t, ctx).Block(ctx, api.BlockInput{ID: r.ID, By: session, Note: "the vendor has not answered"}); err != nil {
		t.Fatal(err)
	}
	needs := deckTab(t, ctx, "needs")
	if !strings.Contains(needs, "It is blocked: the vendor has not answered") || strings.Contains(needs, "has no check") {
		t.Errorf("a request blocked for another cause: %s", needs)
	}
}

// A person's step asked as a human task is shown on the
// Request panel with an answer form submitting its to-do's report, and
// answering moves the request on; a requester's question, a human task too
// , is answered the same way.
func TestAPersonAnswersFromTheRequestPanel(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices", Module: "billing", Kind: api.KindBuild, Review: api.ReviewPerson})
	for _, step := range []string{api.StepIntake, "analyse", "implement", "auto-review", "module-update"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	review := stepOf(t, ctx, r.ID, "review")
	if review.Packet.Ask == nil {
		t.Fatal("the person's review was not asked")
	}
	views, err := application.UseService[api.ViewService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	def, err := views.Request(ctx, api.RequestViewInput{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := render.ValidatePage(def.DSLTree); err != nil {
		t.Fatalf("the request panel is off the page grammar: %v", err)
	}
	raw, _ := json.Marshal(def.DSLTree)
	for _, want := range []string{`"uri":"ktw-agents:todos/report"`, `"id":"` + review.ID + `"`, `"verdict":"${value.decision}"`, "ApproveOrSendBack"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the request panel lacks %s", want)
		}
	}
	// The form's action: the to-do's report, approved.
	pullAndReport(t, ctx, review.ID, person, schedapi.Result{Verdict: api.VerdictApproved, Summary: "looked at the invoices"})
	deadline := time.Now().Add(5 * time.Second)
	for planState(t, ctx, r.ID) != api.PlanDone {
		if time.Now().After(deadline) {
			t.Fatalf("5s after the answer: %s; want done", planState(t, ctx, r.ID))
		}
		time.Sleep(20 * time.Millisecond)
	}

	q := file(t, ctx, req, api.FileInput{Title: "Price it", Module: "billing", Kind: api.KindBuild})
	if _, err := req.Ask(ctx, api.AskInput{ID: q.ID, Question: "Monthly or yearly?", By: intake}); err != nil {
		t.Fatal(err)
	}
	qs := intakeQuestions(t, ctx, q.ID)
	if len(qs) != 1 {
		t.Fatalf("questions about %s: %d; want one human task", q.ID, len(qs))
	}
	def, err = views.Request(ctx, api.RequestViewInput{ID: q.ID})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(def.DSLTree)
	for _, want := range []string{"Monthly or yearly?", "AnswerTheQuestion", `"id":"` + qs[0].ID + `"`, `"answer":"${value.note}"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the requester's question in the panel lacks %s", want)
		}
	}
}

// Sent back from the Request panel (rejected, with what to change), the
// reviewed work is done again, its feedback the whole note, every line of it;
// sent back without a note, it is refused and nothing is done again; and the
// Request panel is a request's first tab, so the Inbox opens on it.
func TestARequestSentBackFromItsPanelIsDoneAgain(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices", Module: "billing", Kind: api.KindBuild, Review: api.ReviewPerson})
	for _, step := range []string{api.StepIntake, "analyse", "implement", "auto-review", "module-update"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	ask := askedBefore(t, ctx, stepOf(t, ctx, r.ID, "review"), "Validate the result", inboxapi.ExpectsVerdict).Packet.AskedIn
	inbox, err := application.UseService[inboxapi.InboxService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inbox.Answer(ctx, inboxapi.AnswerInput{ID: ask, Decision: "rejected", By: person.ID}); err == nil {
		t.Fatal("sent back without a note: accepted")
	}
	if q, err := inbox.Get(ctx, inboxapi.GetInput{ID: ask}); err != nil || q.Entry.Status != inboxapi.StatusPending {
		t.Fatalf("sent back without a note: the question is %+v, %v; want it still pending", q.Entry, err)
	}
	time.Sleep(100 * time.Millisecond) // what a wrongly accepted answer would have scheduled
	if left := steps(t, ctx, r.ID); slices.ContainsFunc(left, func(s string) bool { return strings.HasPrefix(s, "implement") }) {
		t.Fatalf("sent back without a note: %v scheduled; want nothing done again", left)
	}
	const note = "the totals are wrong\nand the VAT line is missing"
	if _, err := inbox.Answer(ctx, inboxapi.AnswerInput{ID: ask, Decision: "rejected", Note: note, By: person.ID}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if left := steps(t, ctx, r.ID); slices.ContainsFunc(left, func(s string) bool { return strings.HasPrefix(s, "implement") }) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("5s after it was sent back: %v scheduled; want its implementation again", steps(t, ctx, r.ID))
		}
		time.Sleep(20 * time.Millisecond)
	}
	redo := stepOf(t, ctx, r.ID, "implement (rework)")
	feedback, _ := redo.Packet.Params["feedback"].(string)
	for _, line := range strings.Split(note, "\n") {
		if !strings.Contains(feedback, line) || !strings.Contains(redo.Packet.Instructions, line) {
			t.Fatalf("the rework's feedback %q, instructions %q; want the whole note, %q among it", feedback, redo.Packet.Instructions, line)
		}
	}
	if st := planState(t, ctx, r.ID); st == api.PlanDone {
		t.Fatal("a request sent back ended done")
	}
	pane, err := application.UseService[objectpaneapi.ObjectPaneService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	det, err := pane.Detail(ctx, objectpaneapi.DetailInput{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(det.Panels) == 0 || det.Panels[0].ID != "tab:request" {
		ids := []string{}
		for _, p := range det.Panels {
			ids = append(ids, p.ID)
		}
		t.Errorf("a request's tabs: %v; want the Request panel first", ids)
	}
}

// A step's result is a one-line summary and, when there is more, its details:
// both kept on the request's step, the Request panel showing the summary with
// the details behind Details…; a summary written on several lines keeps its
// first line, the rest becoming its details.
func TestAStepResultIsASummaryAndItsDetails(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices", Module: "billing", Kind: api.KindBuild})
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "build, P2, one piece",
		Details: "Routed to billing.\n\n- assessed: build\n- estimate: 2 days"})
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "analyse").ID, session, schedapi.Result{Summary: "the plan is three changes\nfirst the template\nthen the mailer"})
	got, err := req.Get(ctx, api.GetInput{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	steps := got.Request.Plan.Steps
	if len(steps) != 2 || steps[0].Result != "build, P2, one piece" || !strings.Contains(steps[0].Details, "estimate: 2 days") {
		t.Fatalf("the intake's step: %+v; want its summary and its details", steps)
	}
	if steps[1].Result != "the plan is three changes" || !strings.Contains(steps[1].Details, "then the mailer") {
		t.Fatalf("a summary on several lines: %+v; want its first line, the rest in details", steps[1])
	}
	views, err := application.UseService[api.ViewService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	def, err := views.Request(ctx, api.RequestViewInput{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := render.ValidatePage(def.DSLTree); err != nil {
		t.Fatalf("the request panel is off the page grammar: %v", err)
	}
	raw, _ := json.Marshal(def.DSLTree)
	for _, want := range []string{`"body":"build, P2, one piece"`, `"details":"Routed to billing.`, `"body":"the plan is three changes"`, `"details":"first the template`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the steps lack %s", want)
		}
	}
}

// The Request panel's answer form shows its fields and one button, Answer: no
// heading of its own and no Delete, Cancel or Reset, which edit an object; once
// the request is done its header names no next step.
func TestTheAnswerFormIsBareAndAnEndedRequestNamesNoNextStep(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices", Module: "billing", Kind: api.KindBuild, Review: api.ReviewPerson})
	for _, step := range []string{api.StepIntake, "analyse", "implement", "auto-review", "module-update"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	ask := askedBefore(t, ctx, stepOf(t, ctx, r.ID, "review"), "Validate the result", inboxapi.ExpectsVerdict).Packet.AskedIn
	views, err := application.UseService[api.ViewService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	panel := func() any {
		def, err := views.Request(ctx, api.RequestViewInput{ID: r.ID})
		if err != nil {
			t.Fatal(err)
		}
		if err := render.ValidatePage(def.DSLTree); err != nil {
			t.Fatalf("the request panel is off the page grammar: %v", err)
		}
		raw, _ := json.Marshal(def.DSLTree)
		var tree any
		_ = json.Unmarshal(raw, &tree)
		return tree
	}

	// The answer form, resolved as the object pane shows it.
	tree, err := render.ResolveComponents(panel(), render.ComponentResolver{})
	if err != nil {
		t.Fatal(err)
	}
	form := findNode(tree, func(n map[string]any) bool {
		s, _ := n["schema"].(map[string]any)
		return n["kind"] == "form" && s["root"] == "ApproveOrSendBack"
	})
	if form == nil {
		t.Fatal("no answer form on the request panel")
	}
	if form["hideTitle"] != true {
		t.Errorf("the answer form draws its own heading: %v", form)
	}
	root := form["schema"].(map[string]any)["types"].(map[string]any)["ApproveOrSendBack"].(map[string]any)
	var actions []string
	for _, e := range asSlice(root["extensions"]) {
		ext, _ := e.(map[string]any)
		if ext["type"] != "ui-form" || ext["name"] != form["variant"] {
			continue
		}
		for _, a := range asSlice(ext["actions"]) {
			m, _ := a.(map[string]any)
			actions = append(actions, m["id"].(string)+":"+m["label"].(string))
		}
	}
	if !slices.Equal(actions, []string{"submit:Answer"}) {
		t.Errorf("the answer form's buttons: %v; want its submit only, Answer (no Delete, Cancel or Reset)", actions)
	}

	// Approved: the request is done, its header names its module and no step.
	inbox, err := application.UseService[inboxapi.InboxService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inbox.Answer(ctx, inboxapi.AnswerInput{ID: ask, Decision: inboxapi.StatusApproved, Note: "fine", By: person.ID}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for planState(t, ctx, r.ID) != api.PlanDone {
		if time.Now().After(deadline) {
			t.Fatalf("5s after the answer: %s; want done", planState(t, ctx, r.ID))
		}
		time.Sleep(20 * time.Millisecond)
	}
	meta := findNode(panel(), func(n map[string]any) bool {
		c, _ := n["content"].(string)
		return n["kind"] == "text" && n["color"] == "dimmed" && strings.HasPrefix(c, "billing")
	})
	if meta == nil || meta["content"] != "billing" {
		t.Errorf("a done request's header line: %v; want its module, no next step", meta)
	}
}

// findNode is the first node of a page tree that match accepts.
func findNode(v any, match func(map[string]any) bool) map[string]any {
	switch n := v.(type) {
	case map[string]any:
		if match(n) {
			return n
		}
		for _, c := range n {
			if f := findNode(c, match); f != nil {
				return f
			}
		}
	case []any:
		for _, c := range n {
			if f := findNode(c, match); f != nil {
				return f
			}
		}
	}
	return nil
}

func asSlice(v any) []any { s, _ := v.([]any); return s }
