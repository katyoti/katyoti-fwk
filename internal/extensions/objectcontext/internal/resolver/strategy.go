// SPDX-License-Identifier: Apache-2.0

package resolver

import (
	"sort"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	ocapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectcontext/api"
)

// StrategyInput is the value passed to every Strategy: the three sources an
// object's effective set is reduced from, already in brief shape, and the
// type's override key.
type StrategyInput struct {
	// Local are the focused object's own children of the type.
	Local []api.ObjectBrief
	// Tree are what propagation reaches, closest-first.
	Tree []api.ObjectBrief
	// Types are what the focused object's type injects.
	Types []api.ObjectBrief
	// OverrideKey is the field closest-wins and accumulate-with-local-override
	// resolve on; an object's may sit in any capability group, so the briefs
	// carry it in StructuredFields.
	OverrideKey string
}

func (in StrategyInput) localSource() []api.ObjectBrief { return in.Local }
func (in StrategyInput) treeSource() []api.ObjectBrief  { return in.Tree }

// Strategy reduces a StrategyInput into a flat ObjectBrief slice
// and a diagnostic by-source grouping.
type Strategy func(StrategyInput) ([]api.ObjectBrief, ocapi.BySource)

// builtinStrategies is the registry of the strategies a
// capability's reduction can name; it is closed — there is no
// registration of more.
var builtinStrategies = map[string]Strategy{
	"accumulate":                     accumulate,
	"closest-wins":                   closestWins,
	"accumulate-with-local-override": accumulateWithLocalOverride,
	"local-only":                     localOnly,
}

// accumulate: union all sources, dedupe by ID. Order: Local first,
// then Tree (closest-first), then Type. Sufficient for kinds like
// rules, patterns, goals where every item is unique.
func accumulate(in StrategyInput) ([]api.ObjectBrief, ocapi.BySource) {
	var bs ocapi.BySource
	seen := map[string]struct{}{}
	var out []api.ObjectBrief

	push := func(brief api.ObjectBrief, where *[]api.ObjectBrief) {
		if _, dup := seen[brief.ID]; dup {
			return
		}
		seen[brief.ID] = struct{}{}
		out = append(out, brief)
		*where = append(*where, brief)
	}
	for _, br := range in.localSource() {
		push(br, &bs.Local)
	}
	for _, br := range in.treeSource() {
		push(br, &bs.Tree)
	}
	for _, br := range in.Types {
		push(br, &bs.Type)
	}
	return out, bs
}

// closestWins: walk Local → Tree (closest-first) → Type, the first
// item per `Reg.OverrideKey` wins. Without an override key,
// degrades to accumulate (matches concepts.md § 4.2 narrative).
func closestWins(in StrategyInput) ([]api.ObjectBrief, ocapi.BySource) {
	if in.OverrideKey == "" {
		return accumulate(in)
	}
	var bs ocapi.BySource
	seen := map[string]struct{}{}
	var out []api.ObjectBrief

	consider := func(brief api.ObjectBrief, where *[]api.ObjectBrief) {
		key := brief.StructuredFields[in.OverrideKey]
		if key == "" {
			// No key in this brief — treat as unique row (still pushed).
			out = append(out, brief)
			*where = append(*where, brief)
			return
		}
		if _, taken := seen[key]; taken {
			return // closer hit already won
		}
		seen[key] = struct{}{}
		out = append(out, brief)
		*where = append(*where, brief)
	}
	for _, br := range in.localSource() {
		consider(br, &bs.Local)
	}
	for _, br := range in.treeSource() {
		consider(br, &bs.Tree)
	}
	for _, br := range in.Types {
		consider(br, &bs.Type)
	}
	return out, bs
}

// accumulateWithLocalOverride: union all, but Local items with the
// same OverrideKey replace same-key items from Tree/Type. Without
// an override key, degrades to plain accumulate.
func accumulateWithLocalOverride(in StrategyInput) ([]api.ObjectBrief, ocapi.BySource) {
	if in.OverrideKey == "" {
		return accumulate(in)
	}
	var bs ocapi.BySource
	localKeys := map[string]struct{}{}
	for _, brief := range in.localSource() {
		bs.Local = append(bs.Local, brief)
		if k := brief.StructuredFields[in.OverrideKey]; k != "" {
			localKeys[k] = struct{}{}
		}
	}
	pushIfNotShadowed := func(brief api.ObjectBrief, where *[]api.ObjectBrief) {
		key := brief.StructuredFields[in.OverrideKey]
		if key != "" {
			if _, shadowed := localKeys[key]; shadowed {
				return
			}
		}
		*where = append(*where, brief)
	}
	for _, br := range in.treeSource() {
		pushIfNotShadowed(br, &bs.Tree)
	}
	for _, br := range in.Types {
		pushIfNotShadowed(br, &bs.Type)
	}
	// Order: Local first, then surviving Tree, then Type.
	out := append([]api.ObjectBrief{}, bs.Local...)
	out = append(out, bs.Tree...)
	out = append(out, bs.Type...)
	return out, bs
}

// localOnly: ignore Tree + Type entirely.
func localOnly(in StrategyInput) ([]api.ObjectBrief, ocapi.BySource) {
	var bs ocapi.BySource
	local := in.localSource()
	out := make([]api.ObjectBrief, 0, len(local))
	for _, brief := range local {
		out = append(out, brief)
		bs.Local = append(bs.Local, brief)
	}
	return out, bs
}

// Reduce runs a named strategy over pre-collected sources.
func Reduce(name string, in StrategyInput) ([]api.ObjectBrief, ocapi.BySource, bool) {
	fn, ok := builtinStrategies[name]
	if !ok {
		return nil, ocapi.BySource{}, false
	}
	items, bs := fn(in)
	return items, bs, true
}

// StrategyNames lists the reductions that exist, for an error that can name
// the alternatives instead of only the mistake.
func StrategyNames() []string {
	out := make([]string, 0, len(builtinStrategies))
	for name := range builtinStrategies {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
