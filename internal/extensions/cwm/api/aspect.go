// SPDX-License-Identifier: Apache-2.0

package api

// Aspect is a named list of capabilities added to an object in one move
// (identity-and-index.md §8, phase 7g). The capability stays the atom, the
// type stays the identity; the aspect is the reusable bundle that is not an
// identity — `aspects: [isa]` on any object brings `want` + `checkable`.
//
// Effective capabilities = type ∪ aspects (expanded, declaration order) ∪ the
// instance's groups, read by one function in the core.
type Aspect struct {
	Name         string   `json:"name"                   yaml:"name"                   jsonschema:"Aspect id: lowercase kebab, what an object names in aspects:"`
	Label        string   `json:"label,omitempty"        yaml:"label,omitempty"`
	Description  string   `json:"description,omitempty"  yaml:"description,omitempty"`
	Capabilities []string `json:"capabilities"           yaml:"capabilities"           jsonschema:"The capability ids this aspect adds, in order; each must be declared"`
	// Pane is how an object carrying the aspect shows in the object pane — an
	// opaque section CWM carries and never reads; the object pane types it
	// (objectpane api.AspectPaneOf).
	Pane map[string]any `json:"pane,omitempty" yaml:"pane,omitempty" jsonschema:"How an object carrying this aspect shows in the object pane; read by the object pane (tabs: the main tabs in order)"`
	// CreateTask is the task that creates an object carrying the aspect: « +
	// New » offers the aspect beside the object types and opens it.
	CreateTask string `json:"create_task,omitempty" yaml:"create_task,omitempty" jsonschema:"The task that creates an object carrying this aspect; + New offers the aspect and opens it"`
	// Provider is the binary that declared the aspect. Set when the catalog is
	// read; never authored, never on the wire.
	Provider string `json:"-" yaml:"-"`
}
