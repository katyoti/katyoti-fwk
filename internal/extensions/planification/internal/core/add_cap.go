// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A WORKER ADDS AT MOST MaxAgentAdds ITEMS TO ONE REQUEST ON ITS OWN
// : the steps an agent's reports add (a report's
// more.add, a blocked report's adds, an agent reviewer's rework) and the
// follow-up requests its reports file, counted from their provenance
// (: a to-do's added_by; a done to-do's result). A
// person's adds and planification's own steps never count. Items past the
// cap are not planned: one person's validate step lists them, and its
// approval plans them - added by the person, so uncounted; rejected, they
// are dropped.

// MaxAgentAdds is how many items agents add to one request on their own.
const MaxAgentAdds = 8

// StepValidateAdds is the person's step that holds what agents would add past
// the cap.
const StepValidateAdds = "validate-adds"

// The validate step's params holding what waits: the steps and the follow-ups.
const (
	paramWaitingAdds      = "adds"
	paramWaitingFollowUps = "follow_ups"
)

// overflow is what a report would add past the cap.
type overflow struct {
	steps []api.NewStep
	fus   []api.FollowUp
}

func (o overflow) any() bool { return len(o.steps)+len(o.fus) > 0 }

// countsAgainstCap is an actor whose adds are counted: an agent, not
// planification adding a step of its own machinery.
func countsAgainstCap(a cwmapi.Actor) bool {
	return a.Kind == "agent" && a.ID != PlanningActor.ID
}

// agentAdds counts the items agents added to a request on their own: its
// to-dos (open and done) an agent added, and the follow-ups its agents'
// successful reports filed.
func agentAdds(ctx context.Context, request string, plan []api.WorkTodo) (int, error) {
	done, err := doneOf(ctx, request)
	if err != nil {
		return 0, err
	}
	n, seen := 0, map[string]bool{}
	for i, x := range append(slices.Clone(done), plan...) {
		if seen[x.ID] {
			continue
		}
		seen[x.ID] = true
		if x.Packet.AddedBy != nil && countsAgainstCap(*x.Packet.AddedBy) {
			n++
		}
		// What its blocked or failed reports filed.
		n += filedFollowUps(x.Packet)
		// A done to-do's report filed its follow-ups (an open one's not yet).
		if r := x.Result; i < len(done) && r != nil && r.Status == schedapi.StatusSuccess && countsAgainstCap(r.By) {
			n += len(followUps(r))
		}
	}
	return n, nil
}

// capAdds keeps what fits under the cap given n items already added - the
// steps first, then the follow-ups - and answers the rest as the overflow.
func capAdds(n int, steps []api.NewStep, fus []api.FollowUp) ([]api.NewStep, []api.FollowUp, overflow) {
	room := max(0, MaxAgentAdds-n)
	var over overflow
	if len(steps) > room {
		steps, over.steps = steps[:room], steps[room:]
	}
	room -= len(steps)
	if len(fus) > room {
		fus, over.fus = fus[:room], fus[room:]
	}
	return steps, fus, over
}

// askOverflow plans the person's validate step listing what waits past the
// cap, at position pos after `after`; it carries `moves` when the waiting
// steps would have. Planification adds it (uncounted), one generation after wt.
func askOverflow(ctx context.Context, req cwmapi.Object, wt api.WorkTodo, over overflow, pos int, after []string, moves string, n int) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "Agents already added %d items to this request on their own (the cap is %d). Approve to add what follows, reject to drop it:\n", n, MaxAgentAdds)
	for _, s := range over.steps {
		fmt.Fprintf(&b, "\n- step %s: %s", s.Step, firstLine(s.Instructions))
	}
	for _, f := range over.fus {
		fmt.Fprintf(&b, "\n- follow-up request: %s", f.Title)
	}
	p := api.WorkPacket{Step: StepValidateAdds, Doer: api.DoerPerson, Interaction: api.InteractionValidate,
		Instructions: b.String(), Phase: wt.Packet.Phase, Moves: moves,
		Params: map[string]any{}}
	if len(over.steps) > 0 {
		p.Params[paramWaitingAdds] = over.steps
	}
	if len(over.fus) > 0 {
		p.Params[paramWaitingFollowUps] = over.fus
	}
	p.Added(wt.Packet, PlanningActor, fmt.Sprintf("past the cap of %d items agents add on their own", MaxAgentAdds))
	if err := checkPacket(p, wt.Priority, ""); err != nil {
		return "", invalid("Collect: step %s: %v", p.Step, err)
	}
	created, err := put(ctx, api.WorkTodo{Todo: schedapi.Todo{
		Module: wt.Module, Target: wt.Target, Request: todoRequestRef(ctx, wt.Todo), Title: p.Step + " — " + titleOf(req),
		For: forOf(p.Doer), Group: api.PlanGroup, Start: startOf(p.Doer), Priority: wt.Priority,
		State: schedapi.StateWaiting, Since: wt.Since, Position: pos, After: slices.Clone(after),
		Reads: wt.Reads, Writes: wt.Writes,
	}, Packet: p})
	if err != nil {
		return "", err
	}
	return created.ID, nil
}

// waitingAdds reads back what a validate step holds.
func waitingAdds(p api.WorkPacket) ([]api.NewStep, []api.FollowUp) {
	var steps []api.NewStep
	var fus []api.FollowUp
	decodeParam(p.Params[paramWaitingAdds], &steps)
	decodeParam(p.Params[paramWaitingFollowUps], &fus)
	return steps, fus
}

func decodeParam(v, into any) {
	if v == nil {
		return
	}
	if raw, err := json.Marshal(v); err == nil {
		_ = json.Unmarshal(raw, into)
	}
}

// isValidateAdds is the person's step that holds what waits past the cap.
func isValidateAdds(p api.WorkPacket) bool {
	return strings.TrimSuffix(p.Step, reworkSuffix) == StepValidateAdds
}
