// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// Surface lists what an object offers (actionsapi.SurfaceService).
//
// THE WHOLE SURFACE OR NOTHING. Four sources — the type's and the effective
// capabilities' actions, the task-side walk, the global registry — and the
// dedupe between them, with `when:` evaluated and `input:` interpolated. A
// list that returned only the declared half would look right and be quietly
// poorer. The capabilities are the EFFECTIVE ones (type ∪ aspects ∪
// instance), so a group added to one object surfaces its actions there alone.
//
// Every expression of a list goes to CWM in ONE evaluation (ObjectEvaluate):
// the object's vocabulary, the capabilities' functions, each asked once.
type Surface struct {
	tasks actionsapi.TaskActionSource
}

// NewSurface lists actions with the tasks tasks reports (nil = none).
func NewSurface(tasks actionsapi.TaskActionSource) *Surface { return &Surface{tasks: tasks} }

// candidate is one action before its expressions are evaluated.
type candidate struct {
	act   cwmapi.Action
	when  string
	input map[string]any
}

// List answers the action surface of a scope.
func (s *Surface) List(ctx context.Context, in actionsapi.ListInput) (actionsapi.ListOutput, error) {
	out := actionsapi.ListOutput{Actions: []cwmapi.Action{}}
	scope := in.Scope
	if scope == "" {
		scope = cwmapi.ActionScopeGlobal
		if in.ID != "" {
			scope = cwmapi.ActionScopeObject
		}
	}
	switch scope {
	case cwmapi.ActionScopeGlobal, cwmapi.ActionScopeObject, cwmapi.ActionScopeCollection:
	default:
		return out, fmt.Errorf("%w: unknown action scope %q", cwmapi.ErrInvalidInput, scope)
	}
	if scope != cwmapi.ActionScopeGlobal && in.ID == "" {
		return out, fmt.Errorf("%w: id required for scope %q", cwmapi.ErrInvalidInput, scope)
	}
	if scope == cwmapi.ActionScopeCollection && in.Type == "" {
		return out, fmt.Errorf("%w: type required for scope %q", cwmapi.ErrInvalidInput, scope)
	}

	var cwm cwmapi.CWMService
	var objType string
	if scope != cwmapi.ActionScopeGlobal {
		var err error
		if cwm, err = application.UseService[cwmapi.CWMService](ctx); err != nil || cwm == nil {
			return out, fmt.Errorf("action surface: no CWMService: %v", err)
		}
		module, _ := cwmapi.ModuleOfObjectID(in.ID)
		got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: in.ID})
		if err != nil {
			return out, err
		}
		objType = got.Object.Type
	}

	var cands []candidate
	seen := map[string]bool{}
	declared := func(ref cwmapi.ActionRef, source string) {
		key := ref.URI + "\x00" + ref.Label
		if ref.URI == "" || seen[key] {
			return
		}
		seen[key] = true
		seen["\x02"+ref.URI] = true
		label := ref.Label
		if label == "" {
			label = ref.URI
		}
		kind := ref.Kind
		if kind == "" {
			kind = "sync"
		}
		cands = append(cands, candidate{
			act: cwmapi.Action{URI: ref.URI, Label: label, Description: ref.Description, Icon: ref.Icon,
				Kind: kind, Source: source, Confirm: ref.Confirm, Preview: ref.Preview, Quick: ref.Quick, BoundInput: ref.Input},
			when: ref.EffectiveWhen(), input: ref.Input,
		})
	}

	switch scope {
	case cwmapi.ActionScopeObject:
		surf, err := cwm.ObjectEffectiveSurface(ctx, cwmapi.ObjectEffectiveSurfaceInput{ID: in.ID})
		if err != nil {
			return out, err
		}
		for _, sa := range surf.Actions {
			if sa.Ref.ObjectScope() == cwmapi.ActionScopeObject { // a collection action is the list's
				declared(sa.Ref, sa.Source)
			}
		}
		cands = s.taskActions(ctx, cands, seen, scope, objType)
	case cwmapi.ActionScopeCollection:
		if t, err := cwm.ObjectTypeGet(ctx, cwmapi.ObjectTypeGetInput{Name: in.Type}); err == nil {
			for _, ref := range t.ObjectType.Actions {
				if ref.ObjectScope() == cwmapi.ActionScopeCollection {
					declared(ref, "type")
				}
			}
		}
		cands = s.taskActions(ctx, cands, seen, scope, in.Type)
	default:
		cands = s.taskActions(ctx, cands, seen, scope, "")
	}

	if scope != cwmapi.ActionScopeGlobal {
		evaluate(ctx, cwm, in.ID, cands)
	}
	for _, c := range cands {
		out.Actions = append(out.Actions, c.act)
	}
	return out, nil
}

// taskActions is the task-side walk and the global registry — the half a
// type never declares.
func (s *Surface) taskActions(ctx context.Context, cands []candidate, seen map[string]bool, scope, typ string) []candidate {
	if s.tasks == nil {
		return cands
	}
	for _, t := range s.tasks(ctx) {
		if t.Name == "" || !taskInScope(t.Kind, scope) {
			continue
		}
		if scope != cwmapi.ActionScopeGlobal && !matchTypes(t.Types, typ) {
			continue
		}
		uri := t.Provider + ":tasks/" + t.Name
		if seen["\x02"+uri] {
			continue
		}
		seen["\x02"+uri] = true
		label := t.Label
		if label == "" {
			label = t.Name
		}
		kind := "async"
		if t.SessionMode == cwmapi.SessionModeChat {
			kind = "ai"
		}
		source := "applies-to"
		if scope == cwmapi.ActionScopeGlobal {
			source = "global"
		}
		cands = append(cands, candidate{
			act:  cwmapi.Action{URI: uri, Label: label, Description: t.Description, Icon: t.Icon, Kind: kind, Source: source},
			when: t.Filter,
		})
	}
	return cands
}

// taskInScope maps a task's declared scope onto the list's.
func taskInScope(kind, scope string) bool {
	switch scope {
	case cwmapi.ActionScopeGlobal, cwmapi.ActionScopeCollection:
		return kind == scope
	default:
		return kind == cwmapi.ActionScopeObject
	}
}

func matchTypes(types []string, name string) bool {
	for _, t := range types {
		if t == "*" || t == name {
			return true
		}
	}
	return false
}

// evaluate resolves every candidate's `when:` and `${...}` in one evaluation.
// A predicate that does not evaluate, or not to a bool, DISABLES its action
// with the reason: an action whose condition could not be read is not one to
// offer. An interpolation that does not resolve becomes EMPTY — never the
// template, which a caller would send as a value.
func evaluate(ctx context.Context, cwm cwmapi.CWMService, id string, cands []candidate) {
	index := map[string]int{}
	var exprs []string
	add := func(e string) {
		e = strings.TrimSpace(e)
		if _, ok := index[e]; !ok && e != "" {
			index[e] = len(exprs)
			exprs = append(exprs, e)
		}
	}
	for _, c := range cands {
		add(c.when)
		for _, v := range c.input {
			for _, e := range templateExprs(v) {
				add(e)
			}
		}
	}
	if len(exprs) == 0 {
		return
	}
	res, err := cwm.ObjectEvaluate(ctx, cwmapi.ObjectEvaluateInput{ID: id, Expressions: exprs, NoCostly: true})
	result := func(e string) cwmapi.EvaluateResult {
		e = strings.TrimSpace(e)
		if err != nil {
			return cwmapi.EvaluateResult{Expression: e, Error: err.Error()}
		}
		if i, ok := index[e]; ok && i < len(res.Results) {
			return res.Results[i]
		}
		return cwmapi.EvaluateResult{Expression: e, Error: "not evaluated"}
	}
	for i := range cands {
		c := &cands[i]
		if c.when != "" {
			r := result(c.when)
			switch b, isBool := r.Value.(bool); {
			case r.Error != "":
				c.act.Disabled, c.act.DisabledReason = true, "when-predicate failed: "+r.Error
			case !isBool:
				c.act.Disabled, c.act.DisabledReason = true, "when-predicate did not return a bool"
			case !b:
				c.act.Disabled, c.act.DisabledReason = true, "condition not met: "+c.when
			}
		}
		if len(c.input) > 0 {
			bound := make(map[string]any, len(c.input))
			for k, v := range c.input {
				bound[k] = interpolate(v, result)
			}
			c.act.BoundInput = bound
		}
	}
}

// templateExprs are the `${...}` expressions of a string value.
func templateExprs(v any) []string {
	s, ok := v.(string)
	if !ok {
		return nil
	}
	var out []string
	for {
		i := strings.Index(s, "${")
		if i < 0 {
			return out
		}
		j := strings.Index(s[i+2:], "}")
		if j < 0 {
			return out
		}
		out = append(out, s[i+2:i+2+j])
		s = s[i+2+j+1:]
	}
}

// interpolate resolves a value's `${...}`: a whole-string expression keeps its
// typed result; an embedded one is written as text. Unresolved (an error or
// nothing) is empty.
func interpolate(v any, result func(string) cwmapi.EvaluateResult) any {
	s, ok := v.(string)
	if !ok || !strings.Contains(s, "${") {
		return v
	}
	if strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}") && strings.Count(s, "${") == 1 {
		r := result(s[2 : len(s)-1])
		if r.Error != "" || r.Value == nil {
			return ""
		}
		return r.Value
	}
	var b strings.Builder
	rest := s
	for {
		i := strings.Index(rest, "${")
		if i < 0 {
			b.WriteString(rest)
			break
		}
		b.WriteString(rest[:i])
		rest = rest[i+2:]
		j := strings.Index(rest, "}")
		if j < 0 {
			b.WriteString("${" + rest)
			break
		}
		if r := result(rest[:j]); r.Error == "" && r.Value != nil {
			b.WriteString(fmt.Sprint(r.Value))
		}
		rest = rest[j+1:]
	}
	return b.String()
}
