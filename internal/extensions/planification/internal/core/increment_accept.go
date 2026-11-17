// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"sort"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// WANTS PUT FORWARD — a goal or a feature an open increment holds, proposed or
// in review, waits on the person's acceptance: « À faire » shows one validate
// item per increment, listing them, each accepted or dropped there — or every
// one at once. A want recorded realized (or assumed) waits on the person's
// confirmation instead: its own item (confirm_items.go).

// The want states that wait on the person, and the transitions accepting one.
const (
	wantProposed   = "proposed"
	wantInReview   = "in-review"
	wantAccepted   = "accepted"
	wantReview     = "review"
	wantAccept     = "accept"
	acceptWantsURI = "ktw-agents:increments/accept"
	cwmTransition  = "ktw-agents:cwm/object-transition"
)

// putForward is a want state waiting on the person's acceptance.
func putForward(state string) bool { return state == wantProposed || state == wantInReview }

// Accept accepts, as a person, the wants an increment holds that are put
// forward: every one, or the one named — each reviewed, then accepted.
func (i *Increments) Accept(ctx context.Context, in api.AcceptWantsInput) (api.AcceptWantsOutput, error) {
	out := api.AcceptWantsOutput{Accepted: []string{}}
	if in.By.Kind != "person" {
		return out, invalid("only a person accepts a goal or a feature")
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return out, err
	}
	inc, err := loadIncrement(ctx, cwm, in.ID)
	if err != nil {
		return out, err
	}
	scope, err := scopeOf(ctx, cwm, inc)
	if err != nil {
		return out, err
	}
	found := false
	for _, t := range scope.Targets {
		if in.Want != "" && t.ID != in.Want {
			continue
		}
		found = true
		if !putForward(t.State) {
			if in.Want != "" {
				return out, invalid("%s is %s: only a goal or a feature put forward (proposed or in review) is accepted", t.ID, t.State)
			}
			continue
		}
		if err := acceptWant(ctx, cwm, t, in.By, inc.ID); err != nil {
			if out.Refused == nil {
				out.Refused = map[string]string{}
			}
			out.Refused[t.ID] = err.Error()
			continue
		}
		out.Accepted = append(out.Accepted, t.ID)
	}
	if in.Want != "" && !found {
		return out, invalid("%s is not held by %s", in.Want, in.ID)
	}
	return out, nil
}

// acceptWant moves one want put forward to accepted: reviewed first when it
// is only proposed.
func acceptWant(ctx context.Context, cwm cwmapi.CWMService, t api.Target, by cwmapi.Actor, increment string) error {
	note := "accepted from « À faire », held by " + increment
	if t.State == wantProposed {
		if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: t.ID, To: wantInReview, Transition: wantReview, By: by, Note: note}); err != nil {
			return fmt.Errorf("review: %w", err)
		}
	}
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: t.ID, To: wantAccepted, Transition: wantAccept, By: by, Note: note}); err != nil {
		return fmt.Errorf("accept: %w", err)
	}
	return nil
}

// wantItems is one validate item per open increment holding wants put
// forward, read from the index: the wants it lists, the oldest increment first.
func wantItems(ctx context.Context, cwm cwmapi.CWMService) []api.PersonItem {
	held := map[string][]api.Target{}
	for _, typ := range []string{"goal", "feature"} {
		list, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Type: typ})
		if err != nil {
			continue
		}
		for i, o := range list.Objects {
			st := ownState(o)
			if !putForward(st) {
				continue
			}
			// Several projects served: the want and its taker named in the
			// project holding them.
			key := ""
			if len(list.Projects) == len(list.Objects) {
				key = list.Projects[i]
			}
			if to := takerOf(ctx, cwm, o); to != "" {
				id := inKey(o, key).ID
				to = inKeyOf(id, to)
				held[to] = append(held[to], api.Target{ID: id, Type: o.Type, Title: o.DisplayTitle(), State: st})
			}
		}
	}
	var ids []string
	for id := range held {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var out []api.PersonItem
	for _, id := range ids {
		inc, err := loadIncrement(ctx, cwm, id)
		if err != nil || releaseState(inc) != api.IncrementOpen {
			continue // a release, or an increment no longer open
		}
		wants := held[id]
		sort.Slice(wants, func(a, b int) bool { return wants[a].ID < wants[b].ID })
		module, _ := cwmapi.ModuleOfObjectID(bareID(id))
		out = append(out, api.PersonItem{Key: api.ActionValidate + ":" + id, Action: api.ActionValidate, Object: id, Module: module,
			Project: keyOf(id), Title: titleOf(inc), Wants: wants,
			Asks: fmt.Sprintf("Accept or drop the %d goals and features it holds that are put forward.", len(wants))})
	}
	return out
}
