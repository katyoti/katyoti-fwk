// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/el"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// hydrated returns the object WITH its body and its type's fields. The index
// holds metadata and capabilities only (7b), and a predicate over
// `object.fields` needs them — so evaluating one is a read, taken once and only
// when something asks.
//
// Only the body and the fields are taken from the file: every other field is
// the given object's. A before-hook is handed the object being written (the
// transition's target state, the update's new fields); returning the stored
// copy would make it judge the object as it was.
func (s *Service) hydrated(obj api.Object) api.Object {
	if obj.Body != "" || obj.Fields != nil {
		return obj
	}
	p, ok := s.index().pathOf(obj.ID)
	if !ok {
		return obj
	}
	full, err := ReadObjectFile(p)
	if err != nil {
		return obj
	}
	obj.Body, obj.Fields = full.Body, full.Fields
	return obj
}

// objectActionEnv is the el environment (object vocabulary) a predicate and an interpolation see:
// the core and the capability groups under `object`, the type's fields under
// `object.fields`, identity at the top level, and the object's children as
// `children`.
func (s *Service) objectActionEnv(obj api.Object) map[string]any {
	obj = s.hydrated(obj)
	caps := objectConditionEnv(obj)
	return map[string]any{
		"object":    caps,
		"id":        obj.ID,
		"type":      obj.Type,
		"parent_id": obj.ParentID(),
		"children":  s.childrenConditionEnv(obj.ID),
		"links":     s.linksConditionEnv(obj.ID),
		"current":   s.currentOf(obj),
	}
}

// linksConditionEnv is `links`: the object's incident edges — the kind, the
// direction from this object, the other end — so a panel or an action can say
// "only when linked".
func (s *Service) linksConditionEnv(id string) []map[string]any {
	edges, err := s.EdgeList(stdctx.Background(), api.EdgeListInput{ObjectID: id})
	if err != nil {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(edges.Edges))
	for _, e := range edges.Edges {
		row := map[string]any{"kind": string(e.Kind), "direction": "out", "target": e.To}
		if e.To == id {
			row["direction"], row["target"] = "in", e.From
		}
		out = append(out, row)
	}
	return out
}

// childrenConditionEnv is the object-side `items`: what hangs under this
// object, with the fields a predicate asks about.
func (s *Service) childrenConditionEnv(id string) []map[string]any {
	ix := s.index()
	if ix.ensureLoaded() != nil {
		return []map[string]any{}
	}
	kids := ix.childrenOf(id)
	out := make([]map[string]any, 0, len(kids))
	for _, k := range kids {
		row := map[string]any{"id": k.ID, "type": k.Type, "title": "", "status": "", "tags": []string{}}
		if life, ok := k.Lifecycle(); ok {
			row["status"] = life.State // a state is the lifecycle's
		}
		row["title"] = k.Name
		if k.Tags != nil {
			row["tags"] = k.Tags
		}
		out = append(out, row)
	}
	return out
}

// evalWhenIn evaluates a predicate in a prepared environment. A predicate that
// does not compile, does not evaluate or does not return a bool DISABLES the
// action with the reason — an action whose condition could not be read is not
// an action anyone should be offered.
func (s *Service) evalWhenIn(expr string, env map[string]any) (disabled bool, reason string) {
	return evalPredicate(s.objectCache(), expr, env)
}

// interpolateObjectInput resolves `${...}` in a declaration's pre-bound input
// against the object environment. An expression that cannot be resolved
// becomes EMPTY rather than the literal `${...}`: a caller must not receive a
// template it would send as a value.
func interpolateObjectInput(exprs *el.Cache, input map[string]any, env map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for k, v := range input {
		val := interpolateValue(exprs, v, env)
		if s, ok := val.(string); ok && strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}") {
			val = ""
		}
		out[k] = val
	}
	return out
}

// currentOf is the MINIMAL projection of an object every expression and
// display sees as `current` (decided 2026-09-25): id, name, type, type_name,
// icon, icon_name, parent, module, file — the icon from the type, since an
// object carries none of its own — and capabilities, the ids of its effective
// capabilities (type, aspects, instance, attached), so a filter asks what an
// object carries (`'system-module' in current.capabilities`), never its type's
// name. Its capability groups are added under their ids
// (`current.lifecycle.state`), a convenience rather than a requirement; the
// minimal fields are written LAST, so a capability can never shadow them.
func (s *Service) currentOf(obj api.Object) map[string]any {
	// The capability groups, the object core and the legacy views — the same
	// vocabulary a condition sees.
	cur := objectConditionEnv(obj)
	typeName, icon, iconName := obj.Type, "", ""
	if t, ok := s.lookupObjectType(obj.Type); ok {
		if t.Label != "" {
			typeName = t.Label
		}
		icon, iconName = t.Icon, t.IconName
	}
	if obj.Icon != "" {
		icon = obj.Icon // the object's own icon wins over its type's
	}
	module, file := "", ""
	ix := s.index()
	if ix.ensureLoaded() == nil {
		module, _ = ix.moduleOf(obj.ID)
		if p, ok := ix.pathOf(obj.ID); ok {
			file = ix.rel(p)
		}
	}
	for k, v := range map[string]any{
		"id": obj.ID, "name": obj.DisplayTitle(), "type": obj.Type, "type_name": typeName,
		"icon": icon, "icon_name": iconName, "parent": obj.ParentID(), "module": module, "file": file,
		"capabilities": stringsAny(s.effectiveCapabilities(obj)),
	} {
		cur[k] = v
	}
	return cur
}

// interpolateValue resolves the `${...}` templates of v over env (el.Interpolate).
func interpolateValue(exprs *el.Cache, v any, env map[string]any) any {
	return el.Interpolate(v, func(expr string) (any, bool) {
		out, err := exprs.Eval(expr, env)
		// A nil result is UNRESOLVED, like an error: el reads a missing key as nil
		// where CEL raised, and an unset optional must not dispatch as a value.
		return out, err == nil && out != nil
	})
}
