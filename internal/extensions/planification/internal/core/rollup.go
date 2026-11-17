// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// THE ROLLUP — where a request stands is read from its to-dos and its plan,
// never stored: ended (its plan done or dropped; or every to-do archived without
// success); blocked (a to-do failed or is blocked, or its end refused); waiting
// on a person (their to-do is ready or theirs); in progress (a to-do is pulled
// or one has ended); planned; new.

// Rollup answers where a request stands, from its to-dos.
func (w *Work) Rollup(ctx context.Context, in api.RollupInput) (api.RollupOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.RollupOutput{}, err
	}
	o, err := load(ctx, cwm, in.Request)
	if err != nil {
		return api.RollupOutput{}, err
	}
	open, err := todosOf(ctx, o.ID)
	if err != nil {
		return api.RollupOutput{}, err
	}
	done, err := doneOf(ctx, o.ID)
	if err != nil {
		return api.RollupOutput{}, err
	}
	out := rollupOf(stateOf(o), open, done)
	out.Request = o.ID
	return out, nil
}

// rollupOf rolls a request's open and archived to-dos up, beside its plan's state.
func rollupOf(requestState string, open, done []api.WorkTodo) api.RollupOutput {
	out := api.RollupOutput{Total: len(open) + len(done), Done: len(done)}
	succeeded := false
	for _, t := range done {
		succeeded = succeeded || (t.Result != nil && t.Result.Status == schedapi.StatusSuccess)
	}
	started, person, blocked := succeeded, "", ""
	for _, t := range open {
		switch {
		case stopped(t):
			if blocked == "" {
				blocked = t.ID + " " + t.Waits
			}
		case t.State == schedapi.StateClaimed || t.State == schedapi.StateReported:
			started = true
		}
		if t.For == schedapi.ForPerson && t.State == schedapi.StateReady && person == "" {
			person = t.ID + " (" + t.Packet.Interaction + ")"
		}
	}
	switch {
	case requestState == api.PlanDone:
		out.State = api.RollupDone
	case requestState == api.PlanDropped:
		out.State = api.RollupDropped
	case out.Total == 0:
		out.State = api.RollupNew
	case len(open) == 0 && !succeeded:
		out.State = api.RollupDropped
	case blocked != "":
		out.State, out.Note = api.RollupBlocked, blocked
	case requestState == api.PlanBlocked:
		out.State, out.Note = api.RollupBlocked, "its end was refused"
	case person != "":
		out.State, out.Note = api.RollupWaiting, "a person's "+person
	case started:
		out.State = api.RollupInProgress
	default:
		out.State = api.RollupPlanned
	}
	return out
}
