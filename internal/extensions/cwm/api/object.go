// SPDX-License-Identifier: Apache-2.0

package api

// Object is the ONE thing CWM stores.
//
// An object is an identity, the object core every object has, and composed
// capabilities (modules/cwm/concepts.md §3). Where it appears is its TYPE's:
// `tree: root | node` draws it in the tree, any other type is listed by the
// panel that shows it; a `document` is an object type like any other.
//
// Everything else a domain needs (want, checkable, confluence, …) is a
// capability, composed by the type, an aspect, the instance, or attached to
// the type (§4). CWM stores the field-groups and never interprets them.
//
// One object is persisted as exactly one YAML file under its module's folder,
// committed with the code (goals.md G1/G2).
type Object struct {
	// ID is the object's identity. It is read from the file CONTENT, never from
	// the filename — so renaming a file never changes what it identifies.
	ID string `json:"id" yaml:"id" jsonschema:"Object identity; stable, read from file content"`
	// Type names the default capability bundle this object composes (§4).
	Type string `json:"type" yaml:"type" jsonschema:"Type = a named capability bundle"`

	// THE OBJECT CORE — every object has it, whatever its type and capabilities:
	// what it is called, what it is about, where it sits, what it says. CWM's
	// own fields, never a capability (concepts.md §2). The `container` and
	// `content` groups that used to hold them are folded into them on read
	// (FoldLegacy).
	Name        string `json:"name,omitempty"        yaml:"name,omitempty"        jsonschema:"The object's label"`
	Description string `json:"description,omitempty" yaml:"description,omitempty" jsonschema:"What the object is about in a sentence or two"`
	Icon        string `json:"icon,omitempty"        yaml:"icon,omitempty"        jsonschema:"The object's icon; its type's when empty"`
	Parent      string `json:"parent,omitempty"      yaml:"parent,omitempty"      jsonschema:"The object this one sits under in the tree; empty = a root"`
	Order       int    `json:"order,omitempty"       yaml:"order,omitempty"       jsonschema:"Position among its siblings; lowest first; 0 = unordered"`
	Slug        string `json:"slug,omitempty"        yaml:"slug,omitempty"        jsonschema:"A short code for the object (C-1 or G-001); never its identity"`
	// Minter is who minted the object's number: set once at
	// create, never edited; an object minted before number blocks has none.
	// Of two objects carrying one id, the one whose minter does not own the
	// block holding the number is the one to renumber.
	Minter string   `json:"minter,omitempty" yaml:"minter,omitempty" jsonschema:"Who minted the object's number; set at create and never edited"`
	Tags   []string `json:"tags,omitempty"        yaml:"tags,omitempty"        jsonschema:"Free-form labels for filtering"`
	// Badges are the ones set explicitly — by a capability or a person; a
	// capability's derived badges are computed when shown.
	Badges []Badge `json:"badges,omitempty" yaml:"badges,omitempty" jsonschema:"Badges shown in the object's header"`
	// Fields are the TYPE's own fields — what this kind of thing is (a
	// criterion's claim, a use case's severity), shaped by the type's `fields:`
	// data-type. Only objects of that type have them; a field any type could
	// want is a capability instead, and what every object has is the core.
	Fields map[string]any `json:"fields,omitempty" yaml:"fields,omitempty" jsonschema:"The type's own fields, shaped by its fields data-type"`
	// Capabilities holds one field-group per composed capability, keyed by
	// capability id. Opaque to CWM: validation is schema-driven, meaning is the
	// contributing pack's business.
	Capabilities map[string]map[string]any `json:"capabilities,omitempty" yaml:"capabilities,omitempty" jsonschema:"Field-groups keyed by capability id"`
	// Edges are the object's authored relations to other objects. The tree is a
	// field (parent); EVERY other reference is an edge — the reference
	// rule, §3.
	Edges []ObjectEdge `json:"edges,omitempty" yaml:"edges,omitempty" jsonschema:"Authored relations from this object"`
	// Aspects are named capability lists this object adds in one move (§8,
	// phase 7g): `aspects: [isa]`. Expanded into the effective set by the core.
	Aspects []string `json:"aspects,omitempty" yaml:"aspects,omitempty" jsonschema:"Declared aspects this object carries; each expands to its capabilities"`
	// Files are this object's OWN named files, by name (2026-09-26): the
	// closest of the three sources, added at runtime through
	// ObjectNamedFileDeclare.
	Files map[string]NamedFile `json:"files,omitempty" yaml:"files,omitempty" jsonschema:"This object's own named files; each a module-relative path"`
	// Hooks are this object's OWN hook entries, by point — the closest
	// declaration, so they replace (or, with when: "false", remove) an entry of
	// the same uri from its type or a capability.
	Hooks Hooks `json:"hooks,omitempty" yaml:"hooks,omitempty" jsonschema:"This object's own hook entries by point"`
	// Agent is what a chat on this object receives from the object itself —
	// the closest layer of the object-context agent context.
	Agent *AgentDecl `json:"agent,omitempty" yaml:"agent,omitempty" jsonschema:"This object's own agent-context declaration"`
	// Body is what the object says, in prose — the markdown of a `.cwm.md`
	// file. Structured data is the type's Fields, never the body.
	Body string `json:"body,omitempty" yaml:"body,omitempty" jsonschema:"What the object says in prose (markdown)"`
}

// ObjectEdge is the AUTHORED form of a relation: `from` is implied by the object
// that declares it, so it is not repeated (repeating it invites the file and the
// id to disagree). It expands to the graph-form api.Edge on load — this is a
// projection of that type, not a second edge model.
type ObjectEdge struct {
	Kind string `json:"kind" yaml:"kind" jsonschema:"Edge kind (serves, realizes, impacts, …)"`
	To   string `json:"to"   yaml:"to"   jsonschema:"Target object id"`
}

// GraphEdges expands the authored edges into graph-form edges rooted at this
// object. Returns nil when the object declares none.
func (o Object) GraphEdges() []Edge {
	if len(o.Edges) == 0 {
		return nil
	}
	out := make([]Edge, 0, len(o.Edges))
	for _, e := range o.Edges {
		out = append(out, Edge{From: o.ID, To: e.To, Kind: EdgeType(e.Kind)})
	}
	return out
}

// EdgeTo returns the target of the object's first edge of kind, "" when it has
// none — the reader of an N:1 relation.
func (o Object) EdgeTo(kind string) string {
	for _, e := range o.Edges {
		if e.Kind == kind {
			return e.To
		}
	}
	return ""
}

// SetEdgeTo makes `to` the object's one edge of kind — the writer of an N:1
// relation; an empty `to` removes it.
func (o *Object) SetEdgeTo(kind, to string) {
	kept := make([]ObjectEdge, 0, len(o.Edges)+1)
	for _, e := range o.Edges {
		if e.Kind != kind {
			kept = append(kept, e)
		}
	}
	if to != "" {
		kept = append(kept, ObjectEdge{Kind: kind, To: to})
	}
	if len(kept) == 0 {
		kept = nil
	}
	o.Edges = kept
}

// Capability returns one capability's field-group and whether it is composed.
func (o Object) Capability(id string) (map[string]any, bool) {
	g, ok := o.Capabilities[id]
	return g, ok
}
