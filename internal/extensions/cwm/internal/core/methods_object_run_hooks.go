// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"fmt"
	"sort"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// RUNNING A POINT. One runner, whatever the point: for each object, every
// applying hook at the point runs through the ActionService with its input
// (declared, resolved over the object — else the hook call), and its answer is
// kept RAW. ObjectRunHooks returns the runs; whoever owns a point reads them —
// the verification service reads lint as findings and verify as pass / fail /
// unproven, an extension its own point.

// hookType is a declared point: CWM's own (the floor) or a hook-type
// contribution.
func (s *Service) hookType(p api.HookPoint) (api.HookType, bool) {
	for _, t := range s.decl().HookTypes {
		if t.Name == p {
			return t, true
		}
	}
	for _, t := range api.BuiltinHookTypes {
		if t.Name == p {
			return t, true
		}
	}
	// A point only an event type names: after the event, no answer.
	for _, t := range api.EventHookTypes(s.decl().EventTypes) {
		if t.Name == p {
			return t, true
		}
	}
	return api.HookType{}, false
}

// runHooks runs point over objs. objects counts the objects that had a hook.
func (s *Service) runHooks(ctx stdctx.Context, objs []api.Object, point api.HookPoint) (objects int, runs []api.HookRun) {
	actions, _ := application.UseService[actionsapi.ActionService](ctx)
	for _, obj := range objs {
		hooks := s.applyingHooks(obj, point, nil)
		if len(hooks) == 0 {
			continue
		}
		objects++
		for _, h := range hooks {
			run := api.HookRun{ObjectID: obj.ID, URI: h.URI, Source: h.Source}
			input, err := s.hookInput(h, hookCall(point, obj, nil, nil))
			switch {
			case err != nil:
				run.Error = "input: " + err.Error()
			case actions == nil:
				run.Error = "no action service to run it"
			default:
				res, err := actions.Run(ctx, actionsapi.RunInput{URI: h.URI, Input: input})
				if err != nil {
					run.Error = err.Error()
				} else {
					run.Answer = res.Value
				}
			}
			runs = append(runs, run)
		}
	}
	return objects, runs
}

// ObjectRunHooks runs one on-demand point over an object, its subtree or a
// module's objects and returns the raw answers.
func (s *Service) ObjectRunHooks(ctx stdctx.Context, in api.ObjectRunHooksInput) (api.ObjectRunHooksOutput, error) {
	t, ok := s.hookType(in.Point)
	if !ok {
		return api.ObjectRunHooksOutput{}, fmt.Errorf("%w: hook point %q is not declared (no hook-type or event type names it)", api.ErrInvalidInput, in.Point)
	}
	if t.Timing == api.HookTimingBefore || t.Timing == api.HookTimingAfter {
		// A write point runs ON the write — running it by hand would ask a
		// before-hook about a write that is not happening.
		return api.ObjectRunHooksOutput{}, fmt.Errorf("%w: %q runs %s the writes, not on demand", api.ErrInvalidInput, in.Point, t.Timing)
	}
	objs, err := s.hookScope(ctx, in.ID, in.Module, in.Subtree)
	if err != nil {
		return api.ObjectRunHooksOutput{}, err
	}
	out := api.ObjectRunHooksOutput{Point: in.Point, Timing: t.Timing, Answer: t.Answer, Runs: []api.HookRun{}}
	n, runs := s.runHooks(ctx, objs, in.Point)
	out.Objects = n
	out.Runs = append(out.Runs, runs...)
	return out, nil
}

// hookScope is the objects a run covers: one object (with its subtree), or a
// module's objects. Exactly one of id and module.
func (s *Service) hookScope(ctx stdctx.Context, id, module string, subtree bool) ([]api.Object, error) {
	if (id == "") == (module == "") {
		return nil, fmt.Errorf("%w: exactly one of id and module", api.ErrInvalidInput)
	}
	ix := s.index()
	if err := ix.ensureLoaded(); err != nil {
		return nil, err
	}
	if module != "" {
		list, err := s.ObjectList(ctx, api.ObjectListInput{Module: module})
		if err != nil {
			return nil, err
		}
		return list.Objects, nil
	}
	obj, ok := ix.get(id)
	if !ok {
		return nil, api.ObjectNotFound(id)
	}
	if subtree {
		return s.subtree(obj), nil
	}
	return []api.Object{obj}, nil
}

// subtree is the object and every descendant, depth first, children in id
// order; a parent cycle cannot hang it.
func (s *Service) subtree(root api.Object) []api.Object {
	ix := s.index()
	var out []api.Object
	seen := map[string]bool{}
	var walk func(api.Object)
	walk = func(o api.Object) {
		if seen[o.ID] {
			return
		}
		seen[o.ID] = true
		out = append(out, o)
		kids := ix.childrenOf(o.ID)
		sort.Slice(kids, func(i, j int) bool { return kids[i].ID < kids[j].ID })
		for _, k := range kids {
			walk(k)
		}
	}
	walk(root)
	return out
}
