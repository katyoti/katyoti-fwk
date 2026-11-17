// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// COLLECTING A RESULT — the scheduler hands each reported to-do of
// planification to Collect; planification, never the worker, decides what
// follows:
//   - what the step came to is written on the request (its history: who, when,
//     the outcome, what was done) — the lasting trace; the to-do is archived;
//   - failed or blocked, it goes back to wait for `work retry`, saying why -
//     an agent's failed check (auto-review, auto-test, verify) has its rework
//     planned instead, under the cap of agent rounds
//     a blocked report that adds steps has them planned before it, and it
//     waits on them instead;
//   - a person's approval confirms the gate requirement the step names, and
//     verifies the deliverable it names (WorkPacket.Verifies); a
//     rejected validation or challenge has the agent work before it done again,
//     with the person's note as feedback, then asks the person again;
//   - a person's answer passes to the to-dos after it; steps the result adds go
//     after it, in order, and what came after waits on them; follow-ups are
//     filed as requests of the module;
//   - an intake that changed the request's template plans it again;
//   - a request whose to-dos have all ended ends: its deliverables checked,
//     its node moved to the state the template reaches, its plan done —
//     refused, the request is blocked, saying why; a split demand ends once its
//     parts have; then every to-do is settled.

// Collect is planification's collector. It runs to its end before the
// report is answered: an apply's collect plans its increment's work without
// running acceptance probes (planIncrements), so it stays bounded by its CWM
// writes; a failure is returned to the reporter.
func Collect(ctx context.Context, t schedapi.Todo) error {
	return collect(ctx, t)
}

func collect(ctx context.Context, t schedapi.Todo) error {
	ctx = context.WithValue(ctx, collectingKey{}, true)
	wt := decode(t)
	// The request in its project (several projects served): what the report
	// reads, writes and puts next is that project's.
	wt.Request = todoRequestRef(ctx, t)
	r := t.Result
	if r == nil {
		return invalid("Collect: %s has no result", t.ID)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return err
	}
	by := r.By
	if ended(ctx, cwm, wt.Request) {
		return collectEnded(ctx, cwm, wt)
	}
	// A follow-up its module would refuse to file is refused before anything
	// is recorded: collected, the step would be recorded and the follow-up
	// lost, what comes after waiting on a collect that fails again
	//
	if err := checkFollowUps(ctx, cwm, followUps(r)); err != nil {
		return backToQueue(ctx, wt, err)
	}
	if r.Status == schedapi.StatusSuccess {
		if err := checkAnswer(wt.Packet, *r); err != nil {
			return backToQueue(ctx, wt, err)
		}
	}
	// A result that ends on a choice opens its decision first: refused or
	// failed, nothing is recorded and the step goes back to the queue.
	decision := ""
	if c := choiceOf(r); c != nil && r.Status == schedapi.StatusSuccess {
		if err := checkChoice(wt.Packet, *c); err != nil {
			return backToQueue(ctx, wt, err)
		}
		if decision, err = openChoice(ctx, wt, *c); err != nil {
			return backToQueue(ctx, wt, fmt.Errorf("open the decision the choice asks: %w", err))
		}
	}
	// A demand's reconcile answers its delta, checked before anything is
	// recorded; its apply answers what each operation changed or created.
	var applied []api.Applied
	if r.Status == schedapi.StatusSuccess && wt.Packet.Doer != api.DoerPerson {
		// A redone step is the same step: a reworked reconcile's delta is
		// kept, a reworked apply checked against it.
		switch strings.TrimSuffix(wt.Packet.Step, reworkSuffix) {
		case StepModuleUpdate, stepModuleUpdateAgain:
			if err := checkCommitted(ctx, cwm, wt.Request); err != nil {
				return backToQueue(ctx, wt, err)
			}
		case api.StepReconcile:
			d := withAboutIncrement(ctx, cwm, wt.Request, deltaOf(r))
			if err := checkDelta(ctx, cwm, d); err != nil {
				return backToQueue(ctx, wt, err)
			}
			if err := checkScreenSwitch(ctx, cwm, wt.Request, d); err != nil {
				return backToQueue(ctx, wt, err)
			}
			if err := keepDelta(ctx, wt.Request, d); err != nil {
				return err
			}
		case api.StepApplyDelta:
			req, err := load(ctx, cwm, wt.Request)
			if err != nil {
				return err
			}
			// What an earlier round applied stays applied: this report
			// answers the operations that waited, and may name others again.
			prior := fieldsOf(req).Applied
			applied = appliedOf(r)
			if err := checkApplied(ctx, cwm, fieldsOf(req).Delta, mergeApplied(prior, applied)); err != nil {
				return backToQueue(ctx, wt, err)
			}
		}
	}
	// An increment's delivery review: an approval whose
	// checks do not hold goes back to the queue; a review is recorded on the
	// increment, whatever its verdict.
	var review *api.DeliveryReview
	if r.Status == schedapi.StatusSuccess && strings.TrimSuffix(wt.Packet.Step, reworkSuffix) == StepDeliveryReview {
		rv := reviewOf(r, wt.Packet.Doer == api.DoerPerson, by)
		if err := checkDeliveryReview(ctx, cwm, wt, rv); err != nil {
			return backToQueue(ctx, wt, err)
		}
		review = &rv
	}
	// A rejection that would send back no work is refused before anything is
	// recorded.
	if r.Status == schedapi.StatusSuccess && r.Verdict == api.VerdictRejected && reworksOnReject(wt.Packet) {
		if err := nothingToRedo(ctx, wt); err != nil {
			return backToQueue(ctx, wt, err)
		}
	}
	if err := record(ctx, cwm, wt); err != nil {
		return err
	}
	closeAsk(ctx, wt)
	if review != nil {
		if err := recordDeliveryReview(ctx, cwm, wt.Request, review); err != nil {
			return err
		}
	}
	if len(applied) > 0 {
		if err := realize(ctx, cwm, wt.Request, applied); err != nil {
			return err
		}
		if err := keepApplied(ctx, cwm, wt.Request, applied); err != nil {
			return err
		}
		// What was made goes to its increment (an increment's proposal: the
		// increment itself), whose release plans their work.
		if err := deliverIncrement(ctx, cwm, wt.Request); err != nil {
			return err
		}
	}
	// Dropped meanwhile (while it was recorded): nothing follows, not even a retry.
	if ended(ctx, cwm, wt.Request) {
		return archive(ctx, wt.ID, "collected after its request ended: "+firstLine(r.Summary))
	}
	if r.Status != schedapi.StatusSuccess {
		// An agent's failed check is reworked at once; any agent failure is
		// counted against the cap.
		if handled, err := collectAgentFailure(ctx, cwm, &wt, r); handled || err != nil {
			return err
		}
		// Its report stays on it, typed: what stopped it is read from it
		// (stopOf), the waits line only shows it.
		wt.State, wt.Waits, wt.Claim, wt.Result = schedapi.StateWaiting, r.Status+": "+firstLine(r.Summary), nil, r
		if steps := addedSteps(r); r.Status == schedapi.StatusBlocked && len(steps) > 0 {
			// The steps a blocked report adds are what lifts it (a person's
			// decision, an answer to fetch): planned before it, from now, and
			// it waits on them - back in the queue once they have ended
			//
			return blockedOnAdded(ctx, cwm, wt, steps, followUps(r), by, r.Summary)
		}
		// What a blocked or failed worker found on the way is filed all the
		// same.
		if err := fileFoundOnTheWay(ctx, cwm, &wt, followUps(r), by); err != nil {
			return err
		}
		_, err := put(ctx, wt)
		return err
	}
	req, err := load(ctx, cwm, wt.Request)
	if err != nil {
		return err
	}
	if r.Verdict == api.VerdictApproved && wt.Packet.Confirms != "" {
		if err := confirmRequirement(ctx, wt.Target, wt.Packet.Confirms, by); err != nil {
			return fmt.Errorf("confirm %s on %s: %w", wt.Packet.Confirms, wt.Target, err)
		}
	}
	// Approving verifies what the step names — and so does choosing on the
	// decision a choice opened, the decide step that took the validation's place.
	if (r.Verdict == api.VerdictApproved || (wt.Packet.EndsOn != "" && r.Answer != "")) && wt.Packet.Verifies != "" && by.Kind == "person" {
		if req, err = verifyByApproval(ctx, req, wt); err != nil {
			return fmt.Errorf("verify %q on %s: %w", wt.Packet.Verifies, wt.Request, err)
		}
	}
	plan, err := todosOf(ctx, wt.Request)
	if err != nil {
		return err
	}
	last := 0
	for _, x := range plan {
		last = max(last, x.Position)
	}

	// What comes after it now: the work its result adds, or the work redone —
	// none once its request is dropped (a drop landing while it is collected).
	if ended(ctx, cwm, wt.Request) {
		return archive(ctx, wt.ID, "collected after its request ended: "+firstLine(r.Summary))
	}
	tail, moves := "", wt.Packet.Moves
	// What the report adds: past the cap of what agents add on their own,
	// it waits on a person's validate step; that step approved, it is added
	//
	steps, fus, capped := addedSteps(r), followUps(r), isValidateAdds(wt.Packet)
	var over overflow
	adds := 0
	switch {
	case capped:
		steps, fus = nil, nil
		if r.Verdict == api.VerdictApproved {
			steps, fus = waitingAdds(wt.Packet)
		}
	case countsAgainstCap(by) && len(steps)+len(fus) > 0:
		if adds, err = agentAdds(ctx, wt.Request, plan); err != nil {
			return err
		}
		steps, fus, over = capAdds(adds, steps, fus)
	}
	// Past the rounds an agent reviewer sends the work back on its own, the
	// round is parked and the plan blocked for the person.
	reopened := reopenedByVerify(ctx, cwm, wt, r)
	if how := sendBackHow(wt.Packet, r, reopened); how != "" && decision == "" && !capped {
		done, err := doneOf(ctx, wt.Request)
		if err != nil {
			return err
		}
		sent, reopens := agentRounds(done)
		if reopened {
			sent = reopens
		}
		if sent >= MaxAgentRounds {
			for _, fu := range fus {
				if _, err := followUp(ctx, wt, fu); err != nil {
					return err
				}
			}
			return parkSentBack(ctx, wt, how, strings.TrimSpace(r.Summary+"\n\n"+r.Details), addedSteps(r), over.fus, sent)
		}
		if reopened {
			wt.Packet.Params = maps.Clone(wt.Packet.Params)
			if wt.Packet.Params == nil {
				wt.Packet.Params = map[string]any{}
			}
			wt.Packet.Params[paramReopened] = true
			if _, err := put(ctx, wt); err != nil {
				return err
			}
		}
	}
	switch {
	case wt.Packet.Template == incrementAddRecipe && (r.Verdict == api.VerdictApproved || r.Verdict == api.VerdictRejected):
		// An addition to a running increment: approved, it is aimed; sent
		// back, the proposal ends and nothing is aimed.
		if err := applyAddition(ctx, cwm, req, r.Verdict == api.VerdictApproved, r.Summary, by); err != nil {
			return err
		}
		if r.Verdict == api.VerdictRejected {
			moves = ""
		}
	case r.Verdict == api.VerdictRejected && review != nil:
		// A delivery review that failed: what it found missing is work
		// inside the increment, and the approval is asked again after.
		if err := rejectDelivery(ctx, cwm, req, *review, strings.TrimSpace(r.Summary+"\n\n"+r.Details), by); err != nil {
			return err
		}
		moves = ""
	case r.Verdict == api.VerdictRejected && wt.Packet.Template == incrementAcceptRecipe:
		// A delivery sent back: the note is work inside the increment.
		if err := sendDeliveryBack(ctx, req, strings.TrimSpace(r.Summary+"\n\n"+r.Details), by); err != nil {
			return err
		}
		moves = ""
	case r.Verdict == api.VerdictRejected && !capped:
		if tail, err = rework(ctx, req, wt, plan, last, strings.TrimSpace(r.Summary+"\n\n"+r.Details), by); err != nil {
			return err
		}
		moves = ""
	case decision != "" || len(steps) > 0 || over.any():
		prev := wt.ID
		if decision != "" {
			takesMove := len(steps) == 0 && len(over.steps) == 0 && wt.Packet.Moves != ""
			added, err := planChoice(ctx, req, wt, plan, last, decision, *choiceOf(r), takesMove, by)
			if err != nil {
				return err
			}
			if added != "" {
				prev, last = added, last+1
				if takesMove {
					moves = "" // the node moves when the decision is taken
				}
			}
		}
		if (len(steps) > 0 || len(over.steps) > 0) && wt.Packet.Moves != "" {
			moves = "" // the node moves when the work added ends
		}
		stepsMove := wt.Packet.Moves
		if len(over.steps) > 0 {
			stepsMove = "" // the validate step after them carries it
		}
		if prev, err = addSteps(ctx, req, wt, steps, last, []string{prev}, stepsMove, by, r.Summary); err != nil {
			return err
		}
		last += len(steps)
		if over.any() {
			// Waiting steps hold what came after; waiting follow-ups alone
			// hold nothing: their validate step stands aside.
			after, mv := []string{wt.ID}, ""
			if len(over.steps) > 0 {
				after, mv = []string{prev}, wt.Packet.Moves
			}
			ask, err := askOverflow(ctx, req, wt, over, last+1, after, mv, adds)
			if err != nil {
				return err
			}
			if len(over.steps) > 0 {
				prev = ask
			}
		}
		if prev != wt.ID {
			tail = prev
		}
	}
	// A delivery approved: the increment is done (its own verb - the
	// release rules of its stream follow).
	if r.Verdict == api.VerdictApproved && wt.Packet.Template == incrementAcceptRecipe && review == nil {
		ends, err := approveDelivery(ctx, cwm, wt.Target, by, firstLine(r.Summary))
		if err != nil {
			return err
		}
		if ends != "" {
			moves = "" // the increment does not move: its approval ends on its own
		}
	}
	// An agent tests before the person's review that would verify what no
	// check verifies yet: the review then waits on the test.
	if tail == "" && wt.Packet.Doer != api.DoerPerson {
		if err := testBeforeReview(ctx, req, wt, plan, last); err != nil {
			return err
		}
		if plan, err = todosOf(ctx, wt.Request); err != nil {
			return err
		}
	}
	for _, x := range plan {
		if !notStarted(x) || !slices.Contains(x.After, wt.ID) {
			continue
		}
		if tail != "" {
			x.After = replaceID(x.After, wt.ID, tail)
		}
		if r.Answer != "" {
			x.Packet.Params = maps.Clone(x.Packet.Params)
			if x.Packet.Params == nil {
				x.Packet.Params = map[string]any{}
			}
			x.Packet.Params["answer"] = r.Answer
		}
		passed := review != nil && review.Verdict == api.VerdictApproved
		if passed {
			// The person's approve-delivery shows the review and its demo path.
			reviewToApproval(&x, *review)
		}
		if tail != "" || r.Answer != "" || passed {
			if _, err := put(ctx, x); err != nil {
				return err
			}
		}
	}

	// It is done: archived, the move left to what comes after when it moved.
	if moves != wt.Packet.Moves {
		wt.Packet.Moves = moves
		if _, err := put(ctx, wt); err != nil {
			return err
		}
	}
	if err := archive(ctx, wt.ID, "collected: "+firstLine(r.Summary)); err != nil {
		return err
	}
	for _, fu := range fus {
		if _, err := followUp(ctx, wt, fu); err != nil {
			return err
		}
	}
	if wt.Packet.Step == api.StepIntake {
		if err := replanAfterIntake(ctx, wt.Request, by); err != nil {
			return err
		}
	}
	if err := endRequest(ctx, cwm, wt.Request, by); err != nil {
		return err
	}
	endSplits(ctx, by)
	// Last: what it released — its successors, the work its request held, the
	// requests that depended on it.
	return settleAll(ctx, by)
}

// addSteps plans the steps a result adds, one after the other, the first after
// `after`; the last carries `moves` (the node moves when the work added ends).
// Each says it was added by `by` for the report's line `why`, one generation
// after wt. It answers the last one's id, or after's
// single id when none was added.
func addSteps(ctx context.Context, req cwmapi.Object, wt api.WorkTodo, steps []api.NewStep, last int, after []string, moves string, by cwmapi.Actor, why string) (string, error) {
	prev := ""
	if len(after) == 1 {
		prev = after[0]
	}
	// A collect run again on the same report (a cut collect) finds the steps
	// it already added, keyed by the report and their rank: none twice
	//
	var open []api.WorkTodo
	if len(steps) > 0 {
		var err error
		if open, err = todosOf(ctx, wt.Request); err != nil {
			return "", err
		}
	}
	for i, ns := range steps {
		key := addedKey(wt, i)
		if j := slices.IndexFunc(open, func(x api.WorkTodo) bool { return x.Packet.AddedKey == key }); j >= 0 {
			prev, after = open[j].ID, []string{open[j].ID}
			continue
		}
		p := api.WorkPacket{Step: ns.Step, Doer: ns.Doer, Task: ns.Task, Interaction: ns.Interaction, Instructions: ns.Instructions,
			Phase: ns.Phase, Params: ns.Params}
		if p.Doer == "" {
			p.Doer = api.DoerTask
		}
		if p.Phase == "" {
			p.Phase = wt.Packet.Phase
		}
		if i == len(steps)-1 {
			p.Moves = moves
		}
		p.Added(wt.Packet, by, firstLine(why))
		p.AddedKey = key
		if err := checkPacket(p, wt.Priority, ns.Skill); err != nil {
			return "", invalid("Collect: step %s: %v", ns.Step, err)
		}
		created, err := put(ctx, api.WorkTodo{Todo: schedapi.Todo{
			Module: wt.Module, Target: wt.Target, Request: todoRequestRef(ctx, wt.Todo), Title: ns.Step + " — " + titleOf(req),
			For: forOf(p.Doer), Skill: ns.Skill, Group: api.PlanGroup, Start: startOf(p.Doer), Priority: wt.Priority,
			State: schedapi.StateWaiting, Since: wt.Since, Position: last + i + 1, After: slices.Clone(after),
			Reads: wt.Reads, Writes: wt.Writes,
		}, Packet: p})
		if err != nil {
			return "", err
		}
		prev, after = created.ID, []string{created.ID}
	}
	return prev, nil
}

// addedKey names the i-th step wt's report adds: the to-do, the report's time
// (a later report of the same to-do adds its own steps) and the rank.
func addedKey(wt api.WorkTodo, i int) string {
	at := ""
	if wt.Result != nil {
		at = wt.Result.At
	}
	return fmt.Sprintf("%s@%s#%d", wt.ID, at, i+1)
}

// blockedOnAdded plans the steps a blocked report adds before its to-do: the
// first waits on what the to-do waited on, and the to-do, no longer blocked,
// waits on the last - it comes back to the queue once they have ended.
func blockedOnAdded(ctx context.Context, cwm cwmapi.CWMService, wt api.WorkTodo, steps []api.NewStep, fus []api.FollowUp, by cwmapi.Actor, why string) error {
	req, err := load(ctx, cwm, wt.Request)
	if err != nil {
		return err
	}
	plan, err := todosOf(ctx, wt.Request)
	if err != nil {
		return err
	}
	last := 0
	for _, x := range plan {
		last = max(last, x.Position)
	}
	// Past the cap of what agents add on their own, the rest waits on a
	// person's validate step, which the to-do then waits on.
	var over overflow
	adds := 0
	if countsAgainstCap(by) {
		if adds, err = agentAdds(ctx, wt.Request, plan); err != nil {
			return err
		}
		steps, _, over = capAdds(adds, steps, nil)
	}
	after := slices.Clone(wt.After)
	tail, err := addSteps(ctx, req, wt, steps, last, after, "", by, why)
	if err != nil {
		return err
	}
	if over.any() {
		if len(steps) > 0 {
			after = []string{tail}
		}
		if tail, err = askOverflow(ctx, req, wt, over, last+len(steps)+1, after, "", adds); err != nil {
			return err
		}
	}
	if tail != "" && !slices.Contains(wt.After, tail) {
		wt.After = append(wt.After, tail)
	}
	clearStop(&wt)
	if err := fileFoundOnTheWay(ctx, cwm, &wt, fus, by); err != nil {
		return err
	}
	if _, err := put(ctx, wt); err != nil {
		return err
	}
	return settleRequest(ctx, wt.Request, by)
}

// paramFiledFollowUps counts, on a to-do, the follow-ups its reports that did
// not succeed filed: the cap of what agents add counts them (a success's are
// counted from its result).
const paramFiledFollowUps = "filed_follow_ups"

// checkFollowUps refuses a follow-up that filing would refuse: no title, a
// kind or a priority no request may have, a module that does not exist.
func checkFollowUps(ctx context.Context, cwm cwmapi.CWMService, fus []api.FollowUp) error {
	for _, fu := range fus {
		if strings.TrimSpace(fu.Title) == "" {
			return invalid("every follow-up needs a title")
		}
		if err := checkAsk(fmt.Sprintf("follow-up %q", fu.Title), fu.Kind, fu.Priority, nil, nil); err != nil {
			return err
		}
		if fu.Module == "" {
			continue
		}
		if _, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: fu.Module, ID: fu.Module}); err != nil {
			return invalid("follow-up %q: module %s: %v", fu.Title, fu.Module, err)
		}
	}
	return nil
}

// fileFoundOnTheWay files the follow-ups a blocked or failed report names
// : within the cap of what agents add on their own
// , each filed and counted on the to-do; past it, a
// person's validate step lists them - standing aside, nothing waits on it.
// The caller puts wt.
func fileFoundOnTheWay(ctx context.Context, cwm cwmapi.CWMService, wt *api.WorkTodo, fus []api.FollowUp, by cwmapi.Actor) error {
	if len(fus) == 0 {
		return nil
	}
	var over overflow
	adds := 0
	if countsAgainstCap(by) {
		plan, err := todosOf(ctx, wt.Request)
		if err != nil {
			return err
		}
		if adds, err = agentAdds(ctx, wt.Request, plan); err != nil {
			return err
		}
		_, fus, over = capAdds(adds, nil, fus)
		if over.any() {
			req, err := load(ctx, cwm, wt.Request)
			if err != nil {
				return err
			}
			last := 0
			for _, x := range plan {
				last = max(last, x.Position)
			}
			if _, err := askOverflow(ctx, req, *wt, over, last+1, slices.Clone(wt.After), "", adds); err != nil {
				return err
			}
		}
	}
	for _, fu := range fus {
		if _, err := followUp(ctx, *wt, fu); err != nil {
			return err
		}
	}
	if len(fus) > 0 {
		wt.Packet.Params = maps.Clone(wt.Packet.Params)
		if wt.Packet.Params == nil {
			wt.Packet.Params = map[string]any{}
		}
		wt.Packet.Params[paramFiledFollowUps] = filedFollowUps(wt.Packet) + len(fus)
	}
	return nil
}

// filedFollowUps reads how many follow-ups a to-do's unsuccessful reports filed.
func filedFollowUps(p api.WorkPacket) int {
	switch n := p.Params[paramFiledFollowUps].(type) {
	case int:
		return n
	case float64:
		return int(n)
	case int64:
		return int(n)
	}
	return 0
}

// backToQueue puts a reported to-do back in the queue, not collected, and
// answers why.
func backToQueue(ctx context.Context, wt api.WorkTodo, why error) error {
	wt.State, wt.Waits, wt.Claim, wt.Result = schedapi.StateReady, "", nil, nil
	if _, err := put(ctx, wt); err != nil {
		return err
	}
	return why
}

// collectEnded collects the report of a step whose request ended (done or
// dropped) while it ran: its result is kept on the request, nothing follows
// from it.
func collectEnded(ctx context.Context, cwm cwmapi.CWMService, wt api.WorkTodo) error {
	if err := record(ctx, cwm, wt); err != nil {
		return err
	}
	closeAsk(ctx, wt)
	return archive(ctx, wt.ID, "collected after its request ended: "+firstLine(wt.Result.Summary))
}

// closeAsk closes the Inbox entry a person's step was asked in when the step
// was answered another way (« À faire », the request view, a relayed report):
// still pending, it is answered as the step was - approved or rejected with
// its note, or the answer given - so the Inbox never asks again what is done.
// Answered in the Inbox, it is no longer pending and is left as it is.
func closeAsk(ctx context.Context, t api.WorkTodo) {
	if t.Packet.AskedIn == "" || t.Result == nil {
		return
	}
	inbox, err := application.UseService[inboxapi.InboxService](ctx)
	if err != nil {
		return
	}
	got, err := inbox.Get(ctx, inboxapi.GetInput{ID: t.Packet.AskedIn})
	if err != nil || (got.Entry.Status != "" && got.Entry.Status != inboxapi.StatusPending) {
		return
	}
	r := t.Result
	note := firstLine(r.Summary)
	if r.Answer != "" {
		note = r.Answer
	}
	if note == "" {
		note = "answered in « À faire »"
	}
	in := inboxapi.AnswerInput{ID: t.Packet.AskedIn, Decision: inboxapi.StatusApproved, Note: note, By: r.By.ID}
	if r.Verdict == api.VerdictRejected {
		in.Decision = inboxapi.StatusRejected
	}
	if _, err := inbox.Answer(ctx, in); err != nil {
		slog.Warn("planification: the Inbox entry of an answered step stays open", "todo", t.ID, "entry", t.Packet.AskedIn, "error", err)
	}
}

// ended reports a request whose plan ended — done or dropped.
func ended(ctx context.Context, cwm cwmapi.CWMService, request string) bool {
	o, err := loadObject(ctx, cwm, request)
	if err != nil {
		return false
	}
	f, has := planOf(o)
	return has && planEnded(f.State)
}

// record writes what a step came to on its request: the lasting trace.
func record(ctx context.Context, cwm cwmapi.CWMService, t api.WorkTodo) error {
	o, err := loadObject(ctx, cwm, t.Request)
	if err != nil {
		return err
	}
	f, has := planOf(o)
	if !has {
		return nil
	}
	r := t.Result
	rec := api.StepRecord{Task: t.Packet.Task, Doer: t.Packet.Doer, Step: t.Packet.Template + "/" + t.Packet.Step, Phase: t.Packet.Phase,
		Ask: t.Packet.AskedIn, Outcome: r.Status, Result: r.Summary, Details: r.Details, EndedAt: r.At}
	if t.Packet.Template == "" {
		rec.Step = t.Packet.Step
	}
	rec.AddedBy, rec.AddedWhy, rec.Generation = t.Packet.AddedBy, t.Packet.AddedWhy, t.Packet.Generation
	by := r.By
	rec.By = &by
	if t.Claim != nil {
		rec.Run, rec.Session, rec.StartedAt = t.Claim.Run, t.Claim.Session, t.Claim.At
	}
	if rec.StartedAt == "" {
		rec.StartedAt = r.At
	}
	if r.Verdict != "" {
		rec.Result = r.Verdict + ": " + rec.Result
	}
	if i := slices.IndexFunc(f.Stale, func(s api.StaleStep) bool { return s.Todo == t.ID }); i >= 0 {
		// Its request changed while it ran: what it came to is checked against that.
		rec.Details = strings.TrimSpace("_Ran while its request changed (" + f.Stale[i].Why + "): check it against what changed._\n\n" + rec.Details)
		f.Stale = slices.Delete(f.Stale, i, i+1)
	}
	for _, x := range f.Steps {
		if x.Step == rec.Step && x.EndedAt == rec.EndedAt && x.Outcome == rec.Outcome && x.Result == rec.Result {
			return nil // collected again (a cut collect): recorded once
		}
	}
	f.Steps = append(f.Steps, rec)
	_, err = writePlan(ctx, cwm, o, f, true)
	return err
}

// checkAnswer refuses a result that does not answer what a person's step asks,
// and a verdict on anything else.
func checkAnswer(p api.WorkPacket, r schedapi.Result) error {
	interaction := asks(p)
	if p.Doer != api.DoerPerson && interaction == "" {
		if r.Verdict != "" {
			return invalid("only a validation or a challenge takes a verdict")
		}
		return nil
	}
	switch interaction {
	case api.InteractionValidate, api.InteractionChallenge:
		if r.Verdict != api.VerdictApproved && r.Verdict != api.VerdictRejected {
			return invalid("a %s takes a verdict: approved or rejected", interaction)
		}
		if r.Verdict == api.VerdictRejected && strings.TrimSpace(r.Summary+r.Details) == "" {
			return invalid("a %s rejected says what to change: the summary is empty", interaction)
		}
	case api.InteractionDecide, api.InteractionProvide:
		if r.Answer == "" {
			return invalid("a step that asks a person to %s takes their answer", interaction)
		}
	}
	return nil
}

// asks is what a step's result answers: its packet's interaction, or - a
// review planned before an agent's review carried its verdict
// , its packet naming none - its recipe step's
// validate or challenge. One reading for every step: a review's report with
// no verdict is refused whenever it was planned, never collected as an
// approval.
func asks(p api.WorkPacket) string {
	if p.Interaction != "" || p.Template == "" {
		return p.Interaction
	}
	step := p.Step
	for strings.HasSuffix(step, reworkSuffix) {
		step = strings.TrimSuffix(step, reworkSuffix)
	}
	r, i, err := recipeAt(cwmapi.Object{}, p.Template+"/"+step)
	if err != nil || i < 0 {
		return ""
	}
	switch in := r.Steps[i].Interaction; in {
	case api.InteractionValidate, api.InteractionChallenge:
		return in
	}
	return ""
}

// addedSteps reads the steps a result adds (its more.add).
func addedSteps(r *schedapi.Result) []api.NewStep {
	var out []api.NewStep
	fromMore(r, "add", &out)
	return out
}

// followUps reads the requests a result files (its more.follow_ups).
func followUps(r *schedapi.Result) []api.FollowUp {
	var out []api.FollowUp
	fromMore(r, "follow_ups", &out)
	return out
}

// fromMore decodes a result's more[key] into into, its text cleaned as stored
// text is (LF line ends, doubly encoded UTF-8 repaired: see cleanText).
func fromMore(r *schedapi.Result, key string, into any) {
	if r == nil || r.More == nil || r.More[key] == nil {
		return
	}
	raw, err := json.Marshal(r.More[key])
	if err != nil {
		return
	}
	var generic any
	if json.Unmarshal(raw, &generic) == nil {
		if clean, err := json.Marshal(cleanValue(generic)); err == nil {
			raw = clean
		}
	}
	_ = json.Unmarshal(raw, into)
}

// reworkSuffix names a step done again on a person's feedback.
const reworkSuffix = " (rework)"

// rework has the agent work before a rejected validation done again — each
// with the person's note as feedback — then the validation asked again; it
// answers the new validation, which what came after waits on. Each step
// added says it was added by the one who rejected, for the rejection's line,
// one generation after the validation.
func rework(ctx context.Context, req cwmapi.Object, v api.WorkTodo, plan []api.WorkTodo, last int, feedback string, by cwmapi.Actor) (string, error) {
	done, err := doneOf(ctx, v.Request)
	if err != nil {
		return "", err
	}
	// Redone in the order they were done: a step that came after another
	// redone one (the module update after the implementation) waits on its redo.
	done = slices.Clone(done)
	slices.SortStableFunc(done, func(a, b api.WorkTodo) int { return a.Position - b.Position })
	redo, n, err := redoRound(ctx, req, v, toRedo(done, v), last, feedback, by, "")
	if err != nil {
		return "", err
	}
	p := v.Packet
	p.AskedIn = ""
	p.Added(v.Packet, by, firstLine(feedback))
	n++
	again, err := put(ctx, api.WorkTodo{Todo: schedapi.Todo{
		Module: v.Module, Target: v.Target, Request: todoRequestRef(ctx, v.Todo), Title: v.Title, For: v.For, Skill: v.Skill, Group: v.Group,
		Start: v.Start, Priority: v.Priority, State: schedapi.StateWaiting, Since: v.Since, Position: n, After: redo,
		Reads: v.Reads, Writes: v.Writes,
	}, Packet: p})
	if err != nil {
		return "", err
	}
	_ = plan
	return again.ID, nil
}

// redoRound plans the agent work of toRedo done again, each step with
// feedback, from position last on: the work first, then what checks or
// records it. It answers the steps planned and the last position used.
// round, when set, marks the first step planned as a round counted against
// MaxAgentRounds that no agent report sent back - a failed hand test, a
// failed check at the end - naming its cause.
func redoRound(ctx context.Context, req cwmapi.Object, v api.WorkTodo, toRedo []api.WorkTodo, last int, feedback string, by cwmapi.Actor, round string) ([]string, int, error) {
	var redo []string
	n := last
	// The work done again comes first; what checks or records it - a
	// verification step, the module update - comes after every step done
	// again, so it never runs on the unfixed work (: an
	// added step left the auto-review ahead of the step that fixes).
	var first, then []api.WorkTodo
	for _, d := range toRedo {
		if checksTheWork(d) {
			then = append(then, d)
		} else {
			first = append(first, d)
		}
	}
	redone := map[string]string{}
	var redoneFirst []string
	for i, d := range append(first, then...) {
		var after []string
		for _, a := range d.After {
			if id, ok := redone[a]; ok {
				after = append(after, id)
			}
		}
		if i >= len(first) {
			for _, id := range redoneFirst {
				if !slices.Contains(after, id) {
					after = append(after, id)
				}
			}
		}
		p := d.Packet
		p.Step, p.Starts, p.Moves, p.AskedIn, p.Stale = d.Packet.Step+reworkSuffix, "", "", "", ""
		p.Params = maps.Clone(p.Params)
		if p.Params == nil {
			p.Params = map[string]any{}
		}
		p.Params["feedback"] = feedback
		delete(p.Params, paramFiledFollowUps) // what the step filed stays counted on it, never on its redo
		delete(p.Params, paramRedoRound)      // a round is counted once, on the step that opened it
		if round != "" && i == 0 {
			p.Params[paramRedoRound] = round
		}
		p.Instructions = d.Packet.Instructions + "\n\nDo it again: " + feedback
		p.Added(v.Packet, by, firstLine(feedback))
		n++
		created, err := put(ctx, api.WorkTodo{Todo: schedapi.Todo{
			Module: d.Module, Target: d.Target, Request: todoRequestRef(ctx, d.Todo), Title: p.Step + " — " + titleOf(req), For: d.For, Skill: d.Skill,
			Group: d.Group, Start: d.Start, Priority: d.Priority, State: schedapi.StateWaiting, Since: d.Since, Position: n,
			After: after, Reads: d.Reads, Writes: d.Writes,
		}, Packet: p})
		if err != nil {
			return nil, n, err
		}
		redone[d.ID] = created.ID
		redo = append(redo, created.ID)
		if i < len(first) {
			redoneFirst = append(redoneFirst, created.ID)
		}
	}
	return redo, n, nil
}

// toRedo is the done agent work a rejected validation sends back, sorted by
// position: the done agent steps it came after; when it came after none (its
// after edges lost - a plan undone, a step put back by hand), the request's
// last round of agent build work - the done agent build and check steps since
// the validation before it: a rejection never asks the
// validation again on unchanged work. Empty when the request has no agent
// work to redo: the rejection is refused (nothingToRedo).
func toRedo(done []api.WorkTodo, v api.WorkTodo) []api.WorkTodo {
	agentWork := func(d api.WorkTodo) bool {
		return d.ID != v.ID && d.Packet.Doer != api.DoerPerson && asks(d.Packet) == ""
	}
	var out []api.WorkTodo
	for _, d := range done {
		if slices.Contains(v.After, d.ID) && agentWork(d) {
			out = append(out, d)
		}
	}
	if len(out) > 0 {
		return out
	}
	// The validation before this one closes the earlier round.
	since := -1
	for _, d := range done {
		if d.ID != v.ID && d.Position < v.Position && (d.Packet.Doer == api.DoerPerson || asks(d.Packet) != "") {
			since = max(since, d.Position)
		}
	}
	for _, d := range done {
		if d.Position <= since || (v.Position > 0 && d.Position >= v.Position) || !agentWork(d) {
			continue
		}
		if d.Packet.Phase == api.PhaseBuild || d.Packet.Phase == api.PhaseVerify || checksTheWork(d) {
			out = append(out, d)
		}
	}
	return out
}

// nothingToRedo refuses a rejection that would send back no work: asked
// again on unchanged work, the validation would come back as it was.
func nothingToRedo(ctx context.Context, v api.WorkTodo) error {
	done, err := doneOf(ctx, v.Request)
	if err != nil {
		return err
	}
	if len(toRedo(done, v)) > 0 {
		return nil
	}
	return invalid("a rejection of %s sends back no work: %s has no agent build step done to redo - report blocked with the step to add (more.add), or drop the request", v.Packet.Step, v.Request)
}

// reworksOnReject is a validation whose rejection redoes the work before it
// (rework): not an increment's addition, delivery review or acceptance, nor
// the validate step of what agents add past the cap.
func reworksOnReject(p api.WorkPacket) bool {
	return p.Template != incrementAddRecipe && p.Template != incrementAcceptRecipe &&
		strings.TrimSuffix(p.Step, reworkSuffix) != StepDeliveryReview && !isValidateAdds(p)
}

// checksTheWork is a step that checks or records the work rather than doing
// it: a verification step or the module update. Done again, it waits on the
// work done again.
func checksTheWork(t api.WorkTodo) bool {
	step := strings.TrimSuffix(t.Packet.Step, reworkSuffix)
	return t.Packet.Phase == api.PhaseVerify || step == StepModuleUpdate
}

// followUp files a request a result asks for, from the to-do's module. Only
// the request's own work (own) is linked to it and holds it open; other work
// found on the way is filed on its own, naming where it was found.
func followUp(ctx context.Context, t api.WorkTodo, fu api.FollowUp) (string, error) {
	origin := "A follow-up of " + t.Request + ", step " + t.Packet.Step + "."
	if !fu.Own {
		origin = "Found while working on " + t.Request + ", step " + t.Packet.Step + "."
	}
	in := api.FileInput{
		Title: fu.Title, Module: fu.Module, Kind: fu.Kind, Priority: fu.Priority,
		Details: strings.TrimSpace(fu.Details + "\n\n" + origin),
		By:      cwmapi.Actor{ID: t.Module, Kind: "module"},
	}
	if in.Module == "" {
		in.Module = t.Module
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return "", err
	}
	// A collect run again on the same report (a cut collect) finds the
	// follow-up it already filed - the same title, from the same request's
	// step, still open - and files no second one.
	id := filedFollowUp(ctx, cwm, in.Module, fu.Title, origin)
	if id == "" {
		out, err := New().File(ctx, in)
		if err != nil {
			return "", fmt.Errorf("file the follow-up %q: %w", fu.Title, err)
		}
		id = out.Request.ID
	}
	if !fu.Own {
		return id, nil // not the request's work: it ends without it
	}
	// Linked to the request that filed it: in its tree, and it ends after.
	if o, err := loadObject(ctx, cwm, id); err != nil || !slices.ContainsFunc(o.Edges, func(e cwmapi.ObjectEdge) bool {
		return e.Kind == api.EdgeFollowsUp && e.To == t.Request
	}) {
		if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: id, To: t.Request, Kind: cwmapi.EdgeType(api.EdgeFollowsUp)}); err != nil {
			return id, fmt.Errorf("link the follow-up %s to %s: %w", id, t.Request, err)
		}
	}
	// Linked, it is its origin's work: its to-dos take the labels of the
	// increment that work belongs to (filed first, they were settled without).
	if err := settleRequest(ctx, id, PlanningActor); err != nil {
		return id, err
	}
	return id, nil
}

// filedFollowUp is the open request of module titled title whose details say
// it came from origin (a request's step), or "" when there is none.
func filedFollowUp(ctx context.Context, cwm cwmapi.CWMService, module, title, origin string) string {
	reqs, err := listRequests(ctx, cwm, module)
	if err != nil {
		return ""
	}
	for _, o := range reqs {
		if o.Name == title && !nodeEnded(o) && !planEnded(planState(o)) && strings.HasSuffix(strings.TrimSpace(fieldsOf(o).Details), origin) {
			return o.ID
		}
	}
	return ""
}

// replanAfterIntake plans the request again when its intake changed the
// template its to-dos came from (a depth, a kind).
func replanAfterIntake(ctx context.Context, request string, by cwmapi.Actor) error {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return err
	}
	o, err := load(ctx, cwm, request)
	if err != nil {
		return err
	}
	if st := planState(o); st == api.PlanSplit || planEnded(st) {
		return nil // split or ended by its intake: its parts carry its work, nothing is planned again
	}
	tmpl, err := templateFor(ctx, o)
	if err != nil {
		return err
	}
	plan, err := todosOf(ctx, request)
	if err != nil {
		return err
	}
	for _, t := range plan {
		if notStarted(t) && t.Packet.Template != "" && t.Packet.Template != tmpl.Name {
			_, err := NewWork().Plan(ctx, api.PlanWorkInput{Request: request, Again: true, NoIntake: true, By: by})
			return err
		}
	}
	return nil
}

// endRequest ends a request whose to-dos have all ended, the one that moves its
// node done: its deliverables checked, its module up to date (module_update.go),
// its node moved, its plan done — refused, the request is blocked, saying why,
// or its module-update step planned again.
func endRequest(ctx context.Context, cwm cwmapi.CWMService, request string, by cwmapi.Actor) error {
	// One end at a time per request: a follow-up's end tries its origin's from
	// the plans service and from the observer at once.
	defer lockRequest(request)()
	open, err := todosOf(ctx, request)
	if err != nil || len(open) > 0 {
		return err
	}
	done, err := doneOf(ctx, request)
	if err != nil {
		return err
	}
	moves, about := "", ""
	for _, t := range done {
		if t.Packet.Moves != "" && t.Result != nil && t.Result.Status == schedapi.StatusSuccess {
			moves, about = t.Packet.Moves, t.Target
		}
	}
	if moves == "" && !movesNothing(ctx, request, done) {
		return nil
	}
	o, err := load(ctx, cwm, request)
	if err != nil {
		return err
	}
	if nodeEnded(o) || planEnded(planState(o)) {
		return nil // ended meanwhile
	}
	plans := NewPlans()
	blockedAs := func(kind, why string) error {
		_, err := plans.blockAs(ctx, api.BlockInput{ID: o.ID, Note: "its to-dos are done, but " + why, By: by}, kind)
		return err
	}
	blocked := func(why string) error { return blockedAs(api.BlockEndRefused, why) }
	_, verr := verifyDeliverables(ctx, cwm, o)
	// What is left unverified is only deliverables no check verifies: an agent
	// tests them first (once each), before anything reaches a person.
	if cur, err := load(ctx, cwm, request); err == nil {
		if titles := autoTestAtEnd(fieldsOf(cur), done); len(titles) > 0 {
			return scheduleAutoTest(ctx, cur, titles, PlanningActor, "its to-dos are done but no check verifies "+strings.Join(titles, ", ")+": an agent tests them first")
		}
	}
	if verr != nil {
		// A check failing at the end is agent work:
		// with no open work linked to it to wait on, its last round of agent
		// work is done again, the check's output as feedback, under the cap.
		if cur, err := load(ctx, cwm, request); err == nil && !walkUnverified(cur) && (&holdReader{cwm: cwm}).endWaitsOn(ctx, cur) == "" {
			feedback := "its check failed at its end: " + verr.Error()
			planned, err := reworkAtEnd(ctx, cwm, request, "check failed", feedback, PlanningActor)
			if err != nil {
				return err
			}
			if planned {
				if planState(o) == api.PlanBlocked {
					if _, err := plans.plan(ctx, api.PlanNextInput{ID: o.ID, By: PlanningActor, Note: feedback + " — its work is done again by an agent"}); err != nil {
						return err
					}
				}
				if !collecting(ctx) {
					return settleRequest(ctx, request, PlanningActor)
				}
				return nil
			}
		}
		return blocked(verr.Error())
	}
	// Its checks pass: what is left is its follow-ups — it waits for them.
	if open := openFollowUps(ctx, cwm, o.ID); len(open) > 0 {
		return waitOnFollowUps(ctx, cwm, o, open, by)
	}
	// Its module is up to date before its node moves (a refusal after the
	// move would leave the node done and its request open): what is missing
	// plans its module-update step again, with it as the feedback.
	if o, err = load(ctx, cwm, request); err != nil {
		return err
	}
	if fails, _ := moduleUpdated(ctx, cwm, o, true); len(fails) > 0 {
		return replanModuleUpdate(ctx, o, done, fails, blocked)
	}
	if moves == "" {
		// Its recipe moves nothing (a goal's follow, a verify round): its
		// node stays where it is and its plan ends -
		// a goal's only once every feature serving it is done.
		if left := featuresLeft(ctx, cwm, o); len(left) > 0 {
			return blocked(strings.Join(left, ", ") + " not done yet: " + o.ID + " follows them")
		}
	} else {
		// What proved the work goes on proving the feature - another
		// sidecar's method, outside the change set (its marker stays in this
		// process).
		proveFeature(ctx, cwm, about, fieldsOf(o))
	}
	after, refused, moveRefused, err := endWrites(ctx, cwm, plans, o.ID, about, moves, by)
	if err != nil {
		return err
	}
	if refused != "" {
		kind := api.BlockEndRefused
		if moveRefused && about == o.ID && moves == api.StateDone {
			// Its own node cannot become done: a person moving it settles
			// the end (EndConfirmed).
			kind = api.BlockMoveRefused
		}
		return blockedAs(kind, refused)
	}
	after(ctx)
	if err := verifyRealizedInIncrement(ctx, cwm, about, o, by); err != nil {
		return err
	}
	return askDeliveryApproval(ctx, cwm, o, by)
}

// endCut is a test's cut of a request's end between its node move and its
// plan's end: an error from it stops the end there, as a fault would. Nil
// outside tests.
var endCut func(request string) error

// endWrites writes a request's end in one CWM change set
// : its node moved (moves, on about; none when moves
// is empty) and its plan finished, all written or none - a node never done
// with its request still open, nor the reverse. What follows the end (its
// to-dos archived, its origin, what waited on it) is answered as after, to run
// once it is written: it lists, and a list does not see what a set staged.
// refused is why the end is refused (the move or the finish): nothing is
// written, the caller blocks the request outside the set. An error is a
// fault - a commit's conflict, a cut: nothing is written, and the next end
// (collect again, the reconciler) does it all.
func endWrites(ctx context.Context, cwm cwmapi.CWMService, plans *Plans, request, about, moves string, by cwmapi.Actor) (after func(context.Context), refused string, moveRefused bool, err error) {
	set := "end-request:" + request
	if _, err := cwm.ChangeSetBegin(ctx, cwmapi.ChangeSetBeginInput{Set: set}); err != nil {
		return nil, "", false, err
	}
	discard := func() {
		if _, err := cwm.ChangeSetDiscard(ctx, cwmapi.ChangeSetDiscardInput{Set: set}); err != nil {
			slog.Warn("planification: a request's end change set was not discarded", "request", request, "error", err)
		}
	}
	sctx := cwmapi.WithChangeSet(ctx, set)
	if moves != "" {
		if err := moveEnded(sctx, cwm, about, moves, by, "its work is done: "+request); err != nil {
			discard()
			return nil, fmt.Sprintf("%s cannot become %s: %v", about, moves, err), true, nil
		}
	}
	if endCut != nil {
		if err := endCut(request); err != nil {
			discard()
			return nil, "", false, err
		}
	}
	_, after, err = plans.finishWrites(sctx, api.FinishInput{ID: request, By: by, Note: "its to-dos are done"})
	if err != nil {
		discard()
		return nil, err.Error(), false, nil
	}
	if _, err := cwm.ChangeSetCommit(ctx, cwmapi.ChangeSetCommitInput{Set: set}); err != nil {
		discard()
		return nil, "", false, err
	}
	return after, "", false, nil
}

// movesNothing reports a request whose recipe moves nothing at its end (a
// goal's follow, a realized feature's verify round) and whose recipe's last
// step succeeded - steps added after it included: nothing else will end it.
func movesNothing(ctx context.Context, request string, done []api.WorkTodo) bool {
	module, _ := cwmapi.ModuleOfObjectID(request)
	recipes, err := templatesFor(ctx, module)
	if err != nil {
		return false
	}
	for _, t := range done {
		if t.Result == nil || t.Result.Status != schedapi.StatusSuccess || t.Packet.Template == "" {
			continue
		}
		for _, r := range recipes {
			if r.Name == t.Packet.Template && r.Transition == "" && len(r.Steps) > 0 && r.Steps[len(r.Steps)-1].Name == t.Packet.Step {
				return true
			}
		}
	}
	return false
}

// featuresLeft is, for a goal, the features serving it or under it that are
// neither done nor dropped - what its follow still waits on; nil otherwise.
func featuresLeft(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object) []string {
	if o.Type != goalType {
		return nil
	}
	var left []string
	for _, f := range featuresOfGoal(ctx, cwm, o.ID) {
		fo, err := loadObject(ctx, cwm, f)
		if err != nil {
			continue
		}
		if st := lifeState(fo); st != api.StateDone && st != api.StateDropped {
			left = append(left, f)
		}
	}
	return left
}

// featuresOfGoal is the features serving a goal and those under it, each once.
func featuresOfGoal(ctx context.Context, cwm cwmapi.CWMService, goal string) []string {
	var out []string
	serving, _ := incoming(ctx, cwm, goal, api.EdgeServes)
	for _, f := range serving {
		if fo, err := loadObject(ctx, cwm, f); err == nil && fo.Type == featureType && !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	if kids, err := childrenOf(ctx, cwm, goal, featureType); err == nil {
		for _, k := range kids {
			if !slices.Contains(out, k.ID) {
				out = append(out, k.ID)
			}
		}
	}
	return out
}

// EndDroppedPlan ends the plan of a goal or a feature moved dropped
// : its work is dropped with it - its to-dos not
// started end (EndTodos), none left for an agent to pull nor counted for its
// increment or release; a step already running finishes as any dropped
// request's does. Called on the object's transitioned event.
func EndDroppedPlan(ctx context.Context, id, to string) error {
	if to != api.StateDropped {
		return nil
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return err
	}
	o, err := loadObject(ctx, cwm, id)
	if err != nil || (o.Type != featureType && o.Type != goalType) || !hasRequest(o) || planEnded(planState(o)) {
		return nil
	}
	_, err = NewPlans().Finish(ctx, api.FinishInput{ID: id, Outcome: api.PlanDropped, By: PlanningActor, Note: "its " + o.Type + " was dropped: its work is not done"})
	return err
}

// EndFollowedGoals tries again the end of the goals a feature serves or sits
// under once it moved done or dropped: a goal's follow refused while it was
// not. Called on the feature's transitioned event.
func EndFollowedGoals(ctx context.Context, id, to string) error {
	if to != api.StateDone && to != api.StateDropped {
		return nil
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return err
	}
	f, err := loadObject(ctx, cwm, id)
	if err != nil || f.Type != featureType {
		return nil
	}
	goals := edgesTo(f, api.EdgeServes)
	if f.Parent != "" {
		goals = append(goals, f.Parent)
	}
	for _, g := range goals {
		o, err := loadObject(ctx, cwm, g)
		if err != nil || o.Type != goalType || !hasRequest(o) || planEnded(planState(o)) || len(featuresLeft(ctx, cwm, o)) > 0 {
			continue
		}
		// Its follow reported blocked on a feature not done: every one is
		// done now, so it is offered again - nothing else would re-queue it.
		todos, err := todosOf(ctx, g)
		if err != nil {
			return err
		}
		for _, t := range todos {
			if t.Packet.Template == goalFollowRecipe && stopStatus(t) == schedapi.StatusBlocked {
				if _, err := NewWork().Retry(ctx, api.RetryWorkInput{ID: t.ID, By: PlanningActor, Note: "its features are done"}); err != nil {
					return err
				}
			}
		}
		if err := endRequest(ctx, cwm, g, PlanningActor); err != nil {
			return err
		}
	}
	return nil
}

// verifyRealizedInIncrement files the verification of a feature its build
// recorded realized, when that build was an open increment's work: an
// agent's review (feature-verify) in the same increment, at the build's
// priority, so the feature ends done - or goes back with its findings - and
// never holds its increment open as realized. Outside
// an open increment nothing is filed: a realized feature waits until an
// increment needs it.
func verifyRealizedInIncrement(ctx context.Context, cwm cwmapi.CWMService, feature string, request cwmapi.Object, by cwmapi.Actor) error {
	f, err := loadObject(ctx, cwm, feature)
	if err != nil || f.Type != featureType || ownState(f) != featureRealized {
		return nil
	}
	inc, _ := releaseOfWork(ctx, cwm, request)
	if inc == "" {
		return nil
	}
	if io, err := loadIncrement(ctx, cwm, inc); err != nil || !open(releaseState(io)) {
		return nil
	}
	_, err = New().File(ctx, api.FileInput{
		On:        feature,
		Kind:      api.KindReview,
		Title:     "Verify the realized feature",
		Details:   "Its build ended in " + inc + " with the feature recorded realized. Verify it is really implemented - run its acceptance criteria, read its code - then move it done (transition verify) or send it back (transition reopen) with what is missing, and file that work.",
		Priority:  fieldsOf(request).Priority,
		Increment: inc,
		By:        by,
	})
	return err
}

// EndConfirmed ends the plan a refused end left blocked once what it is
// about moved to done: its to-dos were done, but the
// object could not become done as an agent (confirm is a person's); the
// person's confirm in its Lifecycle tab settles it, so « À faire » no longer
// asks for it. Called on the object's transitioned event, reading it fresh.
func EndConfirmed(ctx context.Context, id, to string) error {
	if to != api.StateDone {
		return nil
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return err
	}
	o, err := loadObject(ctx, cwm, id)
	if err != nil || ownState(o) != api.StateDone || planState(o) != api.PlanBlocked {
		return nil
	}
	if f, _ := planOf(o); blockKind(f) != api.BlockMoveRefused {
		return nil
	}
	_, err = NewPlans().Finish(ctx, api.FinishInput{ID: id, By: planner, Note: "its to-dos are done and it was confirmed done"})
	return err
}

// A feature's type and the state its build ends in when done is out of reach
// (the method's `feature` workflow).
const (
	featureType     = "feature"
	featureActive   = "active"
	featureRealized = "realized"
)

// moveEnded moves the node a request's end moves. A feature whose build
// ended but which cannot become done - it carries no lifecycle state yet, or
// its workflow has no way to done from where it stands - is recorded realized:
// built, a person confirms it. Built work never shows as not built.
func moveEnded(ctx context.Context, cwm cwmapi.CWMService, about, moves string, by cwmapi.Actor, note string) error {
	err := moveNode(ctx, cwm, about, moves, by, note)
	if moves != api.StateDone {
		return err
	}
	o, lerr := loadObject(ctx, cwm, about)
	if lerr != nil || o.Type != featureType {
		return err
	}
	if st := ownState(o); err == nil && st != "" {
		return nil // moved (or already there)
	}
	if _, rerr := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: about, To: featureRealized, By: by,
		Note: note + " - recorded realized: a person confirms it done"}); rerr != nil {
		if err != nil {
			return err
		}
		return rerr
	}
	// Its criteria pass and nothing of it is tested by hand: verified
	// without the person.
	verifyRealized(ctx, cwm, about)
	return nil
}

// cwm0 is the CWM service, or nil when there is none (openFollowUps answers
// nothing then).
func cwm0(ctx context.Context) cwmapi.CWMService {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return nil
	}
	return cwm
}

// openFollowUps is the follow-ups of a request not ended yet.
func openFollowUps(ctx context.Context, cwm cwmapi.CWMService, request string) []string {
	if cwm == nil {
		return nil
	}
	got, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Capability: api.CapPlan})
	if err != nil {
		return nil
	}
	var open []string
	for _, x := range got.Objects {
		if x.EdgeTo(api.EdgeFollowsUp) == request && !planEnded(planState(x)) {
			open = append(open, x.ID)
		}
	}
	return open
}

// waitOnFollowUps parks a request whose own steps are done until its
// follow-ups have ended: idle, its note naming them — nothing for a person to
// do, unlike a block.
func waitOnFollowUps(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object, open []string, by cwmapi.Actor) error {
	note := "its own steps are done; it waits on its follow-ups: " + strings.Join(open, ", ")
	set := func(f *api.PlanFields) { f.Note = note }
	if st := planState(o); st == api.PlanIdle || st == api.PlanSplit {
		// Already waiting (idle), or split (its parts and its follow-ups end
		// it): only its note says on what.
		f, _ := planOf(o)
		set(&f)
		_, err := writePlan(ctx, cwm, o, f, true)
		return err
	}
	_, err := NewPlans().move(ctx, o.ID, api.PlanIdle, "wait", by, note, set)
	return err
}

// EndOrigin tries again the end of the request a follow-up was filed by, once
// the follow-up ended — by the plans service or moved by hand.
func EndOrigin(ctx context.Context, followUp cwmapi.Object, by cwmapi.Actor) {
	origin := followUp.EdgeTo(api.EdgeFollowsUp)
	if origin == "" {
		return
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return
	}
	if err := endRequest(ctx, cwm, origin, by); err != nil {
		slog.Warn("planification: the request a follow-up was filed by did not end", "request", origin, "follow-up", followUp.ID, "error", err)
	}
}

// collectingKey marks a context inside Collect: it settles once, at its end.
type collectingKey struct{}

func collecting(ctx context.Context) bool { return ctx.Value(collectingKey{}) != nil }

// endSplits ends each split demand whose parts have all ended: done if any
// part is done, dropped if they all were.
func endSplits(ctx context.Context, by cwmapi.Actor) {
	plans := NewPlans()
	all, err := plans.List(ctx, api.ListPlansInput{})
	if err != nil {
		return
	}
	for _, p := range all.Plans {
		if p.State != api.PlanSplit {
			continue
		}
		if len(openFollowUps(ctx, cwm0(ctx), p.ID)) > 0 {
			continue // its follow-ups end first
		}
		parts, done, open := 0, false, false
		for _, q := range all.Plans {
			if q.SplitFrom != p.ID {
				continue
			}
			parts++
			open = open || !planEnded(q.State)
			done = done || q.State == api.PlanDone
		}
		if parts == 0 || open {
			continue
		}
		outcome := api.PlanDropped
		if done {
			outcome = api.PlanDone
		}
		if _, err := plans.Finish(ctx, api.FinishInput{ID: p.ID, Outcome: outcome, By: by, Note: fmt.Sprintf("its %d parts ended", parts)}); err != nil {
			slog.Warn("planification: a split demand whose parts all ended did not end", "demand", p.ID, "outcome", outcome, "error", err)
		}
	}
}

// replaceID replaces one id in a list.
func replaceID(ids []string, old, with string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == old {
			id = with
		}
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}

// incrementAcceptRecipe is the person's approval of an increment's delivery.
const incrementAcceptRecipe = "increment-accept"

// askDeliveryApproval files the person's approval of an increment's delivery
// as the last of its work ends: the increment the ended
// request's work belongs to is open, not on hold, holds goals or features and
// every one of them is done, and no request of its own is open. One card - its
// pane - lists each delivery goal with what it became; approving it ends the
// increment done.
func askDeliveryApproval(ctx context.Context, cwm cwmapi.CWMService, request cwmapi.Object, by cwmapi.Actor) error {
	inc, _ := releaseOfWork(ctx, cwm, request)
	if inc == "" || inc == request.ID {
		return nil
	}
	io, err := loadIncrement(ctx, cwm, inc)
	if err != nil || releaseState(io) != api.IncrementOpen || heldIncrement(ctx, inc) != nil {
		return nil
	}
	if hasRequest(io) && !planEnded(planState(io)) {
		return nil // its proposal, or its approval, is open
	}
	scope, err := scopeOf(ctx, cwm, io)
	if err != nil || scope.Total == 0 || scope.Done < scope.Total {
		return nil
	}
	if _, err = New().File(ctx, api.FileInput{
		On:      inc,
		Kind:    api.KindReview,
		Title:   "Approve the delivery of " + titleOf(io),
		Details: "Everything " + inc + " holds is done. An agent first reviews that the user can use what it delivers (a person where its release or stream asks it); then read its pane - each delivery goal, what it became and the requests that did it, the review's result and its demo path - and approve its delivery (it is done) or send it back saying which goal is not met.",
		By:      PlanningActor,
	}); err != nil {
		return err
	}
	// Nothing to assess: the approval is asked at once.
	_, err = NewWork().Plan(ctx, api.PlanWorkInput{Request: inc, Again: true, NoIntake: true, By: PlanningActor})
	return err
}

// AskDeliveryOnMove asks the approval of the delivery of the increment a goal
// or a feature belongs to once it moved outside a request's end - a person's
// confirm in its Lifecycle tab or from « À faire »:
// asked only as a request's work ended, an increment whose last feature a
// person confirmed stayed delivering with nothing asked. Called on the
// object's transitioned event; askDeliveryApproval asks once.
func AskDeliveryOnMove(ctx context.Context, id string) error {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return err
	}
	o, err := loadObject(ctx, cwm, id)
	if err != nil || (o.Type != featureType && o.Type != goalType) {
		return nil
	}
	return askDeliveryApproval(ctx, cwm, o, PlanningActor)
}

// approveDelivery makes an increment whose delivery a person approved done
// . The approval is recorded whatever the increment came
// to meanwhile, and its collect never fails on it: already done (closed by its
// verb) the plan ends as usual - the node is where the approval takes it; work
// that joined it after its delivery was asked holds it open, and a dropped
// increment never moves: the approval's plan ends saying so - the delivery is
// asked again once that work ends - and that note is answered (empty when the
// approval moves the increment as usual).
func approveDelivery(ctx context.Context, cwm cwmapi.CWMService, id string, by cwmapi.Actor, summary string) (string, error) {
	io, err := loadIncrement(ctx, cwm, id)
	if err != nil {
		return "", err
	}
	note := ""
	switch releaseState(io) {
	case api.IncrementDone:
		return "", nil
	case api.IncrementDropped:
		note = "its delivery approved, but it was dropped meanwhile"
	default:
		why, err := heldBack(ctx, cwm, io)
		if err != nil {
			return "", err
		}
		if len(why) == 0 {
			if gate := usableDelivered(ctx, cwm, io); !gate.Accept {
				// Not usable-delivered: it does not move
				// the person forces it with a note, or its review runs again.
				note = "its delivery approved, but " + GateUsableDelivered + " refuses it (" + gate.Reason + "): force it with a note (increments done --force) or send it back"
				break
			}
			if _, err := NewIncrements().Done(ctx, api.IncrementMoveInput{ID: id, By: by, Note: "its delivery approved: " + summary}); err != nil {
				return "", fmt.Errorf("the delivery of %s approved: %w", id, err)
			}
			return "", nil
		}
		note = "its delivery approved, but work joined it since it was asked (" + strings.Join(why, "; ") + "): asked again once that work ends"
	}
	if _, err := NewPlans().Finish(ctx, api.FinishInput{ID: id, By: by, Note: note}); err != nil {
		return "", fmt.Errorf("the delivery of %s approved: %w", id, err)
	}
	return note, nil
}

// sendDeliveryBack files a person's rejected delivery as a fix inside the
// increment, and ends the approval: it is asked again once that work ends.
func sendDeliveryBack(ctx context.Context, approval cwmapi.Object, note string, by cwmapi.Actor) error {
	title := firstLine(note)
	if title == "" {
		title = "What the delivery of " + approval.ID + " misses"
	}
	module, _ := cwmapi.ModuleOfObjectID(approval.ID)
	// The review it was asked on no longer stands: the next one is run again.
	if cwm, err := cwmOf(ctx); err == nil {
		if err := recordDeliveryReview(ctx, cwm, approval.ID, nil); err != nil {
			return fmt.Errorf("send the delivery of %s back: %w", approval.ID, err)
		}
	}
	if _, err := New().File(ctx, api.FileInput{
		Title: title, Details: "The person sent the delivery of " + approval.ID + " back:\n\n" + note,
		Kind: api.KindFix, Module: module, Increment: approval.ID, By: by,
	}); err != nil {
		return fmt.Errorf("send the delivery of %s back: %w", approval.ID, err)
	}
	_, err := NewPlans().Finish(ctx, api.FinishInput{ID: approval.ID, Outcome: api.PlanDropped, By: by,
		Note: "its delivery was sent back: " + title})
	return err
}
