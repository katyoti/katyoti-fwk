// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// RAISE — what files by itself (a trigger, a rule, a gate) files through one
// door: once per source and object. Firing again appends to the open request
// (when, what changed), and is left alone while that request's own work runs —
// its work is what changes the object. A request raised this way is planned at
// once, like any request — its intake first, which evaluates it. Review first:
// a source that holds files one priority above the open to-dos about the
// object and the objects linked to it, holds those not started until it ends, and names the running ones on the review (their
// results may be stale).

// Triggers implements api.TriggerService.
type Triggers struct{}

var _ api.TriggerService = (*Triggers)(nil)

// NewTriggers returns the trigger service.
func NewTriggers() *Triggers { return &Triggers{} }

// Raise files a request from a source, or appends to the one it filed.
func (t *Triggers) Raise(ctx context.Context, in api.RaiseInput) (api.RaiseOutput, error) {
	src := in.Source
	if src.Name == "" || src.Object == "" || !slices.Contains([]string{api.SourceTrigger, api.SourceRule, api.SourceGate}, src.Kind) {
		return api.RaiseOutput{}, invalid("Raise: a source has a kind (trigger, rule or gate), a name and an object, got %+v", src)
	}
	if in.Title == "" || in.Kind == "" {
		return api.RaiseOutput{}, invalid("Raise: say what is asked (title) and its kind")
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.RaiseOutput{}, err
	}
	obj, err := loadObject(ctx, cwm, src.Object)
	if err != nil {
		return api.RaiseOutput{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)

	if open, ok, err := raisedBy(ctx, cwm, src); err != nil {
		return api.RaiseOutput{}, err
	} else if ok {
		plan, err := todosOf(ctx, open.ID)
		if err != nil {
			return api.RaiseOutput{}, err
		}
		if slices.ContainsFunc(plan, func(td api.WorkTodo) bool { return td.State == schedapi.StateClaimed }) {
			return api.RaiseOutput{Request: open.ID, Note: "its own work is running: not fired again"}, nil
		}
		f := fieldsOf(open)
		s := *f.Source
		s.Times++
		s.FiredAt, s.Change = now, src.Change
		f.Source = &s
		f.Details = strings.TrimSpace(f.Details + "\n\nFired again " + now + ": " + src.Change)
		if err := open.SetCapability(api.CapRequest, f); err != nil {
			return api.RaiseOutput{}, err
		}
		if _, err := update(ctx, cwm, open); err != nil {
			return api.RaiseOutput{}, err
		}
		return api.RaiseOutput{Request: open.ID, Note: fmt.Sprintf("fired again (%d times): appended to %s", s.Times, open.ID)}, nil
	}

	// The work it affects: the open to-dos about the object and what is linked to it.
	var affected []api.WorkTodo
	priority := api.DefaultPriority
	if in.Priority != "" {
		priority = in.Priority
	}
	if in.Holds {
		if affected, err = affectedTodos(ctx, cwm, obj); err != nil {
			return api.RaiseOutput{}, err
		}
		top := len(api.Priorities)
		for _, td := range affected {
			top = min(top, priorityRank(td.Priority))
		}
		if top < len(api.Priorities) {
			priority = api.Priorities[max(0, top-1)]
		}
	}

	module, _ := cwmapi.ModuleOfObjectID(obj.ID)
	src.Times, src.FiredAt = 1, now
	file := api.FileInput{
		Title: in.Title, Kind: in.Kind, Priority: priority, Module: module, Source: &src,
		Details: strings.TrimSpace(in.Details + "\n\nFiled by the " + src.Kind + " " + src.Name + " on " + obj.ID + ": " + src.Change),
		By:      requesterOf(obj, module),
	}
	if in.On && !hasOpenRequest(obj) {
		file.On, file.Module, file.Title = obj.ID, "", ""
	} else {
		file.About = obj.ID
	}
	filed, err := New().File(ctx, file)
	if err != nil {
		return api.RaiseOutput{}, fmt.Errorf("Raise: file from %s %s on %s: %w", src.Kind, src.Name, obj.ID, err)
	}
	out := api.RaiseOutput{Request: filed.Request.ID, Filed: true}
	var stale []string
	for _, td := range affected {
		switch {
		case notStarted(td):
			td.Packet.HeldBy = append(td.Packet.HeldBy, out.Request)
			if _, err := put(ctx, td); err != nil {
				return api.RaiseOutput{}, err
			}
			out.Held++
		case td.State == schedapi.StateClaimed:
			stale = append(stale, td.ID+" ("+td.Packet.Step+" of "+td.Request+")")
			out.Stale++
		}
	}
	if len(stale) > 0 {
		// A to-do a worker holds is not rewritten: the review says which results to check.
		rq, err := loadObject(ctx, cwm, out.Request)
		if err != nil {
			return api.RaiseOutput{}, err
		}
		f := fieldsOf(rq)
		f.Details = strings.TrimSpace(f.Details + "\n\nRunning when it was filed — their results may be stale, check them against this review: " + strings.Join(stale, ", ") + ".")
		if err := rq.SetCapability(api.CapRequest, f); err != nil {
			return api.RaiseOutput{}, err
		}
		if _, err := update(ctx, cwm, rq); err != nil {
			return api.RaiseOutput{}, err
		}
	}
	if err := settleAll(ctx, PlanningActor); err != nil {
		return api.RaiseOutput{}, err
	}
	return out, nil
}

// raisedBy is the open request a source filed on its object, if any.
func raisedBy(ctx context.Context, cwm cwmapi.CWMService, src api.RequestSource) (cwmapi.Object, bool, error) {
	got, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Capability: api.CapRequest})
	if err != nil {
		return cwmapi.Object{}, false, err
	}
	for _, o := range got.Objects {
		s := fieldsOf(o).Source
		if s != nil && s.Kind == src.Kind && s.Name == src.Name && s.Object == src.Object && !nodeEnded(o) {
			return o, true, nil
		}
	}
	return cwmapi.Object{}, false, nil
}

// affectedTodos is the scheduled to-dos about an object or an object linked to
// it (their target, or their request).
func affectedTodos(ctx context.Context, cwm cwmapi.CWMService, obj cwmapi.Object) ([]api.WorkTodo, error) {
	near := map[string]bool{obj.ID: true}
	edges, err := cwm.EdgeList(ctx, cwmapi.EdgeListInput{ObjectID: obj.ID})
	if err != nil {
		return nil, err
	}
	for _, e := range edges.Edges {
		near[e.From], near[e.To] = true, true
	}
	all, err := todosOf(ctx, "")
	if err != nil {
		return nil, err
	}
	var out []api.WorkTodo
	for _, td := range all {
		if near[td.Target] || near[td.Request] {
			out = append(out, td)
		}
	}
	return out, nil
}

// requesterOf is who a source files in the name of: the goal or the feature it
// fired on, else the object's module.
func requesterOf(o cwmapi.Object, module string) cwmapi.Actor {
	if o.Type == "goal" || o.Type == "feature" {
		return cwmapi.Actor{ID: o.ID, Kind: o.Type}
	}
	return cwmapi.Actor{ID: module, Kind: "module"}
}

// hasOpenRequest reports an object carrying a request not ended.
func hasOpenRequest(o cwmapi.Object) bool {
	return hasRequest(o) && !nodeEnded(o)
}
