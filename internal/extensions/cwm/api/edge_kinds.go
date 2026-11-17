// SPDX-License-Identifier: Apache-2.0

package api

// EdgeType is the typed name of an edge kind (e.g. "parent_id",
// "verifies").
type EdgeType string

// Built-in (CWM-generic) edge kinds. Method packs declare additional
// edge kinds in their `_edge_kinds/` folder.
const (
	EdgeTypeParentID    EdgeType = "parent_id"
	EdgeTypeDerivedFrom EdgeType = "derived-from"
	EdgeTypeSupersedes  EdgeType = "supersedes"
)

// Edge is the kind-generic graph edge representation. Method packs
// may layer typed wrappers; the api/ stays small.
type Edge struct {
	From string   `json:"from"            jsonschema:"Source object id"`
	To   string   `json:"to"              jsonschema:"Target object id"`
	Kind EdgeType `json:"kind"            jsonschema:"Edge kind name; one of the declared edge types"`
	// Inverse is the declaration's inverse_name (served-by for serves), so a
	// reader sees the relation from the right side too. Empty when undeclared.
	Inverse string `json:"inverse,omitempty" jsonschema:"The kind read from the target's side, from the declaration's inverse_name"`
	// Label and InverseLabel are the declaration's names for a person, from
	// each side (label, inverse_label). Empty when it declares none.
	Label        string         `json:"label,omitempty"         jsonschema:"The link as a person reads it from its source, from the declaration"`
	InverseLabel string         `json:"inverse_label,omitempty" jsonschema:"The link as a person reads it from its target, from the declaration"`
	Props        map[string]any `json:"props,omitempty" jsonschema:"Edge-kind-specific properties; opaque to CWM"`
	// Ended is set by EdgeList when the target is an archived object
	// (ObjectArchive): the link is valid and its end has left the project.
	Ended bool `json:"ended,omitempty" jsonschema:"The target is an archived object: the link is valid and its end has ended"`
}
