// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"maps"
	"slices"
	"strings"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A FAILED AGENT STEP IS AGENT WORK: what only a
// person can move goes to « À faire »; a step an agent failed is not that.
//   - An agent's check step (an auto-review, an auto-test) that
//     fails has its rework planned at once, as a rejection's: the agent work
//     before it done again with the failure as feedback, then the check asked
//     again - a round counted against MaxAgentRounds (sendsBack).
//   - A check with no agent work before it to do again, and any other failed
//     agent step, waits for an agent's `work retry`: agent work held (K4),
//     never the person's.
// Past the cap - the step's own failures, or for a check the request's
// rounds spent - the failure is the person's (K2: retry it or drop it), as
// every failure was before.

// The packet params of a failed agent step: how many times it failed, and
// that its failure went past the cap to the person.
const (
	paramFailedRounds = "failed_rounds"
	paramFailPastCap  = "fail_past_cap"
)

// failedRounds is how many times a step failed (its params, read back from
// JSON or not).
func failedRounds(t api.WorkTodo) int {
	switch n := t.Packet.Params[paramFailedRounds].(type) {
	case int:
		return n
	case float64:
		return int(n)
	case int64:
		return int(n)
	}
	return 0
}

// failedToAgent says a failed step is an agent's to move on: an agent's
// to-do whose failure is not past the cap.
func failedToAgent(t api.WorkTodo) bool {
	past, _ := t.Packet.Params[paramFailPastCap].(bool)
	return t.For != schedapi.ForPerson && !past
}

// reworksOnFail is an agent's check step whose failure plans its rework: a
// verify-phase step an agent does with no interaction (an auto-review, an
// auto-test) - not a verify, a review or any step with an interaction (a
// validate), not an increment's addition, acceptance or delivery review, nor
// a validate-adds step, which have their own paths.
func reworksOnFail(p api.WorkPacket) bool {
	return p.Phase == api.PhaseVerify && p.Doer != api.DoerPerson && p.Interaction == "" && reworksOnReject(p)
}

// collectAgentFailure is what a failed report on an agent's step does: under the cap, a check step's rework is planned (the failed step
// archived, what came after it waiting on the check asked again) and
// handled answers true; otherwise the step's failure count is kept on wt -
// past the cap, marked the person's - and the caller puts it back to wait.
func collectAgentFailure(ctx context.Context, cwm cwmapi.CWMService, wt *api.WorkTodo, r *schedapi.Result) (handled bool, err error) {
	if r.Status != schedapi.StatusFail || wt.For == schedapi.ForPerson {
		return false, nil
	}
	n := failedRounds(*wt) + 1
	// A person (or the planner) failing an agent's step says so to a person.
	past := n > MaxAgentRounds || !countsAgainstCap(r.By)
	if reworksOnFail(wt.Packet) && countsAgainstCap(r.By) {
		done, err := doneOf(ctx, wt.Request)
		if err != nil {
			return false, err
		}
		// The step's own failures count too: a check with nothing to redo
		// plans no rework, so the request's rounds never grow.
		sent, _ := agentRounds(done)
		past = past || sent >= MaxAgentRounds
		if !past && len(toRedo(done, *wt)) > 0 {
			return true, reworkFailed(ctx, cwm, *wt, r)
		}
	}
	wt.Packet.Params = maps.Clone(wt.Packet.Params)
	if wt.Packet.Params == nil {
		wt.Packet.Params = map[string]any{}
	}
	wt.Packet.Params[paramFailedRounds] = n
	if past {
		wt.Packet.Params[paramFailPastCap] = true
	}
	return false, nil
}

// reworkFailed plans a failed check step's rework: the agent work it came
// after done again with the failure as feedback, then the check asked again;
// what came after the failed step waits on the check asked again, the
// failed step is archived and what it found on the way is filed.
func reworkFailed(ctx context.Context, cwm cwmapi.CWMService, wt api.WorkTodo, r *schedapi.Result) error {
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
	feedback := strings.TrimSpace("its " + wt.Packet.Step + " failed: " + r.Summary + "\n\n" + r.Details)
	again, err := rework(ctx, req, wt, plan, last, feedback, r.By)
	if err != nil {
		return err
	}
	for _, x := range plan {
		if x.ID == wt.ID || !notStarted(x) || !slices.Contains(x.After, wt.ID) {
			continue
		}
		x.After = replaceID(x.After, wt.ID, again)
		if _, err := put(ctx, x); err != nil {
			return err
		}
	}
	if err := fileFoundOnTheWay(ctx, cwm, &wt, followUps(r), r.By); err != nil {
		return err
	}
	// The node moves when the check asked again ends (it carries the move):
	// the failed step, archived, never moves it.
	wt.Packet.Moves = ""
	if _, err := put(ctx, wt); err != nil {
		return err
	}
	if err := archive(ctx, wt.ID, "failed, its rework planned: "+firstLine(r.Summary)); err != nil {
		return err
	}
	return settleAll(ctx, r.By)
}
