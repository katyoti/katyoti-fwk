// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// AN AGENT REVIEWER SENDS WORK BACK AT MOST MaxAgentRounds TIMES PER REQUEST
// : a round is an agent's report that sends the work
// back - a validation it rejects (its rework and the validation asked
// again), a check it fails (its rework), a
// verify-phase step (a review, an auto-review) whose report adds steps, an agent's reopen of a realized feature in its verify round, or
// a failed hand test or check at the end sent back to agents -
// counted from the request's done to-dos, whose reports say who sent it
// back (the provenance its added steps carry). The
// round past the cap is not planned: the step that would send the work back
// is parked - what comes after it keeps waiting - and the request's plan is
// blocked with the reason, which « À faire » shows the person. Resumed, the
// parked round is planned as the person's (uncounted): one more round, and
// the next agent send-back is blocked again. A person's rejected validation
// is never capped.

// MaxAgentRounds is how many times an agent reviewer sends a request's work
// back on its own.
const MaxAgentRounds = 2

// The packet params of a parked send-back: what it sends back, and how.
const (
	paramSentBack    = "sent_back"     // the reviewer's feedback (summary and details)
	paramSentBackHow = "sent_back_how" // reject, adds or reopen
	paramReopened    = "reopened"      // a feature's verify step whose agent reopened the feature
	paramRedoRound   = "redo_round"    // the first step of a round no agent report sent back: why (a failed hand test, a failed check)
	sentBackReject   = "reject"        // a rejected validation: its rework
	sentBackAdds     = "adds"          // a verify step's added steps
	sentBackReopen   = "reopen"        // a realized feature reopened by its verify round
)

// sendsBack says an agent's report on a step sends its request's work back:
// a rejected validation (not an increment's own addition, acceptance or
// delivery review, which have their own paths; not the person's
// validate-adds), or a verify-phase step whose report adds steps.
func sendsBack(p api.WorkPacket, r *schedapi.Result) bool {
	if r != nil && r.Status == schedapi.StatusFail && countsAgainstCap(r.By) && reworksOnFail(p) {
		// An agent's failed check sends the work back too: its rework was
		// planned.
		return true
	}
	if r == nil || r.Status != schedapi.StatusSuccess || !countsAgainstCap(r.By) {
		return false
	}
	switch {
	case p.Template == incrementAddRecipe, p.Template == incrementAcceptRecipe, isValidateAdds(p),
		strings.TrimSuffix(p.Step, reworkSuffix) == StepDeliveryReview:
		return false
	case r.Verdict == api.VerdictRejected:
		return true
	}
	return p.Phase == api.PhaseVerify && len(addedSteps(r)) > 0
}

// sendBackHow is how an agent's report sends the work back - a feature's
// reopen, a rejection's rework, a verify step's added steps - or "".
func sendBackHow(p api.WorkPacket, r *schedapi.Result, reopened bool) string {
	switch {
	case reopened:
		return sentBackReopen
	case !sendsBack(p, r):
		return ""
	case r.Verdict == api.VerdictRejected:
		return sentBackReject
	}
	return sentBackAdds
}

// agentRounds counts the rounds agents sent a request's work back: its done
// to-dos whose agent report sent it back, and its feature verify steps whose
// agent reopened the feature - kept apart (reopens answers the second).
func agentRounds(done []api.WorkTodo) (sent, reopens int) {
	for _, d := range done {
		if d.Packet.Params[paramReopened] == true {
			reopens++
			continue
		}
		// A round opened by a failed hand test or a failed check at the end
		// : counted on its first step.
		if round, _ := d.Packet.Params[paramRedoRound].(string); round != "" {
			sent++
			continue
		}
		if sendsBack(d.Packet, d.Result) {
			sent++
		}
	}
	return sent, reopens
}

// reopenedByVerify says an agent's report on a realized feature's verify
// round left the feature reopened (neither done nor realized).
func reopenedByVerify(ctx context.Context, cwm cwmapi.CWMService, wt api.WorkTodo, r *schedapi.Result) bool {
	if wt.Packet.Template != recipeFeatureVerify || r == nil || r.Status != schedapi.StatusSuccess || !countsAgainstCap(r.By) {
		return false
	}
	o, err := loadObject(ctx, cwm, wt.Target)
	if err != nil || o.Type != featureType {
		return false
	}
	st := ownState(o)
	return st != "" && st != api.StateDone && st != featureRealized
}

// parkSentBack parks the step whose report would send the work back past the
// cap: it stays in the queue, not ready - what comes after it keeps waiting -
// holding what it sends back (its added steps, and the follow-ups its report
// files past the add cap -), and the request's plan is
// blocked with the reason for the person.
func parkSentBack(ctx context.Context, wt api.WorkTodo, how, feedback string, steps []api.NewStep, fus []api.FollowUp, rounds int) error {
	who := "an agent reviewer sent the work back"
	if how == sentBackReopen {
		who = "an agent reopened the feature in its verify round"
	}
	reason := fmt.Sprintf("%s past the cap of %d rounds (%d already): %s", who, MaxAgentRounds, rounds, firstLine(feedback))
	wt.Packet.Params = maps.Clone(wt.Packet.Params)
	if wt.Packet.Params == nil {
		wt.Packet.Params = map[string]any{}
	}
	wt.Packet.Params[paramSentBack], wt.Packet.Params[paramSentBackHow] = feedback, how
	if len(steps) > 0 {
		wt.Packet.Params[paramWaitingAdds] = steps
	}
	if len(fus) > 0 {
		wt.Packet.Params[paramWaitingFollowUps] = fus
	}
	wt.State, wt.Waits, wt.Claim, wt.Result = schedapi.StateWaiting, parkedWaits, nil, nil
	if _, err := put(ctx, wt); err != nil {
		return err
	}
	_, err := NewPlans().blockAs(ctx, api.BlockInput{ID: wt.Request, Note: reason, By: PlanningActor}, api.BlockParked)
	return err
}

// parkedWaits is why a parked send-back is not ready.
const parkedWaits = "parked: an agent reviewer sent the work back past its cap - the person resumes the request for one more round, or drops it"

// parked says a to-do holds a send-back past the cap.
func parked(t api.WorkTodo) bool {
	_, ok := t.Packet.Params[paramSentBackHow].(string)
	return ok
}

// resumeSentBack plans, as the person's round, the send-backs a request
// resumed by `by` had parked: a rejection's rework and the validation asked
// again, a verify step's added steps; a feature's reopen ends its round. The
// follow-ups its report held past the add cap are filed: the person resuming
// approves them as a validate-adds step would. The parked step is then
// archived; what came after it waits on the work planned.
func resumeSentBack(ctx context.Context, request string, by cwmapi.Actor) error {
	todos, err := todosOf(ctx, request)
	if err != nil {
		return err
	}
	var held []api.WorkTodo
	for _, t := range todos {
		if parked(t) {
			held = append(held, t)
		}
	}
	if len(held) == 0 {
		return nil
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return err
	}
	req, err := load(ctx, cwm, request)
	if err != nil {
		return err
	}
	for _, wt := range held {
		plan, err := todosOf(ctx, request)
		if err != nil {
			return err
		}
		last := 0
		for _, x := range plan {
			last = max(last, x.Position)
		}
		how, _ := wt.Packet.Params[paramSentBackHow].(string)
		feedback, _ := wt.Packet.Params[paramSentBack].(string)
		steps, fus := waitingAdds(wt.Packet)
		wt.Packet.Params = maps.Clone(wt.Packet.Params)
		for _, k := range []string{paramSentBack, paramSentBackHow, paramWaitingAdds, paramWaitingFollowUps} {
			delete(wt.Packet.Params, k)
		}
		for _, fu := range fus {
			if _, err := followUp(ctx, wt, fu); err != nil {
				return err
			}
		}
		why := "resumed for one more round: " + firstLine(feedback)
		tail := ""
		switch how {
		case sentBackReject:
			if tail, err = rework(ctx, req, wt, plan, last, feedback, by); err != nil {
				return err
			}
			wt.Packet.Moves = ""
		case sentBackAdds:
			if tail, err = addSteps(ctx, req, wt, steps, last, []string{wt.ID}, wt.Packet.Moves, by, why); err != nil {
				return err
			}
			if tail == wt.ID {
				tail = ""
			} else {
				wt.Packet.Moves = ""
			}
		}
		if tail != "" {
			for _, x := range plan {
				if x.ID == wt.ID || !notStarted(x) || !slices.Contains(x.After, wt.ID) {
					continue
				}
				x.After = replaceID(x.After, wt.ID, tail)
				if _, err := put(ctx, x); err != nil {
					return err
				}
			}
		}
		clearStop(&wt)
		if _, err := put(ctx, wt); err != nil {
			return err
		}
		if err := archive(ctx, wt.ID, why); err != nil {
			return err
		}
	}
	return settleAll(ctx, by)
}
