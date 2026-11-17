// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// THE REQUEST PANEL — a request on its own: where it stands and, when only a
// person can move it on, what is expected in plain words with the actions that
// do it; its deliverables (each check, whether it passed or who verified it by
// hand), a form revising them, its details, its steps and its trail. Every
// card of the planning deck opens it.

// The verbs the request panel and the deck's cards offer.
const (
	requestCheckURI    = "ktw-agents:requests/check"
	requestAutoTestURI = "ktw-agents:requests/auto-test"
	requestVerifyURI   = "ktw-agents:requests/verify"
	requestReviseURI   = "ktw-agents:requests/revise"
	requestWaiveURI    = "ktw-agents:requests/waive"
	todoReleaseURI     = "ktw-agents:todos/release"
	// RequestPanel is the request panel's name (its strip id panel:request):
	// a card opens it.
	RequestPanel = "request"
)

// The cases a person moves on, and what each card carries.
const (
	expectFailed  = "failed"  // a step failed: try it again, or drop the request
	expectHeld    = "held"    // a step stopped on what another moves: nothing to do here
	expectRefused = "refused" // its steps are done, a deliverable is not verified
	expectBlocked = "blocked" // blocked for another cause: re-evaluate or drop it
	expectAnswer  = "answer"  // a person's step is up, or its requester was asked
	expectLapsed  = "lapsed"  // its pulled step's claim lapsed: release it
)

// expectation is what a person is expected to do on a request, in plain words,
// and what its actions need: the to-do to retry, the inbox entry to answer, and
// whether a check can run.
type expectation struct {
	Case   string
	Text   string
	Todo   string
	Ask    string
	Checks bool
	// Untested: a deliverable no check verifies, which an agent can be asked
	// to test (requests autotest).
	Untested bool
	// Lapsed is the pulled to-do whose claim lapsed, to release.
	Lapsed string
}

// expected reads what a person is expected to do on a request from where it
// stands and its deliverables; the zero expectation when nothing waits on a
// person.
func expected(st stand, deliverables []api.Deliverable, askedIn []string) expectation {
	switch {
	case st.failedStep() != "" && st.hold != nil && !st.onPerson:
		// The one reader puts the stop on another - an agent retries it, an
		// outside party or other work moves it: the person is offered no
		// retry.
		verb, summary := stopVerb(*st.step)
		b := *st.hold
		return expectation{Case: expectHeld,
			Text: fmt.Sprintf("Its step %s %s: %s. It waits on %s (%s %s) - nothing to do here.", st.step.Packet.Step, verb, summary, b.Actor, api.BlockerClass(b.Kind), b.Kind)}
	case st.state == stateLabels[api.PlanBlocked] && st.failedStep() != "":
		verb, summary := stopVerb(*st.step)
		return expectation{Case: expectFailed, Todo: st.failedStep(),
			Text: fmt.Sprintf("Its step %s %s: %s. Try it again once the cause is fixed, or drop the request.", st.step.Packet.Step, verb, summary)}
	case st.state == stateLabels[api.PlanBlocked] && !endRefused(st.plan.PlanFields):
		return expectation{Case: expectBlocked, Text: "It is blocked: " + st.why + " — re-evaluate it once that is settled, or drop it."}
	case st.state == stateLabels[api.PlanBlocked]:
		e := expectation{Case: expectRefused}
		var missing []string
		for _, d := range deliverables {
			switch {
			case d.Verified:
			case d.Check == "" && strings.TrimSpace(d.ByHand) == "":
				e.Untested = true
				missing = append(missing, fmt.Sprintf("“%s” has no check: ask an agent to test it, or verify it by hand", d.Title))
			case d.Check == "":
				missing = append(missing, fmt.Sprintf("“%s” has no check: verify it by hand or give it a check", d.Title))
			default:
				e.Checks = true
				missing = append(missing, fmt.Sprintf("“%s”: its check has not passed — run the checks, or verify it by hand", d.Title))
			}
		}
		if len(missing) == 0 {
			e.Text = "Its end was refused: " + st.why
		} else {
			e.Text = "Its steps are done; to end it: " + strings.Join(missing, "; ") + "."
		}
		return e
	case st.awaitsPerson():
		e := expectation{Case: expectAnswer}
		if st.step != nil && st.step.For == schedapi.ForPerson && st.step.State == schedapi.StateReady {
			e.Ask = st.step.Packet.AskedIn
			e.Text = fmt.Sprintf("A person is asked to %s its step %s: answer it here, or in the Inbox.", st.step.Packet.Interaction, st.step.Packet.Step)
		} else {
			if len(askedIn) > 0 {
				e.Ask = askedIn[len(askedIn)-1]
			}
			e.Text = "Its requester was asked a question: answer it here, or in the Inbox."
		}
		return e
	case st.state == stateLabels[standLapsed] && st.step != nil && st.step.Claim != nil:
		return expectation{Case: expectLapsed, Lapsed: st.step.ID,
			Text: fmt.Sprintf("Its step %s, held by %s, lapsed: %s. Release it so another worker can take it.", st.step.Packet.Step, st.step.Claim.By.ID, st.why)}
	}
	return expectation{}
}

// endRefused reports a blocked request whose end was refused - a deliverable
// not verified, its node unable to move - not blocked for another cause: its
// block's kind says so.
func endRefused(f api.PlanFields) bool {
	k := blockKind(f)
	return k == api.BlockEndRefused || k == api.BlockMoveRefused
}

// expectedActions are the actions resolving a case, on an element carrying the
// request's `id` and — by its case — `todo`, `ask`, `checks`.
func expectedActions(onCard bool) []node {
	acts := []node{
		{"id": "check", "label": "Run checks", "icon": "player-play", "uri": requestCheckURI, "when": "checks",
			"args": node{"id": "${item.id}", "by": "${person}"}},
		{"id": "retry", "label": "Try its step again", "icon": "player-play", "uri": deckRetryURI, "when": "todo",
			"args": node{"id": "${item.todo}", "by": "${person}", "note": "tried again from the planning deck"}},
		{"id": "release", "label": "Release its step", "icon": "lock-open", "uri": todoReleaseURI, "when": "lapsed",
			"args": node{"id": "${item.lapsed}", "by": "${person}", "note": "released from the planning deck: its claim lapsed"}},
		{"id": "autotest", "label": "Ask an agent to test", "icon": "robot", "uri": requestAutoTestURI, "when": "untested",
			"args": node{"id": "${item.id}", "by": "${person}"}},
		{"id": "answer", "label": "Open in the Inbox", "icon": "inbox", "when": "ask",
			"navigate": node{"object": "${item.ask}"}},
	}
	if onCard {
		// Verifying by hand and revising need what only the request shows.
		acts = append(acts, node{"id": "resolve", "label": "Verify or revise…", "icon": "list-check", "when": "refused",
			"navigate": node{"object": "${item.id}", "panel": RequestPanel}})
	}
	return append(acts, dropAction())
}

// withExpected lays an expectation on a card: its plain words as the card's
// line, and the fields its actions show on.
func withExpected(c node, e expectation) node {
	if e.Case == "" {
		return c
	}
	c["meta"] = e.Text
	if e.Todo != "" {
		c["todo"] = e.Todo
	}
	if e.Ask != "" {
		c["ask"] = e.Ask
	}
	if e.Checks {
		c["checks"] = true
	}
	if e.Untested {
		c["untested"] = true
	}
	if e.Lapsed != "" {
		c["lapsed"] = e.Lapsed
	}
	if e.Case == expectRefused {
		c["refused"] = true
	}
	return c
}

// openRequestCard is what a click on a card does: open its request, on the request
// panel.
func openRequestCard() node {
	return node{"label": "Open the request", "navigate": node{"object": "${item.id}", "panel": RequestPanel}}
}

// reviseForm is what the request panel's form edits: the deliverables as they
// should be, and why.
type reviseForm struct {
	Deliverables []deliverableEdit `json:"deliverables" jsonschema:"description=Its deliverables as they should be"`
	Note         string            `json:"note" jsonschema:"description=Why they change"`
}

// The inbox's answer: one Answer, called from the Inbox and from here.
const inboxAnswerURI = "ktw-agents:inbox/answer"

// approveForm answers a validation or a challenge: approve it, or send it back
// with what to change.
type approveForm struct {
	Decision string `json:"decision" jsonschema:"description=approved: it goes on; rejected: it is done again,enum=approved|rejected,required"`
	Note     string `json:"note,omitempty" jsonschema:"description=What to change: required when you send it back"`
}

// answerForm answers a decision, what a person provides or a requester's
// question.
type answerForm struct {
	Note string `json:"note" jsonschema:"description=Your answer,required"`
}

// humanTaskBlock answers a person's step asked as a human task
// here, by its to-do's report: chosen on the object
// itself, its move ends the step; a validation is approved or sent back;
// anything else is answered.
func humanTaskBlock(t api.WorkTodo) []node {
	if t.Packet.EndsOn != "" {
		on := endsOnObject(t)
		return []node{render.Link("Choose on "+on+" — its move to "+t.Packet.EndsOn+" ends this step", render.NavigateTo("Choose", on, ""))}
	}
	if t.Packet.Interaction == api.InteractionValidate || t.Packet.Interaction == api.InteractionChallenge {
		return []node{render.AnswerForm(render.InlineSchema[approveForm]("ApproveOrSendBack", map[string]string{"decision": "Your decision", "note": "Your note"}),
			render.RunURI("Answer", todosReportURI, map[string]any{"id": t.ID, "status": "success", "verdict": "${value.decision}", "summary": "${value.note}", "by": "${person}"}),
			"send", approveForm{Decision: "approved"})}
	}
	return []node{render.AnswerForm(render.InlineSchema[answerForm]("AnswerTheQuestion", map[string]string{"note": "Your answer"}),
		render.RunURI("Answer", todosReportURI, map[string]any{"id": t.ID, "status": "success", "answer": "${value.note}", "summary": "${value.note}", "by": "${person}"}),
		"send", answerForm{})}
}

// questionBlock is the request's intake question asked of its requester as a
// human task and the form answering it here, by its
// to-do's report.
func questionBlock(ctx context.Context, request string) []node {
	todos, err := application.UseService[schedapi.TodoService](ctx)
	if err != nil {
		return nil
	}
	id, err := openQuestion(ctx, todos, request)
	if err != nil || id == "" {
		return nil
	}
	got, err := todos.Get(ctx, schedapi.GetInput{ID: id})
	if err != nil {
		return nil
	}
	ask, ok := inboxapi.AskOf(got.Todo)
	if !ok {
		return nil
	}
	return []node{{"kind": "markdown", "content": "**" + ask.Title + "**", "maxHeight": 280},
		render.AnswerForm(render.InlineSchema[answerForm]("AnswerTheQuestion", map[string]string{"note": "Your answer"}),
			render.RunURI("Answer", todosReportURI, map[string]any{"id": id, "status": "success", "answer": "${value.note}", "summary": "${value.note}", "by": "${person}"}),
			"send", answerForm{})}
}

// answerBlock is the question awaiting a person's answer, read from the inbox,
// and the form answering it here — through the inbox's own Answer, so the step
// waiting on it goes on.
func answerBlock(ctx context.Context, ask string, st stand) []node {
	inbox, err := application.UseService[inboxapi.InboxService](ctx)
	if err != nil {
		return nil
	}
	q, err := inbox.Get(ctx, inboxapi.GetInput{ID: ask})
	if err != nil || (q.Entry.Status != "" && q.Entry.Status != inboxapi.StatusPending) {
		return nil
	}
	question := "**" + q.Entry.Title + "**"
	if d := strings.TrimSpace(q.Entry.Description); d != "" {
		question += "\n\n" + d
	}
	if st.step != nil && st.step.Packet.AskedIn == ask && st.step.Packet.EndsOn != "" {
		// Chosen on the object itself (a decision's own panel): its move ends
		// the step, so no answer is typed here.
		on := endsOnObject(*st.step)
		return []node{{"kind": "markdown", "content": question, "maxHeight": 280},
			render.Link("Choose on "+on+" — its move to "+st.step.Packet.EndsOn+" ends this step", render.NavigateTo("Choose", on, ""))}
	}
	args := map[string]any{"id": ask, "note": "${value.note}", "by": "${person.id}"}
	var form render.Node
	interaction := ""
	if st.step != nil && st.step.Packet.AskedIn == ask {
		interaction = st.step.Packet.Interaction // the step's own question, not a requester's
	}
	if interaction == api.InteractionValidate || interaction == api.InteractionChallenge {
		args["decision"] = "${value.decision}"
		form = render.AnswerForm(render.InlineSchema[approveForm]("ApproveOrSendBack", map[string]string{"decision": "Your decision", "note": "Your note"}),
			render.RunURI("Answer", inboxAnswerURI, args), "send", approveForm{Decision: "approved"})
	} else {
		args["decision"] = "approved"
		form = render.AnswerForm(render.InlineSchema[answerForm]("AnswerTheQuestion", map[string]string{"note": "Your answer"}),
			render.RunURI("Answer", inboxAnswerURI, args), "send", answerForm{})
	}
	return []node{{"kind": "markdown", "content": question, "maxHeight": 280}, form}
}

// waiveForm is what a person says waiving a request's module update.
type waiveForm struct {
	Note string `json:"note" jsonschema:"description=Why its module needs no update,required"`
}

// moduleUpdateSection is where a request that changes a module stands on its
// module update: the want it serves, the change-log entry or why there is
// none, the acceptance checks changed, what is still missing or why it is
// exempt, a waiver — and, while it is open and held to the gate, the form a
// person waives it with. Nil for a request whose template changes no module.
func moduleUpdateSection(ctx context.Context, o cwmapi.Object, r api.Request, lang string) node {
	tmpl, err := templateFor(ctx, o)
	if err != nil || !changesModule(tmpl) {
		return nil
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return nil
	}
	line := func(text string) node { return node{"kind": "text", "content": text, "size": "sm"} }
	var lines []node
	if served := servedOf(ctx, cwm, o); served != "" {
		lines = append(lines, line(label(lang, "mu_serves")+": "+served))
	} else {
		lines = append(lines, line(label(lang, "mu_serves_none")))
	}
	mu := r.ModuleUpdate
	switch {
	case mu != nil && mu.Changelog != "":
		lines = append(lines, line(label(lang, "mu_changelog")+": "+mu.Changelog))
	case mu != nil && mu.Recorded():
		lines = append(lines, line(label(lang, "mu_invisible")+": "+mu.Reason))
	default:
		lines = append(lines, line(label(lang, "mu_changelog_none")))
	}
	if mu != nil && len(mu.Checks) > 0 {
		lines = append(lines, line(label(lang, "mu_checks")+": "+strings.Join(mu.Checks, ", ")))
	} else {
		lines = append(lines, line(label(lang, "mu_checks_none")))
	}
	fails, exempt := moduleUpdated(ctx, cwm, o, false)
	switch {
	case r.Waiver != nil:
		lines = append(lines, node{"kind": "text", "content": fmt.Sprintf("%s — %s, %s: %s", label(lang, "mu_waived"), r.Waiver.By.ID, shortTime(r.Waiver.At, ""), r.Waiver.Note), "size": "sm", "color": "orange"})
	case len(fails) > 0:
		lines = append(lines, node{"kind": "text", "content": label(lang, "mu_missing") + ": " + strings.Join(fails, "; "), "size": "sm", "color": "orange"})
	case exempt != "":
		lines = append(lines, node{"kind": "text", "content": label(lang, "mu_exempt") + ": " + exempt, "size": "sm", "color": "dimmed"})
	default:
		lines = append(lines, node{"kind": "text", "content": label(lang, "mu_ok"), "size": "sm", "color": "green"})
	}
	body := []node{{"kind": "vbox", "gap": 2, "children": lines}}
	if !planEnded(r.State) && r.Waiver == nil && exempt == "" {
		body = append(body, node{"kind": "vbox", "gap": 4, "children": []node{
			{"kind": "text", "content": label(lang, "mu_waive"), "size": "sm", "fw": 600},
			{"kind": "text", "content": label(lang, "mu_waive_hint"), "size": "xs", "color": "dimmed"},
			render.AnswerForm(render.InlineSchema[waiveForm]("WaiveModuleUpdate", map[string]string{"note": label(lang, "mu_waive_note")}),
				render.RunURI(label(lang, "mu_waive"), requestWaiveURI, map[string]any{"id": r.ID, "note": "${value.note}", "by": "${person}"}),
				"shield-off", waiveForm{}),
		}})
	}
	return section(label(lang, "mu_title"), "refresh", "cyan", label(lang, "mu_hint"), node{"kind": "vbox", "gap": "sm", "children": body})
}

// verifyForm is what a person says verifying a deliverable by hand.
type verifyForm struct {
	Evidence string `json:"evidence" jsonschema:"description=What you saw that verifies it,required"`
}

type deliverableEdit struct {
	Title string `json:"title" jsonschema:"description=What is delivered"`
	Check string `json:"check,omitempty" jsonschema:"description=How it is verified: a command or an action URI; empty: a person verifies it by hand"`
}

// Request answers the request panel of a request (its id).
func (v *Views) Request(ctx context.Context, in api.RequestViewInput) (render.Def, error) {
	ctx = withReadCache(ctx)
	cwm, err := cwmOf(ctx)
	if err != nil {
		return render.Def{}, err
	}
	o, err := load(ctx, cwm, in.ID)
	if err != nil {
		return render.Def{}, err
	}
	r := toRequest(o)
	todos, err := todosOf(ctx, r.ID)
	if err != nil {
		todos = nil
	}
	st := standOf(toPlanned(o), todos, (len(r.AskedIn) > 0 || openQuestions(ctx, nil)[r.ID]) && r.Plan != nil && (r.Plan.State == api.PlanIdle || r.Plan.State == ""), viewStepHold(ctx, o.ID, o.Name))
	if planEnded(r.State) || r.State == api.PlanSplit {
		st.state, st.why, st.ended = stateLabels[r.State], "", true
	}
	e := expected(st, r.Deliverables, r.AskedIn)

	head := []node{
		{"kind": "hbox", "gap": 6, "wrap": true, "children": []node{
			{"kind": "badge", "text": r.ID, "variant": "outline", "color": "gray", "mono": true},
			{"kind": "badge", "text": st.state, "variant": "light", "color": labelColors()[st.state]},
			{"kind": "badge", "text": r.Module, "variant": "light", "color": "indigo", "keepCase": true},
		}},
		{"kind": "text", "content": st.meta(), "size": "sm", "color": "dimmed"},
	}
	// Actualized: the request that carries its work now, those it carries.
	if next := edgeTo(o, api.EdgeSupersededBy); next != "" {
		head = append(head, render.Link("Superseded by "+next, render.NavigateTo("Superseded by "+next, next, RequestPanel)))
	}
	if prev, err := incoming(ctx, cwm, o.ID, api.EdgeSupersededBy); err == nil {
		for _, p := range prev {
			head = append(head, render.Link("Supersedes "+p, render.NavigateTo("Supersedes "+p, p, RequestPanel)))
		}
	}
	children := []node{section(r.Title, "send", "indigo", "", node{"kind": "vbox", "gap": "xs", "children": head})}
	if whole := wholeDemand(ctx, r.ID); whole != nil {
		children = append(children, whole)
	}
	if e.Case != "" {
		item := withExpected(node{"id": r.ID, "title": "What is expected", "state": st.state}, e)
		body := []node{{"kind": "list", "data": []node{item}, "item": node{"kind": "text", "content": "", "dataField": "$.item.meta", "size": "sm"},
			"toneField": "state", "toneColors": labelColors(), "actions": expectedActions(false)}}
		if e.Case == expectAnswer && e.Ask != "" {
			body = append(body, answerBlock(ctx, e.Ask, st)...)
		} else if e.Case == expectAnswer && st.step != nil && st.step.Packet.Ask != nil {
			body = append(body, humanTaskBlock(*st.step)...)
		} else if e.Case == expectAnswer {
			body = append(body, questionBlock(ctx, r.ID)...)
		}
		children = append(children, section("What is expected", "hand-stop", "orange", "", node{"kind": "vbox", "gap": "sm", "children": body}))
	}

	rows := []node{}
	edits := []deliverableEdit{}
	var unverified []string
	// The deliverables a person's step verifies when they approve it.
	approves := map[string]string{}
	for _, t := range todos {
		for _, title := range approvedBy(t.Packet.Verifies, r.Deliverables) {
			approves[title] = t.Packet.Step
		}
	}
	for _, d := range r.Deliverables {
		row := node{"id": d.Title, "title": d.Title, "check": d.Check, "status": "not verified"}
		switch {
		case d.Verified && d.VerifiedBy != nil && approvedStep(d.Evidence) != "":
			row["status"] = "approved at step " + approvedStep(d.Evidence) + " by " + d.VerifiedBy.ID
		case d.Verified && d.VerifiedBy != nil:
			row["status"] = "verified by " + d.VerifiedBy.ID
		case d.Verified:
			row["status"] = "check passed"
		default:
			unverified = append(unverified, d.Title)
		}
		switch {
		case d.Check != "":
		case !d.Verified && approves[d.Title] != "":
			row["check"] = "verified when a person approves " + approves[d.Title]
		default:
			row["check"] = "no check: a person verifies it"
		}
		rows = append(rows, row)
		edits = append(edits, deliverableEdit{Title: d.Title, Check: d.Check})
	}
	children = append(children, section("Deliverables", "list-check", "teal", "Each one verified by its check, or by a person who saw it",
		node{"kind": "list", "data": rows, "empty": "No deliverable.", "variant": "cards", "item": node{"kind": "vbox", "gap": 2, "children": []node{
			{"kind": "text", "content": "", "dataField": "$.item.title", "size": "sm", "fw": 600},
			{"kind": "hbox", "gap": 6, "wrap": true, "children": []node{
				{"kind": "badge", "text": "", "dataField": "$.item.status", "size": "xs", "variant": "light",
					"colors": map[string]string{"not verified": "orange", "check passed": "green"}},
				{"kind": "text", "content": "", "dataField": "$.item.check", "size": "xs", "color": "dimmed"},
			}},
		}}}))
	if !planEnded(r.State) && len(unverified) > 0 {
		// Verify by hand: a person says what they saw, deliverable by deliverable.
		var forms []node
		for _, title := range unverified {
			forms = append(forms, node{"kind": "vbox", "gap": 4, "children": []node{
				{"kind": "text", "content": title, "size": "sm", "fw": 600},
				render.AnswerForm(render.InlineSchema[verifyForm]("VerifyByHand", map[string]string{"evidence": "What you saw that verifies it"}),
					render.RunURI("Verify by hand", requestVerifyURI, map[string]any{"id": r.ID, "deliverable": title, "evidence": "${value.evidence}", "by": "${person}"}),
					"eye-check", verifyForm{}),
			}})
		}
		children = append(children, section("Verify by hand", "eye-check", "teal", "A deliverable no check verifies: say what you saw; the request ends once each one is verified",
			node{"kind": "vbox", "gap": "md", "children": forms}))
	}
	if mu := moduleUpdateSection(ctx, o, r, in.Language); mu != nil {
		children = append(children, mu)
	}
	if !planEnded(r.State) {
		children = append(children, section("Revise its deliverables", "pencil", "grape", "The full list as it should be; a deliverable whose check changes is verified again",
			render.AnswerForm(render.InlineSchema[reviseForm]("ReviseDeliverables", map[string]string{"deliverables": "Deliverables", "note": "Why they change"}),
				render.RunURI("Revise", requestReviseURI, map[string]any{"id": r.ID, "by": "${person}", "note": "${value.note}", "deliverables": "${value.deliverables}"}),
				"pencil", reviseForm{Deliverables: edits})))
	}
	if strings.TrimSpace(r.Details) != "" {
		children = append(children, section("Details", "file-text", "gray", "", node{"kind": "markdown", "content": r.Details, "maxHeight": 320}))
	}
	var steps, trail []node
	if r.Plan != nil {
		for _, s := range r.Plan.Steps {
			who := s.Task
			if s.By != nil {
				who = s.By.ID
			}
			summary, details := stepResult(s)
			title := s.Step + " — " + s.Outcome + " (" + who + ")"
			if added := stepAdded(s); added != "" {
				title += fmt.Sprintf(" · added, generation %d", s.Generation)
				details = strings.TrimSpace(added + "\n\n" + details)
			}
			item := node{"date": shortTime(s.EndedAt, s.StartedAt), "title": title, "body": summary,
				"color": map[string]string{schedapi.StatusSuccess: "green", schedapi.StatusFail: "red", schedapi.StatusBlocked: "orange"}[s.Outcome]}
			if details != "" {
				item["details"] = details
			}
			steps = append(steps, item)
		}
		for _, t := range r.Plan.Trail {
			trail = append(trail, node{"date": shortTime(t.At, ""), "title": t.From + " → " + t.To + " (" + t.By.ID + ")", "body": firstLine(t.Note)})
		}
	}
	if w := r.Waiver; w != nil {
		// The waiver is a move of its end: it shows among them, in time.
		trail = append(trail, node{"date": shortTime(w.At, ""), "title": label(in.Language, "mu_waived") + " (" + w.By.ID + ")", "body": firstLine(w.Note)})
		sort.SliceStable(trail, func(i, j int) bool { return trail[i]["date"].(string) < trail[j]["date"].(string) })
	}
	if len(steps) > 0 {
		children = append(children, section("Its steps", "list-numbers", "blue", "Each step that ran, who did it and what came of it", node{"kind": "timeline", "items": steps, "data-view": "request-steps"}))
	}
	if len(trail) > 0 {
		children = append(children, section("Its trail", "route", "gray", "Each move of its plan", node{"kind": "timeline", "items": trail}))
	}
	return render.Def{Kind: "page", Title: r.ID + " — " + r.Title, DSLTree: node{"kind": "vbox", "gap": "lg", "children": children}}, nil
}

// wholeDemand is the demand a request belongs to, from its root: every part
// and follow-up, recursively, each where it stands and the step it is on, this
// one marked; how far the whole is; where it was split. Nil for a request with
// neither parent nor part.
func wholeDemand(ctx context.Context, id string) node {
	all, err := NewPlans().List(ctx, api.ListPlansInput{})
	if err != nil {
		return nil
	}
	byID := map[string]api.PlannedObject{}
	children := map[string][]string{}
	for _, p := range all.Plans {
		byID[p.ID] = p
	}
	parent := func(p api.PlannedObject) string {
		if p.SplitFrom != "" {
			return p.SplitFrom
		}
		return p.FollowsUp
	}
	for _, p := range all.Plans {
		if up := parent(p); up != "" && byID[up].ID != "" {
			children[up] = append(children[up], p.ID)
		}
	}
	root := id
	for seen := map[string]bool{}; !seen[root]; {
		seen[root] = true
		if up := parent(byID[root]); up != "" && byID[up].ID != "" {
			root = up
		}
	}
	if len(children[root]) == 0 {
		return nil
	}
	var tree []api.PlannedObject
	var walk func(at string, seen map[string]bool)
	walk = func(at string, seen map[string]bool) {
		if seen[at] || byID[at].ID == "" {
			return
		}
		seen[at] = true
		tree = append(tree, byID[at])
		for _, c := range children[at] {
			walk(c, seen)
		}
	}
	walk(root, map[string]bool{})
	var open []api.PlannedObject
	for _, p := range tree {
		if !planEnded(p.State) && p.State != api.PlanSplit {
			open = append(open, p)
		}
	}
	at := map[string]stand{}
	if stands, err := standsOf(ctx, open, nil); err == nil {
		for _, st := range stands {
			at[st.plan.ID] = st
		}
	}
	count := map[string]int{}
	ended, waitingOnPerson := 0, 0
	var rows []node
	for _, p := range tree {
		state, meta := stateLabels[p.State], p.Module
		if st, ok := at[p.ID]; ok {
			state, meta = st.state, p.Module+" · "+st.stepText()
			if st.why != "" {
				meta += " · " + st.why
			}
			if st.awaitsPerson() {
				waitingOnPerson++
			}
		}
		switch {
		case p.SplitFrom != "":
			meta = "part · " + meta
		case p.FollowsUp != "":
			meta = "follow-up · " + meta
		}
		if p.ID == id {
			meta = "▸ this request · " + meta
		}
		if p.ID != root {
			count[state]++
			if planEnded(p.State) {
				ended++
			}
		}
		rows = append(rows, node{"id": p.ID, "parent": parent(p), "title": p.Title, "state": state, "meta": meta})
	}
	var segments []node
	for _, s := range []string{api.PlanDone, api.PlanDropped, standRunning, api.PlanReady, api.PlanIdle, api.PlanBlocked, api.PlanSplit} {
		segments = append(segments, node{"label": stateLabels[s], "value": count[stateLabels[s]], "color": stateColors[s]})
	}
	total := len(tree) - 1
	summary := fmt.Sprintf("%d of %d ended", ended, total)
	if waitingOnPerson > 0 {
		summary += fmt.Sprintf(" · %d waiting on a person", waitingOnPerson)
	}
	if n := count[stateLabels[api.PlanBlocked]]; n > 0 {
		summary += fmt.Sprintf(" · %d blocked", n)
	}
	body := []node{
		{"kind": "gauge", "segments": segments, "legend": true, "size": "lg", "valueText": summary},
	}
	for _, t := range byID[root].Trail {
		if t.To == api.PlanSplit {
			body = append(body, node{"kind": "text", "content": fmt.Sprintf("%s was split by %s on %s: %s", root, t.By.ID, shortTime(t.At, ""), firstLine(t.Note)), "size": "xs", "color": "dimmed"})
		}
	}
	colors := labelColors()
	body = append(body, node{"kind": "tree", "data": rows, "badgeField": "state", "badgeColors": colors, "item": itemLines(), "open": openRequestCard(),
		"toneField": "state", "toneColors": colors, "pulse": livePulse, "expanded": true})
	return section("The whole demand", "sitemap", "indigo", "From "+root+": its parts and follow-ups in any module, where each stands; a click opens one",
		node{"kind": "vbox", "gap": "sm", "children": body})
}

// stepAdded says where a step added beyond its request's recipe comes from
// : who added it, its generation and why; "" for a
// recipe step.
func stepAdded(s api.StepRecord) string {
	if s.Generation == 0 {
		return ""
	}
	who := "planification"
	if s.AddedBy != nil {
		who = s.AddedBy.ID
	}
	line := fmt.Sprintf("_Added beyond the recipe by %s, generation %d", who, s.Generation)
	if why := strings.TrimSpace(s.AddedWhy); why != "" {
		line += ": " + why
	}
	return line + "_"
}

// stepResult is a step's one-line summary and its detailed result: its details,
// or — a step recorded before details existed — the lines of its result after
// the first (read so, the file left as it is).
func stepResult(s api.StepRecord) (string, string) {
	first, rest, _ := strings.Cut(strings.TrimSpace(s.Result), "\n")
	details := strings.TrimSpace(s.Details)
	if rest = strings.TrimSpace(rest); rest != "" {
		if details != "" {
			details = rest + "\n\n" + details
		} else {
			details = rest
		}
	}
	return strings.TrimSpace(first), details
}

// shortTime is a time as a person reads it (date and minute), else the fallback.
func shortTime(at, fallback string) string {
	if at == "" {
		at = fallback
	}
	if len(at) >= 16 {
		return strings.Replace(at[:16], "T", " ", 1)
	}
	return at
}
