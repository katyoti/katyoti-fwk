// SPDX-License-Identifier: Apache-2.0

package api

import cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"

// THE `context` CAPABILITY — object-context's settings on a type.
//
// A type writes them in its `context:` section; CWM keeps the section and
// checks it against the capability's type_schema, and this service reads it
// (cwmapi.TypeSection). `context` is a type-level capability: it carries no
// group on objects.
//
//	name: decision
//	context:
//	  propagation: inherit-from-ancestors
//	  resolution_strategy: closest-wins
//	  override_key: topic
//
// WHAT IT ANSWERS (Effective): "which objects of type K are in force at
// object X". Three sources: X's own children of type K; the children of type K
// of the objects K's `propagation` reaches — X's ancestors, its descendants,
// or none; and the `injected_objects` of type K that X's own type declares.
// They are projected onto briefs (with K's `override_key` and
// `briefing_fields`) and reduced by K's `resolution_strategy`.

// ContextCapability is the capability — and the section name on a type.
const ContextCapability = "context"

// ContextSettings is a type's `context:` section. The Go type is the one
// source of its schema: the `context-settings` data-type is generated from it.
type ContextSettings struct {
	Propagation        string               `json:"propagation,omitempty"         yaml:"propagation,omitempty"         jsonschema:"description=How far objects of this type reach along the tree: inherit-from-ancestors | propagate-to-ancestors | local | local-visible-up | per-criterion; local when empty,enum=inherit-from-ancestors|propagate-to-ancestors|local|local-visible-up|per-criterion"`
	ResolutionStrategy string               `json:"resolution_strategy,omitempty" yaml:"resolution_strategy,omitempty" jsonschema:"description=How competing objects of this type combine: accumulate | closest-wins | accumulate-with-local-override | local-only | per-criterion-override; accumulate when empty,enum=accumulate|closest-wins|accumulate-with-local-override|local-only|per-criterion-override"`
	OverrideKey        string               `json:"override_key,omitempty"        yaml:"override_key,omitempty"        jsonschema:"description=The field closest-wins and accumulate-with-local-override resolve on"`
	BriefingFields     []string             `json:"briefing_fields,omitempty"     yaml:"briefing_fields,omitempty"     jsonschema:"description=Fields of an object of this type carried into its brief"`
	InjectedObjects    []cwmapi.ObjectBrief `json:"injected_objects,omitempty"    yaml:"injected_objects,omitempty"    jsonschema:"description=Objects every object of this type carries before the tree's"`
}

// Propagation values — how far a type's objects reach along the tree.
const (
	// PropagationInheritFromAncestors — an object sees what its ancestors hold.
	PropagationInheritFromAncestors = "inherit-from-ancestors"
	// PropagationPropagateToAncestors — an object sees what its descendants hold.
	PropagationPropagateToAncestors = "propagate-to-ancestors"
	// PropagationLocalVisibleUp — local, but visible to ancestors, which from
	// the asker's side is the same downward walk.
	PropagationLocalVisibleUp = "local-visible-up"
	// PropagationLocal — no walk at all.
	PropagationLocal = "local"
	// PropagationPerCriterion — the ancestor walk, with per-element filtering
	// done by the per-criterion-override strategy.
	PropagationPerCriterion = "per-criterion"
)

// Defaults when a type declares neither.
//
// LOCAL AND ACCUMULATE, deliberately: a kind that says nothing about reach
// should reach nothing, and a kind that says nothing about conflict should
// drop nothing. Both defaults are the ones that cannot silently hide an
// object — the opposite choice would make an undeclared kind inherit half the
// tree and quietly discard the duplicates.
const (
	DefaultPropagation        = PropagationLocal
	DefaultResolutionStrategy = "accumulate"
)

// ResolutionPerCriterionOverride reads each object's BODY, so it cannot run
// over briefs: Effective refuses it.
const ResolutionPerCriterionOverride = "per-criterion-override"

// ContextOf is a type's `context` section, zero when it writes none.
func ContextOf(t cwmapi.ObjectType) (ContextSettings, error) {
	s, _, err := cwmapi.TypeSection[ContextSettings](t, ContextCapability)
	return s, err
}

// AgentContextKind is the contribution kind of an extension's framing; a pack
// carries them in `agent-contexts/` (contrib.Kinds).
const AgentContextKind = "agent-context"

// AgentFraming is an `agent-context` contribution: the framing an extension
// gives a chat on every object its filter admits — the extension layer of
// object-context agent context, between the rules and the capabilities. The
// goals extension says how goals are managed and brings the goal tools here,
// once, for every goal. Declared and read by object-context; a pack carries
// them in `agent-contexts/`.
type AgentFraming struct {
	Name        string              `json:"name"                  yaml:"name"                  jsonschema:"Unique name within the provider; defaults to the file name"`
	Description string              `json:"description,omitempty" yaml:"description,omitempty" jsonschema:"What this framing is for"`
	Filter      cwmapi.ObjectFilter `json:"filter,omitempty"      yaml:"filter,omitempty"      jsonschema:"The objects it frames (types / capabilities / when); empty frames every object"`
	Agent       cwmapi.AgentDecl    `json:"agent"                 yaml:"agent"                 jsonschema:"What a chat on those objects receives"`
	// Provider is who contributed it, set when the catalog is read — never
	// authored, never on the wire.
	Provider string `json:"-" yaml:"-"`
}
