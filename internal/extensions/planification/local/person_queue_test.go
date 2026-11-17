// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	objectpaneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	planlocal "github.com/katyoti/katyoti-fwk/internal/extensions/planification/local"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// « À FAIRE » — the person's queue: everything waiting on the person, each
// item asking exactly one action of the closed set.

// personQueue reads the queue (of everything when scope is empty).
func personQueue(t *testing.T, ctx context.Context, scope string) api.PersonQueueOutput {
	t.Helper()
	views, err := application.UseService[api.ViewService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	q, err := views.PersonQueue(ctx, api.PersonQueueInput{Target: scope})
	if err != nil {
		t.Fatal(err)
	}
	return q
}

// itemsOf is the queue's items about one object (or moving one request on).
func itemsOf(q api.PersonQueueOutput, id string) []api.PersonItem {
	var out []api.PersonItem
	for _, it := range q.Items {
		if it.Object == id || it.Request == id {
			out = append(out, it)
		}
	}
	return out
}

// actionsOf is the actions of items, in the queue's order.
func actionsOf(items []api.PersonItem) string {
	var out []string
	for _, it := range items {
		out = append(out, it.Action)
	}
	return strings.Join(out, ",")
}

// toReview works a build request up to its person's review.
func toReview(t *testing.T, ctx context.Context, id string) {
	t.Helper()
	for _, step := range []string{api.StepIntake, "analyse", "implement", "auto-review", "module-update"} {
		pullAndReport(t, ctx, stepOf(t, ctx, id, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	autoTested(t, ctx, id)
}

// autoTested reports the auto-test an agent runs before the review, when one
// is planned (a by-hand deliverable tried first): it keeps it by hand.
func autoTested(t *testing.T, ctx context.Context, id string) {
	t.Helper()
	if slices.Contains(steps(t, ctx, id), api.StepAutoTest) {
		pullAndReport(t, ctx, stepOf(t, ctx, id, api.StepAutoTest).ID, session, schedapi.Result{Summary: "only a person can judge it: kept by hand"})
	}
}

// openDecision opens a decision in billing, prepared with a proposal or not,
// planned by no request.
func openDecision(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, title string, proposed bool) string {
	t.Helper()
	o := cwmapi.Object{Type: "decision", Parent: "billing", Name: title}
	fields := map[string]any{"options": []string{"keep", "drop"}}
	if proposed {
		fields["proposal"] = map[string]any{"option": "keep", "why": "it costs nothing"}
	}
	_ = o.SetCapability("decision", fields)
	out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: o})
	if err != nil {
		t.Fatalf("create the decision: %v", err)
	}
	return out.Object.ID
}

// Everything waiting on the person is in one queue, each item with exactly
// one action of the closed set: a person's review (validate), a requester's
// question (answer), a failed step (unblock), a deliverable only a hand test
// verifies once its steps are done (test), an open decision with its options
// and a proposal, planned or not (decide) — not one unprepared, planned or
// not, which an agent prepares first —, a decide request's
// step on a decision (decide, once: the decision's own item), and the Inbox's
// entries no step carries (answer by text, validate by verdict). A step's own
// inbox entry is not a second item.
func TestPersonQueueGathersEverythingWaiting(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	review := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindBuild, Review: api.ReviewPerson,
		Deliverables: []api.Deliverable{{Title: "it builds", Check: "go version"}}})
	toReview(t, ctx, review.ID)
	// A review asked in the Inbox before IN-039: its item carries the entry.
	askedBefore(t, ctx, stepOf(t, ctx, review.ID, "review"), "Validate the result", inboxapi.ExpectsVerdict)
	asked := file(t, ctx, req, api.FileInput{Title: "Mail invoices", Module: "billing", Kind: api.KindBuild})
	if _, err := req.Ask(ctx, api.AskInput{ID: asked.ID, Question: "Which mail server?", By: intake}); err != nil {
		t.Fatal(err)
	}
	failed := file(t, ctx, req, api.FileInput{Title: "Ship invoices", Module: "billing", Kind: api.KindBuild})
	failPastTheCap(t, ctx, stepOf(t, ctx, failed.ID, api.StepIntake).ID, "no SMTP") // past the cap: the person\'s
	refused := file(t, ctx, req, api.FileInput{Title: "Archive invoices", Module: "billing", Kind: api.KindBuild,
		Deliverables: []api.Deliverable{{Title: "seen in the archive", ByHand: "Open the archive; the invoice is there. No agent sees the archive."}}})
	toReview(t, ctx, refused.ID)
	if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: stepOf(t, ctx, refused.ID, "review").ID,
		Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "fine", Verdict: api.VerdictApproved, By: person}}); err != nil {
		t.Fatal(err)
	}
	proposed := openDecision(t, ctx, cwm, "Keep the old format?", true)
	unprepared := openDecision(t, ctx, cwm, "Rename invoices?", false)
	onDecision := decision(t, ctx, cwm, req, "Drop paper invoices?")
	prepared(t, ctx, onDecision)
	preparing := decision(t, ctx, cwm, req, "Print in colour?")
	inbox := inboxOf(t, ctx)
	question, err := inbox.File(ctx, inboxapi.FileInput{Title: "May I deploy?", RequestedBy: "run:r1", Expects: inboxapi.ExpectsText})
	if err != nil {
		t.Fatal(err)
	}
	approval, err := inbox.File(ctx, inboxapi.FileInput{Title: "Confirm the release", RequestedBy: "gate:ready-to-release", Expects: inboxapi.ExpectsVerdict})
	if err != nil {
		t.Fatal(err)
	}

	q := personQueue(t, ctx, "")
	for _, it := range q.Items {
		if !slices.Contains(api.PersonActions, it.Action) && it.Action != api.ActionNotice {
			t.Errorf("%s asks %q: not one action of the closed set", it.Key, it.Action)
		}
		if strings.TrimSpace(it.Asks) == "" {
			t.Errorf("%s does not say what is expected", it.Key)
		}
	}
	want := map[string]string{
		review.ID: "validate", asked.ID: "answer", failed.ID: "unblock", refused.ID: "test",
		proposed: "decide", onDecision: "decide", question.Request.ID: "answer", approval.Request.ID: "validate",
	}
	for id, action := range want {
		if got := actionsOf(itemsOf(q, id)); got != action {
			t.Errorf("%s: items %q, want exactly one %s", id, got, action)
			for _, it := range itemsOf(q, id) {
				t.Logf("  %+v", it)
			}
		}
	}
	if got := itemsOf(q, preparing); len(got) != 0 {
		t.Errorf("an open decision a plan still prepares is in the queue: %+v", got)
	}
	if got := itemsOf(q, unprepared); slices.ContainsFunc(got, func(it api.PersonItem) bool { return it.Object == unprepared && it.Action == api.ActionDecide }) {
		t.Errorf("an unprepared decision is a decide item: %+v; a person cannot decide it", got)
	}
	ask := itemsOf(q, review.ID)[0].Ask
	if ask == "" || slices.ContainsFunc(q.Items, func(it api.PersonItem) bool { return it.Object == ask }) {
		t.Errorf("the review's inbox entry %q is a second item, or the review carries none", ask)
	}
	if it := itemsOf(q, proposed)[0]; it.Proposal != "keep" || len(it.Options) != 2 {
		t.Errorf("the decision's item: %+v; want its options and the one proposed", it)
	}
	if it := itemsOf(q, refused.ID)[0]; it.Deliverable != "seen in the archive" || !strings.Contains(it.ByHand, "Open the archive") {
		t.Errorf("the hand test: %+v; want the deliverable and how to test it", it)
	}
	// In the queue's order: decide, test, validate, answer, unblock.
	last := -1
	for _, it := range q.Items {
		i := slices.Index(append(append([]string{}, api.PersonActions...), api.ActionNotice), it.Action)
		if i < last {
			t.Fatalf("the queue is not by action: %s after a later action", it.Key)
		}
		last = i
	}
}

// One step, one action: a review whose request has a deliverable only a hand
// test verifies is two items — the test first, the validation waiting on it
// (its reason the deliverable) — and once the test passed, the validation
// alone, ready.
func TestPersonQueueTestsBeforeValidation(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindBuild,
		Deliverables: []api.Deliverable{{Title: "seen on paper", ByHand: "Print one; read it."}, {Title: "it builds", Check: "go version"}}})
	toReview(t, ctx, r.ID)
	items := itemsOf(personQueue(t, ctx, ""), r.ID)
	if got := actionsOf(items); got != "test,validate" {
		t.Fatalf("the review's items: %q; want the hand test, then the validation", got)
	}
	if items[0].Deliverable != "seen on paper" {
		t.Errorf("the test is of %q; want seen on paper (it builds has a check)", items[0].Deliverable)
	}
	if !strings.Contains(items[1].WaitsOn, "seen on paper") {
		t.Errorf("the validation waits on %q; want the hand test of seen on paper", items[1].WaitsOn)
	}
	if _, err := req.Test(ctx, api.TestInput{ID: r.ID, Deliverable: "seen on paper", Result: api.TestPasses, Note: "printed and read", By: person}); err != nil {
		t.Fatal(err)
	}
	items = itemsOf(personQueue(t, ctx, ""), r.ID)
	if got := actionsOf(items); got != "validate" || items[0].WaitsOn != "" {
		t.Fatalf("after the test passed: %q (waits on %q); want the validation alone, ready", got, items[0].WaitsOn)
	}
	if !slices.ContainsFunc(items[0].Proven, func(p string) bool { return strings.Contains(p, "seen on paper") }) {
		t.Errorf("the validation does not say the hand test is proven: %v", items[0].Proven)
	}
}

// A hand test passes — the deliverable verified by the person, what they saw
// as its evidence — or fails saying how: the review waiting on it is sent
// back with that note (the work before it done again); with no validation
// waiting, its agent work is done again the same way. Only a person tests by hand, a
// failure says how, and an agent's session is refused whatever by says.
func TestHandTestPassesOrFails(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindBuild,
		Deliverables: []api.Deliverable{{Title: "seen on paper", ByHand: "Print one; read it."}, {Title: "signed", ByHand: "Sign one by hand."}}})
	toReview(t, ctx, r.ID)
	agent := cause.WithCause(ctx, cause.Cause{Kind: cause.KindAgent, ID: "claude-code"})
	pass := api.TestInput{ID: r.ID, Deliverable: "seen on paper", Result: api.TestPasses, Note: "printed and read", By: person}
	if _, err := req.Test(agent, pass); err == nil {
		t.Fatal("an agent's session tested by hand for the person")
	}
	if _, err := req.Test(ctx, api.TestInput{ID: r.ID, Deliverable: "signed", Result: api.TestFails, By: person}); err == nil {
		t.Fatal("a failed hand test that says nothing was recorded")
	}
	if _, err := req.Test(ctx, pass); err != nil {
		t.Fatal(err)
	}
	if d := deliverableOf(t, ctx, req, r.ID, "seen on paper"); !d.Verified || d.VerifiedBy == nil || d.VerifiedBy.ID != person.ID ||
		!strings.Contains(d.Evidence, "tested by hand: printed and read") {
		t.Fatalf("after the test passed: %+v; want verified by the person, what they saw as evidence", d)
	}
	reviewBefore := stepOf(t, ctx, r.ID, "review")
	if _, err := req.Test(ctx, api.TestInput{ID: r.ID, Deliverable: "signed", Result: api.TestFails, Note: "no signature on page 2", By: person}); err != nil {
		t.Fatal(err)
	}
	redo := stepOf(t, ctx, r.ID, "implement (rework)")
	if !strings.Contains(redo.Packet.Instructions, "no signature on page 2") {
		t.Fatalf("after the test failed the work done again does not carry the note: %q", redo.Packet.Instructions)
	}
	if again := stepOf(t, ctx, r.ID, "review"); again.ID == reviewBefore.ID {
		t.Errorf("the review stayed asked after the failed test sent it back")
	}

	// No validation waiting: its agent work is done again at once, the
	// failure as feedback, and its plan resumes.
	ended := file(t, ctx, req, api.FileInput{Title: "Archive invoices", Module: "billing", Kind: api.KindBuild,
		Deliverables: []api.Deliverable{{Title: "seen in the archive", ByHand: "Open the archive; the invoice is there."}}})
	toReview(t, ctx, ended.ID)
	if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: stepOf(t, ctx, ended.ID, "review").ID,
		Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "fine", Verdict: api.VerdictApproved, By: person}}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.Test(ctx, api.TestInput{ID: ended.ID, Deliverable: "seen in the archive", Result: api.TestFails, Note: "not there", By: person}); err != nil {
		t.Fatal(err)
	}
	if redo := stepOf(t, ctx, ended.ID, "implement (rework)"); !strings.Contains(redo.Packet.Instructions, "a hand test failed") || !strings.Contains(redo.Packet.Instructions, "not there") {
		t.Fatalf("after its hand test failed the work done again does not say why: %q", redo.Packet.Instructions)
	}
	if st := planState(t, ctx, ended.ID); st != api.PlanReady {
		t.Fatalf("after its hand test failed its plan is %s; want resumed for its rework", st)
	}
}

// The queue's count is every item but the notices — the rail's badge — and a
// module's queue holds only that module's items.
func TestPersonQueueCount(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindBuild,
		Deliverables: []api.Deliverable{{Title: "seen on paper", ByHand: "Print one; read it."}}})
	toReview(t, ctx, r.ID)
	if _, err := inboxOf(t, ctx).File(ctx, inboxapi.FileInput{Title: "May I deploy?", Expects: inboxapi.ExpectsText}); err != nil {
		t.Fatal(err)
	}
	views, err := application.UseService[api.ViewService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	q := personQueue(t, ctx, "")
	notices := q.ByAction[api.ActionNotice]
	if q.Count != len(q.Items)-notices || q.Count != 3 {
		t.Errorf("the count = %d of %d items (%d notices); want 3: the test, the review, the question", q.Count, len(q.Items), notices)
	}
	c, err := views.PersonQueueCount(ctx, api.PersonQueueInput{})
	if err != nil || c.Count != q.Count {
		t.Errorf("PersonQueueCount = %+v, %v; want %d", c, err, q.Count)
	}
	billing := personQueue(t, ctx, "billing")
	if billing.Count != 2 {
		t.Errorf("billing's queue counts %d; want its request's two items", billing.Count)
	}
	if mine := personQueue(t, ctx, r.ID); mine.Count != 2 {
		t.Errorf("the request's queue counts %d; want its two items", mine.Count)
	}
	if other := personQueue(t, ctx, "shipping"); other.Count != 0 {
		t.Errorf("shipping's queue counts %d; want none", other.Count)
	}
}

// The « À faire » panel is declared on the Global strip with its count, on
// modules and on requests; its page has a section per action asked, and each
// card carries its buttons — a hand test's form (requests/test), the
// validation's buttons disabled until it passed (disabledWhen), each a verb
// of planification or the Inbox.
func TestPersonQueuePanel(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindBuild,
		Deliverables: []api.Deliverable{{Title: "seen on paper", ByHand: "Print one; read it."}}})
	toReview(t, ctx, r.ID)
	pane, err := application.UseService[objectpaneapi.ObjectPaneService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	global, err := pane.GlobalPanels(ctx, objectpaneapi.GlobalPanelsInput{})
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(global.Panels, func(p objectpaneapi.ObjectPanel) bool { return p.ID == "home:person-queue" })
	if i < 0 {
		t.Fatalf("« À faire » is not on the Global strip: %+v", global.Panels)
	}
	if p := global.Panels[i]; p.Source != "ktw-agents:views/person-queue-page" || p.Count != "ktw-agents:views/person-queue-count" {
		t.Errorf("« À faire » = %+v; want its page as source and its count", p)
	}
	for _, id := range []string{"billing", r.ID} {
		d, err := pane.Detail(ctx, objectpaneapi.DetailInput{ID: id})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.ContainsFunc(d.Panels, func(p objectpaneapi.ObjectPanel) bool { return p.ID == "tab:person-queue" }) {
			t.Errorf("« À faire » is not on %s's pane", id)
		}
	}
	views, err := application.UseService[api.ViewService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	def, err := views.PersonQueuePage(ctx, api.PersonQueueInput{Language: "fr"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(def.DSLTree)
	page := string(raw)
	for _, want := range []string{"Tester à la main — 1", "Valider — 1", "ktw-agents:requests/test", "ktw-agents:todos/report",
		`"disabledWhen":"waits_on"`, "Print one; read it.", "Déjà prouvé", `"label":"Résultat"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %s", want)
		}
	}
	if strings.Contains(page, `"label":"It"`) {
		t.Error("a hand test's result field is labelled It; want Result")
	}
	if def.Title != "À faire" {
		t.Errorf("the page's title = %q", def.Title)
	}
}

// « À faire » holds only decisions ready to decide: an
// open decision with no proposal, outside any request, is not a decide item,
// and the decision-prepared rule files a decide request on it — an agent
// prepares it first. Prepared, it is the person's; resolved, it leaves. A
// decision a piece of work raised (its source) is that work's to prepare, and
// raises nothing.
func TestPersonQueueHoldsOnlyDecisionsReadyToDecide(t *testing.T) {
	defer planlocal.TriggerGrace(200 * time.Millisecond)()
	ctx, cwm, req := world(t)
	alone := openDecision(t, ctx, cwm, "Rename invoices?", false)
	preparing := decision(t, ctx, cwm, req, "Print in colour?")

	q := personQueue(t, ctx, "")
	if got := itemsOf(q, alone); len(got) != 0 {
		t.Fatalf("the unprepared decision is in « À faire »: %+v; want none until it is prepared", got)
	}
	if got := itemsOf(q, preparing); len(got) != 0 {
		t.Errorf("a decision its request still prepares is in the queue: %+v", got)
	}
	slowly(t, "the unprepared decision's preparation filed", func() (string, bool) {
		got := raised(t, ctx, cwm, "decision-prepared", alone)
		return fmt.Sprint(got), len(got) == 1 && got[0].Kind == api.KindDecide
	})
	o, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: alone})
	if err != nil {
		t.Fatal(err)
	}
	if f, _, _ := cwmapi.DecodeCapability[api.RequestFields](o.Object, api.CapRequest); f.Kind != api.KindDecide {
		t.Fatalf("the preparation is not filed on the decision: %+v", f)
	}
	slowly(t, "the preparation planned, an agent's intake first", func() (string, bool) {
		got := steps(t, ctx, alone)
		return fmt.Sprint(got), len(got) > 0 && got[0] == api.StepIntake
	})
	if got := raised(t, ctx, cwm, "decision-prepared", preparing); len(got) != 0 {
		t.Errorf("a decision its request prepares raised a preparation: %+v", got)
	}

	ready := openDecision(t, ctx, cwm, "Keep the old format?", true)
	if got := itemsOf(personQueue(t, ctx, ""), ready); len(got) != 1 || got[0].Action != api.ActionDecide || got[0].Proposal != "keep" {
		t.Fatalf("a prepared decision: %+v; want one decide item with its proposal", got)
	}
	planlocal.Settle(ctx, 3*time.Second)
	if got := raised(t, ctx, cwm, "decision-prepared", ready); len(got) != 0 {
		t.Errorf("a prepared decision raised a preparation: %+v", got)
	}
	choose(t, ctx, cwm, ready, "keep", person)
	if got := itemsOf(personQueue(t, ctx, ""), ready); len(got) != 0 {
		t.Errorf("a decided decision is still in the queue: %+v", got)
	}
}

// « À faire » Decide holds only the person's decisions (
// ): a prepared decision marked an agent's is not asked of the
// person - an agent takes it with its reasons, the person may reopen it -
// while a prepared one of the person's is, decided inline from its card.
func TestPersonQueueHoldsOnlyThePersonsDecisions(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	theirs := openDecision(t, ctx, cwm, "Charge for exports?", true)
	o := cwmapi.Object{Type: "decision", Parent: "billing", Name: "Which cache?"}
	_ = o.SetCapability("decision", map[string]any{"options": []string{"keep", "drop"},
		"proposal": map[string]any{"option": "keep", "why": "it costs nothing"},
		"owner":    "agent", "owner_reason": "technical and reversible"})
	out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: o})
	if err != nil {
		t.Fatalf("create the agent's decision: %v", err)
	}
	agents := out.Object.ID

	q := personQueue(t, ctx, "")
	if got := itemsOf(q, agents); len(got) != 0 {
		t.Errorf("an agent's decision is in « À faire »: %+v; want none - an agent takes it", got)
	}
	if got := itemsOf(q, theirs); len(got) != 1 || got[0].Action != api.ActionDecide || len(got[0].Options) != 2 {
		t.Fatalf("the person's prepared decision: %+v; want one decide item with its options", got)
	}
}

// A prepared decision is chosen in « À faire » on its content, not its title:
// its card shows the question and its context, and under each option what it
// means, its pros and cons, its cost and what follows if chosen — the
// decision's own prose, read by decisions — before the option's form.
func TestPersonQueueShowsAPreparedDecisionsContent(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	o := cwmapi.Object{Type: "decision", Parent: "billing", Name: "Archive old invoices?", Body: `## Question

Invoices older than ten years fill the disk.

## Options

### keep

- **Means:** Every invoice stays online
- **Pros:**
  - Nothing to migrate
- **Cons:**
  - The disk fills by 2027
- **Cost:** A new disk
- **If chosen:** Nothing changes

### drop

- **Means:** Older invoices move to cold storage
- **Pros:**
  - The disk stays small
- **Cons:**
  - A restore takes a day
- **Cost:** Two agent-hours
- **If chosen:** A monthly archive job runs
`}
	_ = o.SetCapability("decision", map[string]any{"options": []string{"keep", "drop"},
		"proposal": map[string]any{"option": "drop", "why": "the disk is the constraint"}})
	out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: o})
	if err != nil {
		t.Fatalf("create the decision: %v", err)
	}
	got := itemsOf(personQueue(t, ctx, ""), out.Object.ID)
	if len(got) != 1 || len(got[0].Prepared) != 2 || got[0].Prepared[1].Cost != "Two agent-hours" ||
		!strings.Contains(got[0].Reasoning, "fill the disk") || strings.Contains(got[0].Reasoning, "### keep") {
		t.Fatalf("the prepared decision's item: %+v; want its reasoning without the options' prose and both options prepared", got)
	}
	views, err := application.UseService[api.ViewService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	def, err := views.PersonQueuePage(ctx, api.PersonQueueInput{Language: "fr"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(def.DSLTree)
	page := string(raw)
	for _, want := range []string{"Invoices older than ten years fill the disk.",
		"Ce que cela signifie", "Every invoice stays online", "Older invoices move to cold storage",
		"Pour", "Nothing to migrate", "The disk stays small", "Contre", "The disk fills by 2027", "A restore takes a day",
		"Coût", "A new disk", "Two agent-hours", "Si elle est choisie", "Nothing changes", "A monthly archive job runs",
		"the disk is the constraint"} {
		if !strings.Contains(page, want) {
			t.Errorf("« À faire » lacks %q", want)
		}
	}
}

// A request whose steps are done and whose end waits on a hand test, resumed
// from « À faire »: the resume tries its end again - refused, it is blocked
// again saying why and its hand test stays in « À faire »; never ready with
// no to-do and in no queue.
func TestAResumedRequestKeepsItsHandTest(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Mail invoices", Module: "billing", Kind: api.KindBuild,
		Deliverables: []api.Deliverable{{Title: "seen in the mailbox", ByHand: "Open the mailbox; the invoice is there."}}})
	toReview(t, ctx, r.ID)
	if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: stepOf(t, ctx, r.ID, "review").ID,
		Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "fine", Verdict: api.VerdictApproved, By: person}}); err != nil {
		t.Fatal(err)
	}
	if st := planState(t, ctx, r.ID); st != api.PlanBlocked {
		t.Fatalf("its end: %s; want refused (blocked)", st)
	}
	if got := actionsOf(itemsOf(personQueue(t, ctx, ""), r.ID)); got != api.ActionTest {
		t.Fatalf("before the resume « À faire » asks %q; want its hand test", got)
	}
	plans, err := application.UseService[api.PlanService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plans.Plan(ctx, api.PlanNextInput{ID: r.ID, By: person, Note: "resumed from « À faire »"}); err != nil {
		t.Fatal(err)
	}
	if st := planState(t, ctx, r.ID); st != api.PlanBlocked {
		t.Errorf("resumed with its steps done and its hand test not done: %s; want blocked again, saying why", st)
	}
	if got := actionsOf(itemsOf(personQueue(t, ctx, ""), r.ID)); got != api.ActionTest {
		t.Errorf("after the resume « À faire » asks %q; want its hand test", got)
	}
	// Its hand test passed, it ends.
	if _, err := req.Test(ctx, api.TestInput{ID: r.ID, Deliverable: "seen in the mailbox", Result: api.TestPasses, Note: "there", By: person}); err != nil {
		t.Fatal(err)
	}
	if st := planState(t, ctx, r.ID); st != api.PlanDone {
		t.Errorf("after its hand test passed: %s; want done", st)
	}
}
