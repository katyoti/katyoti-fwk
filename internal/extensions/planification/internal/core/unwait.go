// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"slices"
	"strings"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	profileapi "github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// Unwait removes a waiting to-do's wait, by a person:
// the act the Blockers tab's « Retirer l'attente » and the reconciler's
// decision option « Remove the wait » name. A blocked report's stop is
// cleared - whatever it holds on (an object, a kind nobody classified, none):
// its waits line, its report and its hold go together. A wait its request
// takes from a depends-on edge drops that edge, else the next settle would
// derive it again. A wait on its increment's dependency is cleared once that
// dependency no longer holds; while it does, it is refused - the increment's
// own dependency is the person's act there. Any other wait - a step no
// longer open, a line no reader classifies, an unreadable increment - is
// cleared: settle derives it again if it still holds. A review's hold is
// never removed: its review ends it. Only a waiting to-do has a wait to
// remove (waitRemovable, the Blockers tab's predicate too). The request is
// then settled: the step goes back to the queue.
func (p *Plans) Unwait(ctx context.Context, in api.UnwaitInput) (api.PlannedOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.PlannedOutput{}, err
	}
	if err := profileapi.PersonAct(in.By, "todo:unwait"); err != nil {
		return api.PlannedOutput{}, invalid("Unwait: a wait is removed by a person - %s", err.Error())
	}
	if strings.TrimSpace(in.Todo) == "" {
		return api.PlannedOutput{}, invalid("Unwait: name the waiting to-do (todo)")
	}
	if _, err := personsAct(ctx, in.By, "unwait:"+in.Todo, "Unwait: a person removes a wait themselves (the Blockers tab, or their own command)"); err != nil {
		return api.PlannedOutput{}, err
	}
	t, err := getTodo(ctx, in.Todo)
	if err != nil {
		return api.PlannedOutput{}, err
	}
	switch {
	case strings.HasPrefix(t.Waits, reviewHoldWaits):
		return api.PlannedOutput{}, invalid("Unwait: %s %s - its review ends that wait", in.Todo, t.Waits)
	case !waitRemovable(t):
		return api.PlannedOutput{}, invalid("Unwait: %s waits on nothing a person removes (%s)", in.Todo, orDefault(t.Waits, "no wait"))
	case strings.HasPrefix(t.Waits, waitsOnDependency):
		dep, _ := waitTarget(t)
		if err := dropDependency(ctx, t.Request, dep); err != nil {
			return api.PlannedOutput{}, err
		}
	case stopStatus(t) == schedapi.StatusBlocked:
		clearStop(&t)
		t.Held = nil
		if _, err := put(ctx, t); err != nil {
			return api.PlannedOutput{}, err
		}
	default:
		if inc, dep, ok := incrementWait(t.Waits); ok {
			if err := incrementStillWaits(ctx, inc, dep); err != nil {
				return api.PlannedOutput{}, err
			}
		}
		clearStop(&t)
		t.Held = nil
		if _, err := put(ctx, t); err != nil {
			return api.PlannedOutput{}, err
		}
	}
	if err := settleRequest(ctx, t.Request, in.By); err != nil {
		return api.PlannedOutput{}, err
	}
	return p.Get(ctx, api.GetPlanInput{ID: t.Request})
}

// reviewHoldWaits begins the wait settle writes for a step its review holds.
const reviewHoldWaits = "held by the review of "

// waitRemovable says whether a person may remove a to-do's wait: it waits,
// and not on its review. It is the one predicate of Unwait and of the
// Blockers tab's « Remove the wait » (api.IncrementBlocker.Unwait), so the
// button is offered only where the act accepts.
func waitRemovable(t api.WorkTodo) bool {
	return t.State == schedapi.StateWaiting && !strings.HasPrefix(t.Waits, reviewHoldWaits)
}

// incrementStillWaits refuses to remove a to-do's wait on its increment's
// dependency while that dependency holds: inc still depends on dep and dep
// has not ended - the next settle would put the wait back. Removing it is
// then the increment's act (its depends-on edge), not the step's.
func incrementStillWaits(ctx context.Context, inc, dep string) error {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return err
	}
	io, _, err := loadAny(ctx, cwm, inc)
	if err != nil {
		return nil // an unreadable increment holds nothing: settle reads it again
	}
	if isDoneIncrement(io) || !slices.Contains(edgesTo(io, api.EdgeDependsOn), dep) {
		return nil
	}
	if do, _, err := loadAny(ctx, cwm, dep); err == nil && waitEnded(do) {
		return nil
	}
	return invalid("Unwait: its increment %s depends on %s, still open - the wait is the increment's: remove %s's dependency on %s, or end %s", inc, dep, inc, dep, dep)
}

// dropDependency removes request's depends-on edge to dep.
func dropDependency(ctx context.Context, request, dep string) error {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return err
	}
	o, err := loadObject(ctx, cwm, request)
	if err != nil {
		return err
	}
	if !slices.Contains(dependsOn(o), dep) {
		return invalid("Unwait: %s does not depend on %s", request, dep)
	}
	o.Edges = slices.DeleteFunc(o.Edges, func(e cwmapi.ObjectEdge) bool { return e.Kind == api.EdgeDependsOn && e.To == dep })
	module, _ := cwmapi.ModuleOfObjectID(o.ID)
	_, err = cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: o})
	return err
}
