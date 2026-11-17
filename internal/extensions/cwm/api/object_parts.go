// SPDX-License-Identifier: Apache-2.0

package api

// ObjectDiagnostic is one structured problem flagged on an object. Code is
// a stable machine token (e.g. "parse-error", "type-unloadable",
// "schema-violation"); Label is a short human headline; Description
// carries the full message (parser line/col, schema path, etc.).
type ObjectDiagnostic struct {
	Code        string `json:"code"                  yaml:"code"                  jsonschema:"Stable diagnostic token"`
	Label       string `json:"label"                 yaml:"label"                 jsonschema:"Short human headline"`
	Description string `json:"description,omitempty" yaml:"description,omitempty" jsonschema:"Full message (line/col, schema path, …)"`
}

// BadgeDef declares one header badge derived from a field of the object, so it
// can never disagree with it. On a capability the field is one of its own
// (`state`); on a type it is a path — `<capability>.<field>`, `body.<key>`, or
// a core field (`slug`).
//
//	badges:
//	  - field: state
//	    icon: progress
//	    tooltip: Where the object stands in its workflow
//	    colors: {draft: gray, active: blue, done: green}
type BadgeDef struct {
	Field   string            `json:"field"             yaml:"field"             jsonschema:"The field the badge shows: the capability's own or a path"`
	Label   string            `json:"label,omitempty"   yaml:"label,omitempty"   jsonschema:"A prefix shown before the value; none = the value alone"`
	Icon    string            `json:"icon,omitempty"    yaml:"icon,omitempty"    jsonschema:"An icon name"`
	Tooltip string            `json:"tooltip,omitempty" yaml:"tooltip,omitempty" jsonschema:"Shown on hover"`
	Colors  map[string]string `json:"colors,omitempty"  yaml:"colors,omitempty"  jsonschema:"value → color name; unmapped values render gray"`
	// Capability is the capability that declared it — set by CWM when it
	// gathers the surface, never authored.
	Capability string `json:"capability,omitempty" yaml:"-"`
}

// CapabilityRef is one capability in force on an object, as the resolver
// answers it. Schema is the data-type URI describing its field-group; the
// object stores the group's values under `capabilities.<id>`.
type CapabilityRef struct {
	ID     string `json:"id"               yaml:"id"               jsonschema:"Capability id; its values live under capabilities.<id> on the object"`
	Schema string `json:"schema,omitempty" yaml:"schema,omitempty" jsonschema:"DataType URI for this capability's field-group (resolved via GetSchemaAST)"`
	Label  string `json:"label,omitempty"  yaml:"label,omitempty"  jsonschema:"Section header; defaults to the id"`
	// Verify says the capability declares a `verify` hook: an object carrying
	// it can be verified on its own (the verification service's Verify).
	Verify bool `json:"verify,omitempty" yaml:"-" jsonschema:"It declares a verify hook; derived and never authored"`
	// Source says where this capability came from — "type" (its type's bundle),
	// "object" (carried by the object itself) or "attached" (put on every object
	// of its types by the capability's own selector). Derived at resolve time,
	// never authored.
	Source string `json:"source,omitempty" yaml:"-" jsonschema:"type | aspect:<name> | object | attached — who brings it; derived and never authored"`
}

// ----------------------------------------------------------------------
// Picker support — drives the GUI "+ new child" flow
// ----------------------------------------------------------------------

// TypeChoice is one entry in a picker list returned by
// `node.list-allowed-child-types` or `node.list-allowed-item-types`.
// Carries enough for a kind-picker dialog to render a row + select
// the chosen type. The schema for the chosen type is fetched in a
// separate roundtrip via `node-type.get` / `item-type.get`.
type TypeChoice struct {
	Name        string `json:"name"                   jsonschema:"Type / kind name (canonical)"`
	Label       string `json:"label,omitempty"        jsonschema:"Human-readable display label; falls back to Name when empty"`
	Icon        string `json:"icon,omitempty"         jsonschema:"Icon identifier"`
	Description string `json:"description,omitempty"  jsonschema:"Short one-line description for the picker row"`
	// CreateTask is the task that creates an object of this type in place of
	// the plain form (ObjectType.CreateTask): the picker opens it.
	CreateTask string `json:"create_task,omitempty" jsonschema:"Task URI run instead of the plain create form"`
	// Named: the parent's type names it among its allowed children, or it
	// names the parent's type among the parents it accepts ("*", any parent,
	// names none) - not offered only because nothing restricts it. « + New » here offers the named ones.
	Named bool `json:"named,omitempty" jsonschema:"The parent's type names it or it names its accepted parents - not offered only because nothing restricts it"`
}

// SystemTaskChatURI is the canonical predefined URI for "open a chat
// about this node". Always included in the QuickActions set on every
// node's NodeDetail bundle. The GUI maps it to the per-node chat
// surface in the Node tab's vertical strip.
const SystemTaskChatURI = "system:/tasks/chat"

// ----------------------------------------------------------------------
// Actions (cwm-v3 §2.2 — bidirectional source UNION)
// ----------------------------------------------------------------------

// Action is one entry of an action surface (actions' action-surface/List).
// Represents a task that can be invoked on the given object, sourced
// from either the type side (the object type's `actions:`) or the
// task side (TASK.md frontmatter `kind: object` + `types:` + optional
// `filter:` el predicate). Effective set = deduped UNION of both
// sources.
type Action struct {
	URI            string         `json:"uri"                       jsonschema:"Action URI ready to invoke through ActionService"`
	Label          string         `json:"label"                     jsonschema:"Human-readable label (binding-site override wins)"`
	Description    string         `json:"description,omitempty"     jsonschema:"Short human description shown under the label; from the binding-site override or the target task"`
	Icon           string         `json:"icon,omitempty"            jsonschema:"Icon identifier"`
	Kind           string         `json:"kind"                      jsonschema:"sync | async | ai"`
	Source         string         `json:"source"                    jsonschema:"Provenance: 'node-type' (declared on the object type's actions) | 'applies-to' (declared on the task's frontmatter via kind and types)"`
	Confirm        bool           `json:"confirm,omitempty"         jsonschema:"Caller must show a confirmation dialog before Run"`
	Preview        bool           `json:"preview,omitempty"         jsonschema:"Future-surface action: GUI shows an 'À venir' badge + coming-soon panel on click; never dispatches or mutates"`
	Quick          bool           `json:"quick,omitempty"           jsonschema:"Declared quick: also a small button on the object's first page"`
	Disabled       bool           `json:"disabled,omitempty"        jsonschema:"Binding's when-predicate is false (or failed) for this node right now"`
	DisabledReason string         `json:"disabled_reason,omitempty" jsonschema:"Why the action is disabled — shown as the tooltip"`
	BoundInput     map[string]any `json:"bound_input,omitempty"     jsonschema:"Pre-bound input values from the binding; merged under the caller's payload at launch"`
}
