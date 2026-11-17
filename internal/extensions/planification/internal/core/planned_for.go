// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// PLANNED FOR — an increment aimed at a later release of its stream
// : an intention beside ships-in, which stays its
// stream's next release only (decision). Planned for the
// next release, it ships in it; once a later release becomes the next, one
// action ships every increment planned for it; a dropped release returns them
// to the backlog, each saying so (toIncrement).

// PlanFor plans an increment for a planned release of its stream: the next one
// ships it in, a later one records the intention.
func (i *Increments) PlanFor(ctx context.Context, in api.PlanForInput) (api.IncrementOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.IncrementOutput{}, err
	}
	o, err := loadIncrement(ctx, cwm, in.ID)
	if err != nil {
		return api.IncrementOutput{}, err
	}
	if st := releaseState(o); st != api.IncrementOpen {
		return api.IncrementOutput{}, invalid("PlanFor: %s is %s: only an open increment is planned", in.ID, st)
	}
	r, _, err := loadAny(ctx, cwm, in.In)
	if err != nil {
		return api.IncrementOutput{}, err
	}
	if r.Type != api.ReleaseType || releaseState(r) != api.ReleasePlanned {
		return api.IncrementOutput{}, invalid("PlanFor: %s is no planned release: an increment is planned for a release still to come", in.In)
	}
	if why, err := beyondOpenTwo(ctx, cwm, r); err != nil {
		return api.IncrementOutput{}, err
	} else if why != "" {
		return api.IncrementOutput{}, invalid("PlanFor: %s", why)
	}
	sm := edgeTo(r, api.EdgeInStream)
	own := edgeTo(o, api.EdgeInStream)
	if own == "" && sm != "" {
		// An increment in no stream takes its release's: never hidden from
		// the views that list increments by stream.
		// Written with what it is planned for, once every check passed - a
		// refused plan-for leaves it as it was; the
		// next release's ship-in writes it itself.
		o, own = withEdge(o, api.EdgeInStream, sm), sm
	}
	if own == "" || own != sm {
		return api.IncrementOutput{}, invalid("PlanFor: %s is in stream %q, %s in %q: an increment is planned for a release of its own stream", in.In, sm, in.ID, own)
	}
	if next, ok := nextPlannedRelease(ctx, cwm, sm); ok && next == in.In {
		// The next release: assigned, not only intended.
		return shipIn(ctx, api.ShipInInput{ID: in.ID, In: in.In}, true)
	}
	if edgeTo(o, api.EdgeShipsIn) != "" {
		o = withEdge(o, api.EdgeShipsIn, "")
	}
	o = withEdge(o, api.EdgePlannedFor, in.In)
	module, _ := cwmapi.ModuleOfObjectID(o.ID)
	out, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: o})
	if err != nil {
		return api.IncrementOutput{}, err
	}
	if err := settleAll(ctx, PlanningActor); err != nil {
		return api.IncrementOutput{}, err
	}
	return api.IncrementOutput{Increment: toIncrement(ctx, out.Object)}, nil
}

// Unplan takes an increment's planned-for release off it.
func (i *Increments) Unplan(ctx context.Context, in api.GetInput) (api.IncrementOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.IncrementOutput{}, err
	}
	o, err := loadIncrement(ctx, cwm, in.ID)
	if err != nil {
		return api.IncrementOutput{}, err
	}
	if edgeTo(o, api.EdgePlannedFor) == "" {
		return api.IncrementOutput{Increment: toIncrement(ctx, o)}, nil
	}
	o = withEdge(o, api.EdgePlannedFor, "")
	module, _ := cwmapi.ModuleOfObjectID(o.ID)
	out, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: o})
	if err != nil {
		return api.IncrementOutput{}, err
	}
	return api.IncrementOutput{Increment: toIncrement(ctx, out.Object)}, nil
}

// ShipPlanned ships in a release every open increment planned for it, once it
// is its stream's next planned release; a held one stays planned.
func (r *Releases) ShipPlanned(ctx context.Context, in api.MoveInput) (api.ShipPlannedOutput, error) {
	out := api.ShipPlannedOutput{Release: in.ID, Shipped: []string{}, Stayed: []string{}}
	if err := checkActor(in.By); err != nil {
		return out, err
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return out, err
	}
	rel, err := loadRelease(ctx, cwm, in.ID)
	if err != nil {
		return out, err
	}
	sm := edgeTo(rel, api.EdgeInStream)
	if next, ok := nextPlannedRelease(ctx, cwm, sm); !ok || next != in.ID {
		return out, invalid("ShipPlanned: %s is not the next release of %s (%s): what is planned for it ships once it is", in.ID, sm, next)
	}
	planned, err := plannedFor(ctx, cwm, in.ID)
	if err != nil {
		return out, err
	}
	for _, o := range planned {
		if heldIncrement(ctx, o.ID) != nil {
			out.Stayed = append(out.Stayed, o.ID+": on hold - it ships once resumed")
			continue
		}
		if releaseState(o) != api.IncrementOpen {
			continue
		}
		if _, err := shipIn(ctx, api.ShipInInput{ID: o.ID, In: in.ID}, true); err != nil {
			return out, err
		}
		out.Shipped = append(out.Shipped, o.ID)
	}
	return out, nil
}

// plannedFor is the increments planned for a release.
func plannedFor(ctx context.Context, cwm cwmapi.CWMService, release string) ([]cwmapi.Object, error) {
	ids, err := incoming(ctx, cwm, release, api.EdgePlannedFor)
	if err != nil {
		return nil, err
	}
	var out []cwmapi.Object
	for _, id := range ids {
		if o, _, err := loadAny(ctx, cwm, id); err == nil && o.Type == api.IncrementType {
			out = append(out, o)
		}
	}
	return out, nil
}
