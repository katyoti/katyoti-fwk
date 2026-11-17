// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"slices"
	"strings"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// READINESS — the scheduler hands out what is ready; planification says what
// is. A to-do of a request is ready once every to-do it comes after has ended
// (been collected and archived), every request its request depends on has —
// its intake never waits, a preparation step only until each dependency's own
// preparation is done — and every review holding it has; else it waits,
// saying on what. A failed or blocked one waits for `work retry`. A person's
// to-do that becomes ready is asked in the inbox, once — the question names
// the request and its step, never the to-do. The step that starts the
// template moves the node to its start state as it becomes ready.

// settleRequest settles a request's to-dos.
func settleRequest(ctx context.Context, request string, by cwmapi.Actor) error {
	todos, err := todosOf(ctx, request)
	if err != nil {
		return err
	}
	return settle(ctx, todos, by)
}

// SettleAll settles every scheduled to-do of planification, as planification
// itself: what the observer calls when a request ends past the plans service.
func SettleAll(ctx context.Context) error { return settleAll(ctx, PlanningActor) }

// SettleTodos settles the to-dos listed, each read again under the lock: a
// list taken before other settles ran decides nothing on its stale rows.
func SettleTodos(ctx context.Context, todos []api.WorkTodo) error {
	return settle(ctx, todos, PlanningActor)
}

// settleAll settles every scheduled to-do of planification.
func settleAll(ctx context.Context, by cwmapi.Actor) error {
	todos, err := todosOf(ctx, "")
	if err != nil {
		return err
	}
	return settle(ctx, todos, by)
}

// settling serialises the settles of this process: a request's end settles
// from the plans service and from the observer, and two settles from stale
// lists would ask a person twice or write an older copy over a newer one.
var settling sync.Mutex

// settle releases or holds each to-do not started — each read again just
// before it is decided, so a settle that comes second finds nothing left to
// change.
func settle(ctx context.Context, todos []api.WorkTodo, by cwmapi.Actor) error {
	settling.Lock()
	defer settling.Unlock()
	cwm, err := cwmOf(ctx)
	if err != nil {
		return err
	}
	// What is still scheduled is read now, under the lock - never taken from
	// todos, listed before it: a settle that waited behind others would find
	// there a to-do collected meanwhile and hold what came after it ("after
	// <collected>") over the newer settle's word, a hold's reason included.
	scheduled := map[string]api.WorkTodo{}
	open := map[string][]api.WorkTodo{} // the to-dos still scheduled, per request (all modules)
	all, err := todosOf(ctx, "")
	if err != nil {
		return err
	}
	for _, t := range all {
		open[t.Request] = append(open[t.Request], t)
		scheduled[t.ID] = t
	}
	lab := newLabelling()
	// A person's to-do whose question was answered while nothing listened
	// (the entry's updated event missed: a hub restarting) is collected once
	// this settle is over - its answer collects, and settles in turn.
	var asked []string
	defer func() {
		if len(asked) > 0 {
			go answerMissed(context.WithoutCancel(ctx), asked)
		}
	}()
	for _, t := range todos {
		fresh, err := getTodo(ctx, t.ID)
		if err != nil || fresh.Archived != "" {
			continue // collected or archived meanwhile (Get answers an archived one from done/)
		}
		t = fresh
		if !notStarted(t) {
			continue
		}
		gone, blocked := requestGone(ctx, cwm, t.Request)
		if gone != "" {
			// Its request ended or was split: its own steps are no one's work.
			if err := archive(ctx, t.ID, gone); err != nil && !errors.Is(err, cwmapi.ErrNotFound) && !errors.Is(err, fs.ErrNotExist) {
				return err // archived meanwhile (the end's own sweep) is no error
			}
			closeAsked(ctx, t, gone)
			continue
		}
		why := waitsOn(ctx, cwm, t, scheduled, open)
		if why == "" && t.For == schedapi.ForAgent {
			// A request blocked by hand is stopped: no agent takes its work
			// until it is resumed.
			why = blocked
		}
		l := lab.of(ctx, cwm, t.Request)
		priority := t.Priority
		if l.inc != "" && l.inc != t.Request {
			// The work an increment holds: pulled at the higher of its
			// request's priority and its increment's; all of it but its
			// intake held while the increment is on hold; its build onward
			// held while an increment it depends on is not done - its
			// intake, inputs and preparation run at once
			//
			h := lab.holdOf(ctx, cwm, l.inc)
			if base := l.priority; base != "" {
				priority = higherPriority(base, h.priority)
			} else {
				priority = higherPriority(priority, h.priority)
			}
			if why == "" && t.Packet.Phase != api.PhaseRequests {
				why = h.held
			}
			if why == "" && !beforeBuild(t.Packet.Phase) {
				why = h.dependency
			}
		}
		state := schedapi.StateReady
		if why != "" {
			state = schedapi.StateWaiting
		}
		if state == schedapi.StateReady && t.For == schedapi.ForPerson && t.Packet.AskedIn != "" {
			asked = append(asked, t.Packet.AskedIn)
		}
		if t.State == state && t.Waits == why && slices.Equal(t.Labels, l.labels) && t.Rank == l.rank && t.Priority == priority {
			if state == schedapi.StateReady {
				endIfReached(ctx, cwm, t)
			}
			continue
		}
		becameReady := state == schedapi.StateReady && (t.State != state || t.Waits != why)
		t.State, t.Waits, t.Labels, t.Rank, t.Priority = state, why, l.labels, l.rank, priority
		saved, err := put(ctx, t)
		if err != nil {
			return err
		}
		if becameReady {
			if err := onReady(ctx, cwm, saved, by); err != nil {
				return err
			}
		}
	}
	return nil
}

// answerMissed collects the person's to-dos asked in entries already answered
// or dismissed: Answered does what the entry's updated event would have.
func answerMissed(ctx context.Context, entries []string) {
	inbox, err := application.UseService[inboxapi.InboxService](ctx)
	if err != nil || inbox == nil {
		return
	}
	for _, entry := range entries {
		got, err := inbox.Get(ctx, inboxapi.GetInput{ID: entry})
		if err != nil || got.Entry.Status == "" || got.Entry.Status == inboxapi.StatusPending {
			continue
		}
		if _, err := New().Answered(ctx, entry); err != nil {
			slog.Warn("planification: an answer missed while nothing listened was not collected", "entry", entry, "error", err)
		}
	}
}

// requestGone says why a request's to-dos not started leave the queue: it
// ended (done or dropped), or it was split — its parts carry its work and it
// ends with them. "" while its own steps are still its work. blocked says why
// its agents' to-dos wait while it is blocked by hand (the block verb): until
// it is resumed (Plan), its work is no agent's.
func requestGone(ctx context.Context, cwm cwmapi.CWMService, request string) (gone, blocked string) {
	o, err := loadObject(ctx, cwm, request)
	if err != nil {
		return "", ""
	}
	switch st := planState(o); {
	case st == api.PlanSplit:
		return "its request was split: its parts carry its work", ""
	case planEnded(st):
		return "its request ended " + st, ""
	}
	if f, ok := planOf(o); ok && blockKind(f) == api.BlockManual {
		return "", blockedByHandWaits + f.Note
	}
	return "", ""
}

// blockedByHandWaits starts the reason an agent's to-do waits on its request
// blocked by hand.
const blockedByHandWaits = "its request is blocked: "

// waitsOnDependency starts the reason a to-do waits on a request it depends on.
const waitsOnDependency = "its request waits on "

// SettleStaleWaits settles again the to-dos of a request that wait on a
// dependency it no longer has: a depends-on edge removed past the requests
// service (cwm edge-delete) releases what it held.
// Nothing is read further for an object carrying no plan, or none waiting so.
func SettleStaleWaits(ctx context.Context, request string) error {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return err
	}
	o, err := loadObject(ctx, cwm, request)
	if err != nil || !hasRequest(o) {
		return nil
	}
	deps := edgesTo(o, api.EdgeDependsOn)
	todos, err := todosOf(ctx, request)
	if err != nil {
		return err
	}
	var stale []api.WorkTodo
	for _, t := range todos {
		if d, ok := strings.CutPrefix(t.Waits, waitsOnDependency); ok && notStarted(t) && !slices.Contains(deps, d) {
			stale = append(stale, t)
		}
	}
	if len(stale) == 0 {
		return nil
	}
	return settle(ctx, stale, PlanningActor)
}

// SettleScope settles again the open to-dos labelled with a scope - an
// increment or a release - so their labels follow it: moved to a new id (a
// CWM move, the old id given) or changed past the increments service (its
// release edited by hand). The labels are the scheduler's opaque fields, so
// planification re-derives them.
func SettleScope(ctx context.Context, scope string) error {
	if scope == "" {
		return nil
	}
	todos, err := todosOf(ctx, "")
	if err != nil {
		return err
	}
	var labelled []api.WorkTodo
	for _, t := range todos {
		if notStarted(t) && (slices.Contains(t.Labels, "increment:"+scope) || slices.Contains(t.Labels, "release:"+scope)) {
			labelled = append(labelled, t)
		}
	}
	if len(labelled) == 0 {
		return nil
	}
	return settle(ctx, labelled, PlanningActor)
}

// waitsOn is why a to-do is not ready; "" when it is.
func waitsOn(ctx context.Context, cwm cwmapi.CWMService, t api.WorkTodo, scheduled map[string]api.WorkTodo, open map[string][]api.WorkTodo) string {
	if stopped(t) {
		return t.Waits // until retried
	}
	if parked(t) {
		return parkedWaits // until its request is resumed
	}
	for _, a := range t.After {
		if _, still := scheduled[a]; still {
			return "after " + a
		}
	}
	for _, r := range t.Packet.HeldBy {
		if o, err := loadObject(ctx, cwm, r); err == nil && !nodeEnded(o) {
			return "held by the review of " + r
		}
	}
	if t.Packet.Phase == api.PhaseRequests {
		return "" // the intake is never held back
	}
	o, err := loadObject(ctx, cwm, t.Request)
	if err != nil {
		return ""
	}
	if w := waitsOnGUIBatch(ctx, cwm, t, o); w != "" {
		return w
	}
	for _, d := range edgesTo(o, api.EdgeDependsOn) {
		if proposalOn(ctx, cwm, o, d, t.Packet.Phase) {
			continue
		}
		if !descendsFrom(ctx, cwm, o, d) && !dependencyPassed(ctx, cwm, d, t.Packet.Phase, open[d]) {
			return waitsOnDependency + d
		}
	}
	return ""
}

// beforeBuild reports a phase that comes before the build: the intake, the
// inputs, the preparation. An unknown phase is not.
func beforeBuild(phase string) bool {
	i := api.PhaseIndex(phase)
	return i >= 0 && i < api.PhaseIndex(api.PhaseBuild)
}

// proposalOn reports a step of an increment's own plan up to its build - its
// proposal: analysed, reconciled, approved, applied - on an increment it
// depends on: it never waits on it. Only the work the
// increment holds waits, from its build on, and its own acceptance.
func proposalOn(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object, dep, phase string) bool {
	if o.Type != api.IncrementType || !(beforeBuild(phase) || phase == api.PhaseBuild) {
		return false
	}
	d, _, err := loadAny(ctx, cwm, dep)
	return err == nil && d.Type == api.IncrementType
}

// descendsFrom reports a request filed from another - a follow-up of it, or a
// part split from one, at any depth. Such a request never waits on it: that
// one, its own steps done, waits on its follow-ups to end.
func descendsFrom(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object, origin string) bool {
	seen := map[string]bool{o.ID: true}
	for range 16 {
		up := o.EdgeTo(api.EdgeFollowsUp)
		if up == "" {
			up = o.EdgeTo(api.EdgeSplitFrom)
		}
		if up == origin {
			return true
		}
		if up == "" || seen[up] {
			return false
		}
		seen[up] = true
		next, err := loadObject(ctx, cwm, up)
		if err != nil {
			return false
		}
		o = next
	}
	return false
}

// dependencyPassed reports a dependency a to-do of that phase no longer waits
// on: it ended or was decided - or nothing is being built on it (no agent's
// to-do of it open up to its build:) and its own
// lifecycle reached done while its plan is left open (a feature verified, its
// plan idle or blocked:), or it is a realized screen
// feature whose only missing proof is its GUI test, wherever that test waits
// (RQ-324: what follows is built; the feature reaches
// done once its test passed), or only a person's checks of it are left (its
// by-hand deliverables, a person's review or validation:
// they still gate its own done and its release) - or, for a preparation step,
// its own preparation is done (it is planned, and none of its requests, inputs
// or preparation to-dos is open).
func dependencyPassed(ctx context.Context, cwm cwmapi.CWMService, dep, phase string, openTodos []api.WorkTodo) bool {
	o, err := loadObject(ctx, cwm, dep)
	if err == nil && (nodeEnded(o) || decided(o)) {
		return true
	}
	if err == nil && !building(openTodos) && (lifeState(o) == api.StateDone || builtAwaitingGUI(ctx, cwm, o) || onlyPersonsChecksLeft(openTodos)) {
		return true
	}
	if phase != api.PhasePreparation {
		return false
	}
	planned := len(openTodos) > 0
	if !planned {
		if done, err := doneOf(ctx, dep); err == nil && len(done) > 0 {
			planned = true
		}
	}
	for _, t := range openTodos {
		if api.PhaseIndex(t.Packet.Phase) <= api.PhaseIndex(api.PhasePreparation) {
			return false
		}
	}
	return planned
}

// building reports a build or a rebuild under way on a dependency: taken in
// (its intake no longer open) and an agent's to-do of it open up to its build -
// what depends on it waits (RQ-427). A request filed on
// it and not taken in yet is no rebuild (its intake may drop, merge or route
// it:); its verify round builds nothing.
func building(openTodos []api.WorkTodo) bool {
	agent := false
	for _, t := range openTodos {
		if t.Packet.Phase == api.PhaseRequests {
			return false
		}
		agent = agent || (t.For != schedapi.ForPerson && api.PhaseIndex(t.Packet.Phase) <= api.PhaseIndex(api.PhaseBuild))
	}
	return agent
}

// onlyPersonsChecksLeft reports a dependency whose open to-dos are all a
// person's checks, from its verify phase on (a by-hand test, a review, a
// validation): no agent has anything left to do on it.
func onlyPersonsChecksLeft(openTodos []api.WorkTodo) bool {
	for _, t := range openTodos {
		if t.For != schedapi.ForPerson || api.PhaseIndex(t.Packet.Phase) < api.PhaseIndex(api.PhaseVerify) {
			return false
		}
	}
	return len(openTodos) > 0
}

// waitsOnGUIBatch holds a realized feature's verify round while its GUI tests
// wait in their batch: the round would only find
// gui-tested unmet; it is ready once the batch has run them. "" otherwise.
func waitsOnGUIBatch(ctx context.Context, cwm cwmapi.CWMService, t api.WorkTodo, o cwmapi.Object) string {
	if t.Packet.Template != recipeFeatureVerify || t.Target != o.ID {
		return ""
	}
	if b := guiBatches(ctx, cwm, o); len(b) > 0 {
		return guiBatchWaits + strings.Join(b, ", ")
	}
	return ""
}

// guiBatchWaits begins the wait of work whose GUI tests run in a batch.
const guiBatchWaits = "its GUI test runs in the batch of "

// recipeFeatureVerify is the verify round of a realized feature.
const recipeFeatureVerify = "feature-verify"

// onReady is what a to-do becoming ready does: the template's first step moves
// the node to its start state; a person's to-do is asked in the inbox.
func onReady(ctx context.Context, cwm cwmapi.CWMService, t api.WorkTodo, by cwmapi.Actor) error {
	if t.Packet.Starts != "" {
		if err := moveNode(ctx, cwm, t.Target, t.Packet.Starts, by, "its work starts: "+t.Packet.Step); err != nil {
			return fmt.Errorf("%s starts %s: %w", t.Packet.Step, t.Target, err)
		}
	}
	if t.For != schedapi.ForPerson || t.Packet.AskedIn != "" || moduleOff(ctx, t) {
		return nil
	}
	if t.Packet.EndsOn != "" {
		if o, err := loadObject(ctx, cwm, endsOnObject(t)); err == nil && ownState(o) == t.Packet.EndsOn {
			// Its target got there before it was asked: it ends rather than
			// asks — once this settle is over (the report collects, and its
			// collection settles in turn).
			go endReachedAlready(context.WithoutCancel(ctx), t)
			return nil
		}
	}
	return askPerson(ctx, t)
}

// endIfReached ends a person's step already asked whose target got where it
// ends while the event that should have ended it was missed (moved by another
// process, or while nothing listened): each settle finds it, never a person.
func endIfReached(ctx context.Context, cwm cwmapi.CWMService, t api.WorkTodo) {
	if t.For != schedapi.ForPerson || t.Packet.EndsOn == "" || !askedOf(t) {
		return
	}
	o, err := loadObject(ctx, cwm, endsOnObject(t))
	if err != nil || ownState(o) != t.Packet.EndsOn {
		return
	}
	if by, ok := movedBy(ctx, o.ID, t.Packet.EndsOn); ok {
		go func() {
			settling.Lock() // after this settle
			settling.Unlock()
			if err := endOnTarget(context.WithoutCancel(ctx), t, by); err != nil {
				slog.Warn("planification: a step whose target was reached did not end", "todo", t.ID, "error", err)
			}
		}()
	}
}

// askedOf says a person's step was asked: as a human task
// or in an Inbox entry asked before.
func askedOf(t api.WorkTodo) bool {
	return t.Packet.Ask != nil || t.Packet.AskedIn != ""
}

// moduleOff reports a to-do whose module's scheduling is off - the module in
// the to-do's project when several projects are served.
func moduleOff(ctx context.Context, t api.WorkTodo) bool {
	s, err := sched(ctx)
	if err != nil {
		return false
	}
	m, err := s.Modes(ctx, schedapi.ModesInput{})
	if err != nil {
		return false
	}
	modes, several := modesByRef(m.Modules)
	return offFor(modes, newProjectRefs(ctx, several), t)
}

// askTitle is what an Inbox entry asks, in plain words: the answer expected,
// the request, the step - never the step's name twice.
func askTitle(t api.WorkTodo) string {
	verb := map[string]string{api.InteractionValidate: "Validate the result", api.InteractionChallenge: "Challenge the result",
		api.InteractionDecide: "Decide"}[t.Packet.Interaction]
	if verb == "" {
		verb = "Answer"
	}
	return fmt.Sprintf("%s: %s (step %s)", verb, strings.TrimPrefix(t.Title, t.Packet.Step+" — "), t.Packet.Step)
}

// askPerson asks a person's to-do as a human task (askHumanTask).
func askPerson(ctx context.Context, t api.WorkTodo) error {
	return askHumanTask(ctx, t)
}

// askHumanTask asks a person's step as a human task (
// decision): its to-do is the one
// record - it carries what it asks (kind, question, about, what it settles:
// the step) and is answered by its report, the person's verdict or answer,
// collected as every step's; no inbox entry, no router.
func askHumanTask(ctx context.Context, t api.WorkTodo) error {
	kind := map[string]string{api.InteractionValidate: inboxapi.KindApprove, api.InteractionChallenge: inboxapi.KindApprove,
		api.InteractionDecide: inboxapi.KindDecide}[t.Packet.Interaction]
	if kind == "" {
		kind = inboxapi.KindProvide
	}
	about := t.Request
	if t.Packet.On != "" {
		about = t.Packet.On
	}
	ask := &inboxapi.HumanTask{Kind: kind, Title: askTitle(t), Question: t.Packet.Instructions, About: about,
		Satisfies: []string{t.Request + "/" + t.Packet.Step}, AnswerableBy: inboxapi.AnswerablePerson,
		AskedBy: "planification:" + t.Request + "/" + t.Packet.Step}
	if t.Packet.Ask != nil && t.Packet.Ask.Kind == ask.Kind && t.Packet.Ask.Title == ask.Title {
		return nil // asked already
	}
	// Its label and its kind's defaults come from the contributed kinds of
	// questions, never a list here.
	resolved, err := inboxapi.ResolveAsk(ctx, *ask)
	if err != nil {
		return fmt.Errorf("ask %s as a human task: %w", t.Packet.Step, err)
	}
	t.Packet.Ask, t.Packet.Label = &resolved, resolved.Label
	_, err = put(ctx, t)
	return err
}

// todoAnswered reports a person's to-do asked in an inbox entry with their
// answer: a validation or a challenge approved, or rejected with their note; a
// decision or what they provide passed on. Dismissed, any of them is blocked —
// a dismissed validation is no rejection, nothing says what to change. It
// answers the to-do, or "" when none was asked there.
func todoAnswered(ctx context.Context, entry string, e inboxapi.Entry, answer string) (string, error) {
	all, err := todosOf(ctx, "")
	if err != nil {
		return "", err
	}
	i := slices.IndexFunc(all, func(t api.WorkTodo) bool { return t.Packet.AskedIn == entry })
	if i < 0 {
		return "", nil
	}
	t := all[i]
	if t.State != schedapi.StateReady {
		return t.ID, nil // taken another way meanwhile
	}
	s, err := sched(ctx)
	if err != nil {
		return "", err
	}
	by := cwmapi.Actor{ID: e.ResolvedBy, Kind: "person"}
	if by.ID == "" {
		by.ID = "person"
	}
	r := schedapi.Result{Status: schedapi.StatusSuccess, Summary: answer, By: by} // a long answer: its first line, the rest in details
	switch {
	case e.Status == inboxapi.StatusDismissed:
		r.Status, r.Summary = schedapi.StatusBlocked, "the person dismissed it: "+answer
	case t.Packet.Interaction == api.InteractionValidate || t.Packet.Interaction == api.InteractionChallenge:
		r.Verdict = api.VerdictRejected
		if e.Status == inboxapi.StatusApproved {
			r.Verdict = api.VerdictApproved
		}
	default:
		r.Answer = answer
	}
	if _, err := s.Report(ctx, schedapi.ReportInput{ID: t.ID, Result: r}); err != nil {
		return "", fmt.Errorf("answer %s: %w", t.ID, err)
	}
	return t.ID, nil
}
