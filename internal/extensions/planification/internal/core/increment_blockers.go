// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// WHAT BLOCKS AN INCREMENT - the one reader of
// planning check, on the increment's scope, followed through its waits down
// to the root even beyond the increment: each root that holds its work is
// listed once, under its own kind, with who acts, since when and the pieces of
// the increment's work it holds; the open incidents of the reconciler's list
// on that work join its incidents. No second reader: what holds a piece of
// work is read once, by blockers.go.

// Blockers answers everything that locks an increment, grouped by kind.
func (i *Increments) Blockers(ctx context.Context, in api.GetInput) (api.IncrementBlockersOutput, error) {
	ctx = withReadCache(ctx)
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.IncrementBlockersOutput{}, err
	}
	inc, err := loadIncrement(ctx, cwm, in.ID)
	if err != nil {
		return api.IncrementBlockersOutput{}, err
	}
	read, r, err := readScope(ctx, api.PlanningCheckInput{Increment: inc.ID}, true)
	if err != nil {
		return api.IncrementBlockersOutput{}, err
	}
	// Each root once: the blockers it holds name it as theirs.
	type rooted struct {
		b     api.IncrementBlocker
		holds map[string]bool
	}
	byRoot := map[string]*rooted{}
	var order []string
	works := map[string]bool{}
	for _, b := range read.Blockers {
		root := b
		if got, ok := r.roots[rootKey(b)]; ok {
			root = got
		}
		root.Class = api.BlockerClass(root.Kind)
		works[b.Work], works[root.Work] = true, true
		for _, c := range b.Chain {
			works[c] = true
		}
		k := rootKey(root)
		got, ok := byRoot[k]
		if !ok {
			got = &rooted{b: api.IncrementBlocker{Blocker: root}, holds: map[string]bool{}}
			byRoot[k] = got
			order = append(order, k)
		}
		if b.Work != root.Work {
			got.holds[b.Work] = true
		}
	}
	// The incidents' list: an open record on this work is its root's record,
	// or an incident of its own.
	records, err := Incidents(ctx)
	if err != nil {
		return api.IncrementBlockersOutput{}, err
	}
	for _, rec := range records {
		if !works[rec.Work] {
			continue
		}
		matched := false
		for _, k := range order {
			got := byRoot[k]
			if got.b.Kind == api.BlockerIncident && got.b.Work == rec.Work && (rec.Todo == "" || got.b.Todo == "" || rec.Todo == got.b.Todo) && got.b.Incident == "" {
				got.b.Incident, got.b.Sweeps = rec.Key, rec.Sweeps
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		b := api.Blocker{Class: api.BlockerClass(api.BlockerIncident), Kind: api.BlockerIncident, Work: rec.Work, Todo: rec.Todo,
			Cause: rec.Cause, Reason: rec.Reason, Actor: orDefault(rec.Actor, "the reconciler"), ActorKind: api.ActorSystem, Since: rec.Since}
		if strings.HasPrefix(b.Actor, "the person") {
			b.ActorKind = api.ActorPerson // only the person's act clears it
		}
		if o, err := loadObject(ctx, cwm, rec.Work); err == nil {
			b.Title = o.Name
		}
		k := "incident|" + rec.Key
		byRoot[k] = &rooted{b: api.IncrementBlocker{Blocker: b, Incident: rec.Key, Sweeps: rec.Sweeps}, holds: map[string]bool{}}
		order = append(order, k)
	}
	out := api.IncrementBlockersOutput{Increment: inc.ID, Counts: map[string]int{}, Groups: []api.IncrementBlockerGroup{}}
	groups := map[string][]api.IncrementBlocker{}
	for _, k := range order {
		got := byRoot[k]
		b := got.b
		for w := range got.holds {
			b.Holds = append(b.Holds, w)
		}
		sort.Strings(b.Holds)
		b.Age = ageOf(b.Since, r.now)
		if t, ok := r.byID[b.Todo]; ok {
			b.Unwait = waitRemovable(t)
		}
		if o, err := loadObject(ctx, cwm, b.Work); err == nil && !r.inScope(ctx, o) {
			b.Outside = r.whereIs(ctx, o)
		}
		groups[b.Kind] = append(groups[b.Kind], b)
	}
	for _, kind := range api.BlockerKinds {
		bs := groups[kind]
		if len(bs) == 0 {
			continue
		}
		sort.SliceStable(bs, func(i, j int) bool {
			if bs[i].Since != bs[j].Since {
				return bs[i].Since < bs[j].Since // the oldest first
			}
			return bs[i].Work < bs[j].Work
		})
		out.Groups = append(out.Groups, api.IncrementBlockerGroup{Class: api.BlockerClass(kind), Kind: kind, Title: api.BlockerGroupTitles[kind], Blockers: bs})
		out.Counts[kind] = len(bs)
		out.Total += len(bs)
	}
	// What the reconciler already repaired on its work, the newest first
	// : a repair on the increment's own work or on work holding it.
	repaired, err := Repaired(ctx)
	if err != nil {
		return api.IncrementBlockersOutput{}, err
	}
	for k := len(repaired) - 1; k >= 0; k-- {
		rd := repaired[k]
		o, err := loadObject(ctx, cwm, rd.Work)
		if err != nil || !works[rd.Work] && !r.inScope(ctx, o) {
			continue
		}
		out.Repaired = append(out.Repaired, api.IncrementRepair{RepairDone: rd, Title: o.Name})
	}
	return out, nil
}

// BlockerCount is the Blockers tab's badge: how many roots hold
// the increment - the verb's total - in red.
func (i *Increments) BlockerCount(ctx context.Context, in api.IncrementPaneInput) (api.IncrementCountOutput, error) {
	b, err := i.Blockers(ctx, api.GetInput{ID: in.ID})
	if err != nil {
		return api.IncrementCountOutput{}, err
	}
	return api.IncrementCountOutput{Count: b.Total, Color: "red"}, nil
}

// ageOf is how long ago since was, in days, hours or minutes; "" when it
// cannot be read.
func ageOf(since string, now time.Time) string {
	at, err := time.Parse(time.RFC3339Nano, since)
	if err != nil {
		return ""
	}
	d := now.Sub(at)
	switch {
	case d < 0:
		return "0m"
	case d >= 48*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d >= time.Hour:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
}
