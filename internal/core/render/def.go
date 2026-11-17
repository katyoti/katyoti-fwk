// SPDX-License-Identifier: Apache-2.0

// Package render holds the ONE definition of something to display.
//
// A panel on an object, the result of a method a panel calls, a display the
// LLM triggers and an artifact the terminal mounts are all the same thing: a
// component to render and the data it renders. They used to be four shapes
// naming the same fields differently; they are now this struct, embedded where
// a consumer needs more (the LLM tool adds an artifact id and action
// shortcuts, the host content adds its message type).
//
// Enriching what can be displayed is therefore one change: add a kind or an
// extension here and every surface that takes a Def can show it.
//
// The fields are the one component vocabulary — `schema`, `variant`, `ui`,
// `data` — spoken by the LLM tool, object panels, the page DSL and the GUI
// (modules/display/document-display.md).
//
// A leaf: it imports nothing from the project, so every layer can embed it.
package render

// Def is one thing to display: which component, and what it shows. It is the
// ONE component object (modules/display/document-display.md): the same fields
// serve a chat artifact, an object panel's content or its source's answer, a
// node nested in a page and a file.
//
//   - a ui-component, by kind: form | view | table | tree-table | wizard read
//     their Schema (a data-type URI, or its AST inline), the Variant of its
//     ui-<kind> extension, an optional inline UI block and their Data; page is
//     a tree of nodes in DSLTree bound to Data; extension is a NAMED UI
//     extension (a registered view such as What's Next) given Data as input;
//   - a document, by MediaType (or a document kind: markdown, html, mermaid,
//     drawio, deck), its content from Data or File.
//
// A schema named by id and a UI block are resolved by the host (DocumentOpen)
// before display. Document() turns a Def into the envelope the display entry
// point takes.
type Def struct {
	Kind string `json:"kind,omitempty" yaml:"kind,omitempty" jsonschema:"description=Component type: form | view | table | tree-table | wizard for a schema-bound surface; page for a DSL tree; extension for a named UI extension; or a document kind (markdown | html | mermaid | drawio | deck)"`

	// MediaType makes the Def a document of that type (any type a display
	// engine accepts: application/pdf, image/png, text/x-go, …).
	MediaType string `json:"mediaType,omitempty" yaml:"mediaType,omitempty" jsonschema:"description=Media type of a document to display from file or data; overrides the kind"`

	// Extension names the registered UI extension to mount when Kind is
	// "extension".
	Extension string `json:"extension,omitempty" yaml:"extension,omitempty" jsonschema:"description=Registered UI extension to mount when kind is extension"`

	// Schema is a data-type URI (string) or a schema AST (object).
	Schema  any            `json:"schema,omitempty" yaml:"schema,omitempty"   jsonschema:"description=The schema of a schema-bound component: a data-type URI such as ktw-demo:/data-type/book (resolved by the host) or the schema AST inline"`
	Variant string         `json:"variant,omitempty" yaml:"variant,omitempty"  jsonschema:"description=Which ui-<kind> extension of the schema to use by name; default is default"`
	UI      map[string]any `json:"ui,omitempty" yaml:"ui,omitempty"       jsonschema:"description=An inline ui-<kind> extension block laid over the schema: it replaces the same-named extension or is added and becomes the variant"`
	Data    any            `json:"data,omitempty" yaml:"data,omitempty"     jsonschema:"description=The data: the value a component shows or edits (one record or the rows of a table); a page's outer data tree its nodes bind with dataField: '$.path'; an extension's input; a document kind's content (drawio {xml; filename})"`

	File string `json:"file,omitempty" yaml:"file,omitempty" jsonschema:"description=Path to an existing file (absolute or workspace-relative) or the resource URI of a published document (<provider>:/<kind>/<path> such as a theme's sample deck) that the host reads and renders. For kind drawio or deck: display an existing diagram or deck in one step without spending tokens on its content. Mutually exclusive with data"`
	Mode string `json:"mode,omitempty" yaml:"mode,omitempty" jsonschema:"description=CRUD mode. Default depends on kind: form edit / view view / table view / wizard create"`

	DSLTree any `json:"dslTree,omitempty" yaml:"dslTree,omitempty" jsonschema:"description=Root of the DSL tree (a UIPageNode). Required when kind is page. Its schema-bound nodes take schema / variant / ui / data like a component. A dashboard is built from: section (icon / title / count badge / aside nodes at its right / an empty state); card; badge (a tone color / an icon / a tooltip / mono); gauge (value against max or stacked segments {value; color; label} with a legend); kpi; markdown (maxHeight makes it collapsible); actions on a button / a list or table row / a link / a badge / a kpi — each names ONE verb: uri (a method run with args; it may answer {ok; message; open}) or navigate {object; panel} or chat {task; target} or create {type; parent} or query {args} (re-read the panel source with args over its input: a filter) or tab (show a tab of this page: its panel id) — with an optional confirm; the host runs it and re-reads the panel. An accordion folds panels {label; badge; content}. In Go build it with render.Node builders and check it with render.ValidatePage. See modules/display/ for the DSL grammar"`

	Title string `json:"title,omitempty" yaml:"title,omitempty" jsonschema:"description=Optional tab label. Defaults to a kind-derived name when omitted"`
}
