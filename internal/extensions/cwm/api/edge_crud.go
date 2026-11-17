// SPDX-License-Identifier: Apache-2.0

package api

// EdgeCreateInput requests creation of a typed edge between two
// existing nodes. Both endpoints MUST already exist; the verb
// returns api.ErrInvalidInput when either is missing. Edge kind
// MUST be registered in the kinds registry; unknown kinds return
// api.ErrInvalidInput.
//
// Special case: api.EdgeTypeParentID is reserved — the containment parent
// is the object's `parent` (ObjectCreate, ObjectUpdate, ObjectMove), which
// keeps the tree invariants enforced. EdgeCreate with kind=
// parent_id returns api.ErrTypeRuleViolation.
type EdgeCreateInput struct {
	From string   `json:"from"          jsonschema:"Source object id; required"`
	To   string   `json:"to"            jsonschema:"Target object id; required"`
	Kind EdgeType `json:"kind"          jsonschema:"Edge type name; required (must be registered)"`
}

// EdgeCreateOutput returns the created edge.
type EdgeCreateOutput struct {
	Edge Edge `json:"edge"`
}

// EdgeListInput requests the edges incident to an object. When ObjectID
// is empty, returns all edges across the graph (use sparingly on
// large workspaces). Kind filters the result; Direction further
// narrows to outgoing/incoming/both.
type EdgeListInput struct {
	ObjectID  string    `json:"object_id,omitempty"   jsonschema:"Filter to edges incident to this object id; empty = all edges"`
	Kind      EdgeType  `json:"kind,omitempty"        jsonschema:"Filter to a single edge kind; empty = all kinds"`
	Direction Direction `json:"direction,omitempty"   jsonschema:"out | in | both; defaults to both"`
}

// Direction is the edge-walk direction.
type Direction string

const (
	// DirectionBoth returns the union of outgoing + incoming edges (default).
	DirectionBoth Direction = ""
	// DirectionOut returns only edges where the object is the From endpoint.
	DirectionOut Direction = "out"
	// DirectionIn returns only edges where the object is the To endpoint.
	DirectionIn Direction = "in"
)

// EdgeListOutput returns the matching edges. Sorted by (From, To,
// Kind) for byte-stable output across runs.
type EdgeListOutput struct {
	Edges []Edge `json:"edges"`
}

// EdgeDeleteInput removes a specific (From, To, Kind) triple. An absent edge
// is refused with ErrNotFound: a wrong id or kind must not look like success.
type EdgeDeleteInput struct {
	From string   `json:"from"          jsonschema:"Source object id; required"`
	To   string   `json:"to"            jsonschema:"Target object id; required"`
	Kind EdgeType `json:"kind"          jsonschema:"Edge type name; required"`
}

// EdgeDeleteOutput is empty — success vs failure flows through the
// returned error.
type EdgeDeleteOutput struct{}
