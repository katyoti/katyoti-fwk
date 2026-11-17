// SPDX-License-Identifier: Apache-2.0

package api

import "fmt"

// EdgeTypeRegistration is the lightweight read-only view of one
// edge-type declaration (a pack's `edge-types/{name}.yaml`).
type EdgeTypeRegistration struct {
	Name        EdgeType `json:"name"                        yaml:"name"`
	From        string   `json:"from"                        yaml:"from"                      jsonschema:"Source kind/type (e.g. task, any-item)"`
	To          string   `json:"to"                          yaml:"to"                        jsonschema:"Target kind/type (e.g. requirement, code-symbol)"`
	Cardinality string   `json:"cardinality,omitempty"       yaml:"cardinality,omitempty"     jsonschema:"1:1 | 1:N | N:1 | M:N (source:target); empty = M:N"`
	InverseName string   `json:"inverse_name,omitempty"      yaml:"inverse_name,omitempty"    jsonschema:"Optional name for traversal in the opposite direction"`
	Description string   `json:"description,omitempty"       yaml:"description,omitempty"`
	// Label and InverseLabel are what a person reads of a link of this kind,
	// from its source and from its target (the Links tab); the names stay the
	// identifiers. Translatable (contribution-type edge-type).
	Label        string `json:"label,omitempty"         yaml:"label,omitempty"         jsonschema:"The link as a person reads it from its source; empty = the name"`
	InverseLabel string `json:"inverse_label,omitempty" yaml:"inverse_label,omitempty" jsonschema:"The link as a person reads it from its target; empty = the inverse name"`

	// THE OBJECT SIDE (phase 7e, identity-and-index.md §3): who may take part,
	// in the model's atom — capabilities — what the right side is, and where
	// the edge is stored, once. When FromCapabilities / ToCapabilities are set
	// they are THE constraint and the type names above are documentation.
	FromCapabilities []string `json:"from_capabilities,omitempty" yaml:"from_capabilities,omitempty" jsonschema:"Capabilities the SOURCE object must carry (effective set); supersedes from"`
	ToCapabilities   []string `json:"to_capabilities,omitempty"   yaml:"to_capabilities,omitempty"   jsonschema:"Capabilities the TARGET object must carry, checked when it is in the index; supersedes to"`
	Target           string   `json:"target,omitempty"            yaml:"target,omitempty"            jsonschema:"What the right side is: object (default — an id in this project) | path (a file in the project) | uri (a resource or tool URI)"`
	ObjectStorage    string   `json:"object_storage,omitempty"    yaml:"object_storage,omitempty"    jsonschema:"Where the edge is stored, once: edges (an entry in the source's edges list — the default) | field (a property on one side, named by field)"`
	Field            string   `json:"field,omitempty"             yaml:"field,omitempty"             jsonschema:"For object_storage field: <side>.<field> (left.parent) or <side>.<capability>.<property>"`
}

// Edge target kinds and storage kinds (EdgeTypeRegistration.Target / ObjectStorage).
const (
	EdgeTargetObject = "object"
	EdgeTargetPath   = "path"
	EdgeTargetURI    = "uri"

	EdgeStorageEdges = "edges"
	EdgeStorageField = "field"
)

// TargetKind returns the declared target kind, defaulting to object.
func (r EdgeTypeRegistration) TargetKind() string {
	if r.Target == "" {
		return EdgeTargetObject
	}
	return r.Target
}

// Edge cardinalities, source:target. An N:1 or 1:1 kind is carried at most
// once by a source object.
const (
	Cardinality11 = "1:1"
	Cardinality1N = "1:N"
	CardinalityN1 = "N:1"
	CardinalityMN = "M:N"
)

// Validate refuses a declaration CWM cannot honour: no name, or a cardinality
// outside 1:1 | 1:N | N:1 | M:N. Empty cardinality is M:N.
func (r EdgeTypeRegistration) Validate() error {
	if r.Name == "" {
		return fmt.Errorf("%w: edge type has no name", ErrInvalidInput)
	}
	switch r.Cardinality {
	case "", Cardinality11, Cardinality1N, CardinalityN1, CardinalityMN:
		return nil
	}
	return fmt.Errorf("%w: edge type %q: cardinality %q is not one of 1:1, 1:N, N:1, M:N", ErrInvalidInput, r.Name, r.Cardinality)
}

// StoredAsField reports a kind stored as a property on one side, never in an
// edges list.
func (r EdgeTypeRegistration) StoredAsField() bool { return r.ObjectStorage == EdgeStorageField }
