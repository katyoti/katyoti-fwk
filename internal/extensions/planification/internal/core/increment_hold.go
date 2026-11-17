// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"slices"
	"strings"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// AN INCREMENT ON HOLD — put aside saying why: its work is not pulled (each
// to-do waits saying so), it leaves the release it ships in so that release
// no longer waits for it, and its hold shows wherever it appears. Resumed,
// its work is pulled again and it waits for a release: its stream's next cut
// or a person's ship-in. The hold lives beside the person's queue holds, in
// sys-planification's planning space, keyed by the increment: the increment
// keeps its state.

// incrementHoldKey prefixes an increment's key among the holds.
const incrementHoldKey = "increment:"

// heldIncrement is an increment's hold, or nil.
func heldIncrement(ctx context.Context, inc string) *api.Hold {
	holds, err := holdsOf(ctx)
	if err != nil {
		return nil
	}
	if h, ok := holds[incrementHoldKey+inc]; ok {
		return &h
	}
	return nil
}

// Hold puts an open increment on hold, saying why.
func (i *Increments) Hold(ctx context.Context, in api.IncrementHoldInput) (api.IncrementOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.IncrementOutput{}, err
	}
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		return api.IncrementOutput{}, invalid("Hold: says why the increment waits (reason)")
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.IncrementOutput{}, err
	}
	o, err := loadIncrement(ctx, cwm, in.ID)
	if err != nil {
		return api.IncrementOutput{}, err
	}
	if st := releaseState(o); st != api.IncrementOpen {
		return api.IncrementOutput{}, invalid("Hold: %s is %s: only an open increment is put on hold", in.ID, st)
	}
	h := api.Hold{Key: incrementHoldKey + o.ID, Object: o.ID, Reason: reason, By: in.By, At: time.Now().UTC().Format(time.RFC3339)}
	if err := editHolds(ctx, keyOf(o.ID), func(hs []api.Hold) []api.Hold {
		hs = slices.DeleteFunc(hs, func(x api.Hold) bool { return x.Key == h.Key })
		return append(hs, h)
	}); err != nil {
		return api.IncrementOutput{}, err
	}
	// The release it shipped in no longer waits for it; a planned one keeps it
	// planned for it (planned-for), so its pane still shows it, on hold, with
	// Resume - shipped again in one action once resumed.
	if rel := edgeTo(o, api.EdgeShipsIn); rel != "" {
		o = withEdge(o, api.EdgeShipsIn, "")
		if r, _, err := loadAny(ctx, cwm, rel); err == nil && releaseState(r) == api.ReleasePlanned {
			o = withEdge(o, api.EdgePlannedFor, rel)
		}
		module, _ := cwmapi.ModuleOfObjectID(o.ID)
		if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: o}); err != nil {
			return api.IncrementOutput{}, err
		}
	}
	if err := settleAll(ctx, PlanningActor); err != nil {
		return api.IncrementOutput{}, err
	}
	if o, err = loadIncrement(ctx, cwm, in.ID); err != nil {
		return api.IncrementOutput{}, err
	}
	return api.IncrementOutput{Increment: toIncrement(ctx, o)}, nil
}

// Resume takes an increment off hold: its work is pulled again.
func (i *Increments) Resume(ctx context.Context, in api.IncrementHoldInput) (api.IncrementOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.IncrementOutput{}, err
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.IncrementOutput{}, err
	}
	o, err := loadIncrement(ctx, cwm, in.ID)
	if err != nil {
		return api.IncrementOutput{}, err
	}
	found, err := dropHolds(ctx, incrementHoldKey+o.ID)
	if err != nil {
		return api.IncrementOutput{}, err
	}
	if !found {
		return api.IncrementOutput{}, invalid("Resume: %s is not on hold", in.ID)
	}
	if err := settleAll(ctx, PlanningActor); err != nil {
		return api.IncrementOutput{}, err
	}
	if o, err = loadIncrement(ctx, cwm, in.ID); err != nil {
		return api.IncrementOutput{}, err
	}
	return api.IncrementOutput{Increment: toIncrement(ctx, o)}, nil
}
