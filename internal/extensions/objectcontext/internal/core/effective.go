// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"sort"
	"strings"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectcontext/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectcontext/internal/resolver"
)

// Effective answers which objects of a kind are in force at an object — a
// query over CWM's reads, not a storage rule.
//
// THREE SOURCES, reduced by ONE of the four strategies:
//
//	Local — the focused object's own children of the kind;
//	Tree  — what the kind's `propagation:` reaches: up the ancestors, down
//	        the descendants, or nothing;
//	Type  — the focused object's type's `injected_objects`.
//
// THE KIND DECLARES THE RULES, NOT THE ASKER: propagation and resolution come
// from the type being collected — how far a rule reaches belongs to rules.
// Every setting is read from a type's `context:` section (api.ContextOf).
func (s *Service) Effective(ctx context.Context, in api.EffectiveInput) (api.EffectiveOutput, error) {
	if in.ID == "" || in.Type == "" {
		return api.EffectiveOutput{}, fmt.Errorf("%w: id and type required", cwmapi.ErrInvalidInput)
	}
	r, err := readerOf(ctx)
	if err != nil {
		return api.EffectiveOutput{}, err
	}
	focused, err := r.get(ctx, in.ID)
	if err != nil {
		return api.EffectiveOutput{}, err
	}
	kindType, declared := r.objectType(ctx, in.Type)
	if !declared {
		// An undeclared kind has no propagation and no strategy: refusing names
		// the gap, a default would invent one.
		return api.EffectiveOutput{}, fmt.Errorf("%w: object type %q is not declared, so it declares no propagation or resolution_strategy", cwmapi.ErrInvalidInput, in.Type)
	}
	kind, err := api.ContextOf(kindType)
	if err != nil {
		return api.EffectiveOutput{}, fmt.Errorf("%w: %v", cwmapi.ErrInvalidInput, err)
	}
	if kind.ResolutionStrategy == api.ResolutionPerCriterionOverride {
		// It reads each object's BODY; the sources reach the reduction as
		// briefs. Running it would return an empty set that looks like
		// "nothing inherited".
		return api.EffectiveOutput{}, fmt.Errorf("%w: type %q declares resolution_strategy %q, which reads each object's body and is not yet evaluated on the object side",
			cwmapi.ErrNotImplemented, in.Type, kind.ResolutionStrategy)
	}
	strategy := kind.ResolutionStrategy
	if strategy == "" {
		strategy = api.DefaultResolutionStrategy
	}
	propagation := kind.Propagation
	if propagation == "" {
		propagation = api.DefaultPropagation
	}
	b := briefer{r: r, ctx: ctx, kinds: map[string]api.ContextSettings{in.Type: kind}}

	local := b.childrenOfType(focused.ID, in.Type)
	var visit []string
	switch propagation {
	case api.PropagationInheritFromAncestors, api.PropagationPerCriterion:
		visit = b.ancestors(focused)
	case api.PropagationPropagateToAncestors, api.PropagationLocalVisibleUp:
		visit = b.descendants(focused.ID)
	}
	var tree []cwmapi.ObjectBrief
	for _, v := range visit {
		tree = append(tree, b.childrenOfType(v, in.Type)...)
	}
	// The Type source is the FOCUSED object's type's: "every object of my type
	// carries these" is a statement about the holder.
	var injected []cwmapi.ObjectBrief
	if ft, ok := r.objectType(ctx, focused.Type); ok {
		fc, err := api.ContextOf(ft)
		if err != nil {
			return api.EffectiveOutput{}, fmt.Errorf("%w: %v", cwmapi.ErrInvalidInput, err)
		}
		for _, br := range fc.InjectedObjects {
			if br.Type == in.Type || br.Type == "" {
				x := br
				if x.Type == "" {
					x.Type = in.Type
				}
				injected = append(injected, x)
			}
		}
	}

	briefs, bySource, ok := resolver.Reduce(strategy, resolver.StrategyInput{Local: local, Tree: tree, Types: injected, OverrideKey: kind.OverrideKey})
	if !ok {
		return api.EffectiveOutput{}, fmt.Errorf("%w: unknown resolution_strategy %q on type %q; the declared ones are %s",
			cwmapi.ErrInvalidInput, strategy, in.Type, strings.Join(resolver.StrategyNames(), ", "))
	}
	out := api.EffectiveOutput{Objects: briefs, Strategy: strategy, Propagation: propagation}
	if out.Objects == nil {
		out.Objects = []cwmapi.ObjectBrief{}
	}
	if in.IncludeBy {
		bs := bySource
		out.BySource = &bs
	}
	return out, nil
}

// briefer walks the tree and projects objects onto briefs.
type briefer struct {
	r     reader
	ctx   context.Context
	kinds map[string]api.ContextSettings // each type's context section
}

func (b briefer) childrenOfType(id, typ string) []cwmapi.ObjectBrief {
	var out []cwmapi.ObjectBrief
	for _, c := range b.r.children(b.ctx, id, typ) {
		out = append(out, b.brief(c))
	}
	return out
}

// ancestors is the parent chain, closest first. A cycle stops the walk — the
// containment lint reports it.
func (b briefer) ancestors(obj cwmapi.Object) []string {
	var chain []string
	seen := map[string]bool{obj.ID: true}
	cur := obj
	for {
		parent := cur.ParentID()
		if parent == "" || seen[parent] {
			return chain
		}
		seen[parent] = true
		chain = append(chain, parent)
		next, err := b.r.get(b.ctx, parent)
		if err != nil {
			return chain
		}
		cur = next
	}
}

// descendants is every descendant, breadth-first.
func (b briefer) descendants(id string) []string {
	seen := map[string]bool{id: true}
	var out []string
	frontier := []string{id}
	for len(frontier) > 0 {
		var next []string
		for _, cur := range frontier {
			for _, c := range b.r.children(b.ctx, cur, "") {
				if !seen[c.ID] {
					seen[c.ID] = true
					out = append(out, c.ID)
					next = append(next, c.ID)
				}
			}
		}
		frontier = next
	}
	return out
}

// brief projects an object onto the shape the strategies consume, carrying
// its type's override key and briefing fields — looked up across its
// capability groups, then its type's fields (one read, only when a declared
// field is in no group).
func (b briefer) brief(obj cwmapi.Object) cwmapi.ObjectBrief {
	brief := cwmapi.ObjectBrief{ID: obj.ID, Type: obj.Type, Source: obj.ParentID(), Title: obj.Name}
	if life, ok := obj.Lifecycle(); ok {
		brief.Summary = life.State
	}
	kind, ok := b.kinds[obj.Type]
	if !ok {
		t, declared := b.r.objectType(b.ctx, obj.Type)
		if !declared {
			return brief
		}
		// A section that does not decode carries no field into the brief;
		// CWM refuses such a type when it is declared.
		kind, _ = api.ContextOf(t)
		b.kinds[obj.Type] = kind
	}
	var full *cwmapi.Object
	add := func(name string) {
		if name == "" {
			return
		}
		v := fieldAcrossCapabilities(obj, name)
		if v == "" {
			if full == nil {
				got, err := b.r.get(b.ctx, obj.ID)
				if err != nil {
					got = obj
				}
				full = &got
			}
			v = scalarString(full.Fields[name])
		}
		if v != "" {
			if brief.StructuredFields == nil {
				brief.StructuredFields = map[string]string{}
			}
			brief.StructuredFields[name] = v
		}
	}
	add(kind.OverrideKey)
	for _, name := range kind.BriefingFields {
		add(name)
	}
	return brief
}

// fieldAcrossCapabilities finds a named scalar in any capability group, in
// sorted group order so two groups carrying the name resolve the same way.
func fieldAcrossCapabilities(obj cwmapi.Object, name string) string {
	ids := make([]string, 0, len(obj.Capabilities))
	for k := range obj.Capabilities {
		ids = append(ids, k)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if v, ok := obj.Capabilities[id][name]; ok {
			if s := scalarString(v); s != "" {
				return s
			}
		}
	}
	return ""
}

// scalarString renders a scalar as the strategies' string key; a list or a
// map is not rendered — its JSON spelling is a comparison nobody declared.
func scalarString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		return fmt.Sprintf("%v", t)
	case int:
		return fmt.Sprintf("%d", t)
	}
	return ""
}
