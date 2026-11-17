// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// EndDroppedIncrement ends the work of an increment moved dropped
// : its own plan (its proposal's steps) and the plans
// of the goals and features it took that no open increment or release holds
// still end dropped, through the plans' finish - their to-dos not started are
// archived (EndTodos), none left for an agent to pull under a dropped
// increment's label; a step already running finishes as any dropped
// request's does. A want another open scope took keeps its work. Called on
// the increment's move and on its transitioned event: an end already made is
// skipped.
func EndDroppedIncrement(ctx context.Context, id, to string) error {
	if to != api.IncrementDropped {
		return nil
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return err
	}
	inc, err := loadObject(ctx, cwm, id)
	if err != nil || inc.Type != api.IncrementType || releaseState(inc) != api.IncrementDropped {
		return nil
	}
	var ends []string
	if hasRequest(inc) && !planEnded(planState(inc)) {
		ends = append(ends, id)
	}
	work, err := workOf(ctx, cwm, id)
	if err != nil {
		return err
	}
	for _, w := range work {
		o, err := loadObject(ctx, cwm, w)
		if err != nil || (o.Type != featureType && o.Type != goalType) || !hasRequest(o) || planEnded(planState(o)) {
			continue
		}
		if len(liveTakers(ctx, cwm, w)) > 0 {
			continue // another open increment or release holds it
		}
		ends = append(ends, w)
	}
	note := "its increment " + id + " was dropped: its work is not done"
	var errs []error
	for _, e := range ends {
		if _, err := NewPlans().Finish(ctx, api.FinishInput{ID: e, Outcome: api.PlanDropped, By: PlanningActor, Note: note}); err != nil {
			errs = append(errs, fmt.Errorf("end %s: %w", e, err))
		}
	}
	return errors.Join(errs...)
}
