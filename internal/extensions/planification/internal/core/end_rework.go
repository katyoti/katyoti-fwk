// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"slices"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// AN END ONLY AGENT WORK CAN MOVE IS NOT THE PERSON'S:
// « À faire » asks the person only for what the person alone moves.
//   - A failed hand test with no validation waiting (the request blocked by
//     its refused end) sends the request's last round of agent work back at
//     once, the failure as its feedback, and the plan resumes: the person
//     tests again once the rework has landed (its end refused again on the
//     deliverable still unverified - the test card).
//   - A check failing at the end does the same, its output as the feedback -
//     unless the request waits on open work linked to it (its follow-ups,
//     its parts, what it depends on), which the hold reader names, or the
//     check is a live GUI walk, which its GUI batch runs.
//   - An end refused while such open work is not done - an increment's own
//     work included - waits on that work: no card for the person.
// Each is a round counted against MaxAgentRounds; past the cap, or with no
// agent work to do again, the end is blocked for the person, as before.

// reworkAtEnd plans, under the cap, the request's last round of agent work
// done again with feedback, round naming why (a failed hand test, a failed
// check). It answers whether it planned it.
func reworkAtEnd(ctx context.Context, cwm cwmapi.CWMService, request, round, feedback string, by cwmapi.Actor) (bool, error) {
	done, err := doneOf(ctx, request)
	if err != nil {
		return false, err
	}
	open, err := todosOf(ctx, request)
	if err != nil {
		return false, err
	}
	if sent, _ := agentRounds(append(slices.Clone(done), open...)); sent >= MaxAgentRounds {
		return false, nil
	}
	done = slices.Clone(done)
	slices.SortStableFunc(done, func(a, b api.WorkTodo) int { return a.Position - b.Position })
	last := 0
	for _, t := range append(slices.Clone(done), open...) {
		last = max(last, t.Position)
	}
	// The whole round of agent work before it, not only the steps it came
	// right after (an auto-test put before a review would be all).
	v := lastValidation(done, last)
	v.After = nil
	redo := toRedo(done, v)
	if len(redo) == 0 {
		return false, nil
	}
	req, err := load(ctx, cwm, request)
	if err != nil {
		return false, err
	}
	if _, _, err := redoRound(ctx, req, v, redo, last, feedback, by, round); err != nil {
		return false, err
	}
	return true, nil
}

// walkUnverified says a request has a deliverable not verified whose check
// runs a live GUI walk: the GUI batch runs it, never an agent's rework.
func walkUnverified(o cwmapi.Object) bool {
	for _, d := range fieldsOf(o).Deliverables {
		if !d.Verified && len(liveScripts(d.Check)) > 0 {
			return true
		}
	}
	return false
}

// lastValidation is the request's last done validation (a validate or a
// challenge, a person's or an agent reviewer's) when no build work was done
// after it: the round sent back is the agent work before it, as its
// rejection would. Otherwise - none, or a round already redone after it - a
// step after every done one, so the round is the agent work since the last
// validation.
func lastValidation(done []api.WorkTodo, last int) api.WorkTodo {
	var v *api.WorkTodo
	for i := range done {
		d := done[i]
		if (d.Packet.Interaction == api.InteractionValidate || d.Packet.Interaction == api.InteractionChallenge) &&
			(v == nil || d.Position > v.Position) {
			v = &done[i]
		}
	}
	if v != nil && !slices.ContainsFunc(done, func(d api.WorkTodo) bool {
		return d.Position > v.Position && d.Packet.Doer != api.DoerPerson && d.Packet.Interaction == "" && d.Packet.Phase == api.PhaseBuild
	}) {
		return *v
	}
	return api.WorkTodo{Todo: schedapi.Todo{Position: last + 1}}
}

// ownHold is a person's own block of a plan: its
// last block, by a person, by hand - not its end refused, not a failed hand
// test, not a send-back parked, which are not a hold. It answers that
// block's entry.
func ownHold(pf *api.PlanFields) (cwmapi.LifecycleEntry, bool) {
	if pf == nil || blockKind(*pf) != api.BlockManual {
		return cwmapi.LifecycleEntry{}, false
	}
	for i := len(pf.Trail) - 1; i >= 0; i-- {
		e := pf.Trail[i]
		if e.To != api.PlanBlocked {
			continue
		}
		if e.Transition != "block" || e.By.Kind != api.ActorPerson {
			return cwmapi.LifecycleEntry{}, false
		}
		return e, true
	}
	return cwmapi.LifecycleEntry{}, false
}

// endWaitsOnWork begins the reason of an end refused that waits on open work.
const endWaitsOnWork = "its end waits on the open work linked to it: "

// handTestFailed begins the note of a plan blocked by a failed hand test.
const handTestFailed = "a hand test failed: "

// endWaitsOn is the first open work a request blocked by its refused end
// waits on - its own follow-ups, its parts, what it depends on, an
// increment's own work - or "" when none is open: that work moves it, never
// the person.
func (r *holdReader) endWaitsOn(ctx context.Context, o cwmapi.Object) string {
	openWork := func(x cwmapi.Object) bool {
		return x.ID != o.ID && hasRequest(x) && !planEnded(planState(x))
	}
	for _, x := range r.followUpsOf(ctx, o.ID) {
		if openWork(x) {
			return x.ID
		}
	}
	for _, x := range r.partsOf(ctx, o.ID) {
		if openWork(x) {
			return x.ID
		}
	}
	for _, dep := range dependsOn(o) {
		if d, _, err := loadAny(ctx, r.cwm, dep); err == nil && !waitEnded(d) {
			return dep
		}
	}
	if o.Type == api.IncrementType {
		for _, x := range incrementWork(ctx, r.cwm, o) {
			if openWork(x) && planState(x) != api.PlanSplit {
				return x.ID
			}
		}
	}
	return ""
}
