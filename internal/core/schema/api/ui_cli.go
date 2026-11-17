// SPDX-License-Identifier: Apache-2.0

package schemaapi

// ExtensionUICLI is the well-known kind string for the CLI rendering hint
// extension attached to a Type AST. The terminal renderer in
// `internal/extensions/tui/render/` reads this extension off the output
// type of an AutoKTSP slot method and renders accordingly.
const ExtensionUICLI = "ui-cli"

// UICLIMode names the high-level rendering strategy for a Type's body
// onto a terminal. Defaults to UICLIModeText when the ui-cli extension
// is present but `mode` is unset; the renderer falls back to pretty JSON
// when no ui-cli extension is attached at all.
type UICLIMode string

const (
	// UICLIModeText renders one field per line in `key: value` form. The
	// `primary` field (if set) is colored / bolded on TTY output.
	UICLIModeText UICLIMode = "text"

	// UICLIModeTable renders an aligned table with one row per array
	// element. Requires the host Type to be Kind == array (the row Type is
	// the items'). `columns` selects/orders fields; empty = use every
	// public field of the row Type.
	UICLIModeTable UICLIMode = "table"

	// UICLIModeTree renders recursive data as an indented tree with
	// branch glyphs. Requires `tree-child-field` to point at the field
	// carrying the recursive child list.
	UICLIModeTree UICLIMode = "tree"

	// UICLIModeTreeTable renders a FLAT node array (linked by `parent-field`)
	// as an indented tree WITH aligned columns — the CLI twin of the GUI
	// tree-table widget. `tree-column` carries the indent; `column-kinds`
	// maps a column to a rich cell (version ✓/✎ · direction arrows · state).
	UICLIModeTreeTable UICLIMode = "tree-table"

	// UICLIModeJSON forces pretty-printed JSON regardless of TTY. Mostly
	// used as a flag-driven override (`--out json`) rather than declared
	// on the Type — the renderer also falls back to pretty JSON when no
	// ui-cli extension is present.
	UICLIModeJSON UICLIMode = "json"
)

// UICLI is the typed view of an `ui-cli` Extension's Payload. Build it
// from a raw `*Type` via `UICLIFromType` (which returns the parsed
// extension when present, or `nil, false` when absent).
//
// Renderers consume UICLI; they should never reach into Extension.Payload
// directly so the grammar stays centralised here.
type UICLI struct {
	// Mode is the high-level strategy. Empty defaults to text mode when
	// the renderer is given an object body, table mode when given an
	// array body. The Mode field is normalized by UICLIFromType.
	Mode UICLIMode

	// Columns selects + orders the fields rendered as table columns
	// (table mode). Empty = render every public field in DSL declaration
	// order.
	Columns []string

	// Primary names the field considered most salient — bolded / colored
	// in text mode, used as the row label in table mode when present.
	Primary string

	// RichOnlyFields lists field names that the renderer should drop in
	// plain mode (non-TTY, NO_COLOR). Useful for decorative-only fields
	// like icons or color hints that clutter piped output.
	RichOnlyFields []string

	// TreeChildField names the field that carries child rows in tree
	// mode. Required when Mode == UICLIModeTree.
	TreeChildField string

	// Sort lists fields to sort by (table mode). Empty = keep input
	// order.
	Sort []string

	// --- Tree-table mode (UICLIModeTreeTable) ---

	// ParentField links flat rows into a hierarchy: row[ParentField] points at
	// the parent row's IDField (empty / unknown ⇒ root). Distinct from
	// TreeChildField (which is nested-children).
	ParentField string

	// IDField names each node's id field (default "id").
	IDField string

	// StateField names the field rendered as a colored status label (default "state").
	StateField string

	// TreeColumn names the column that carries the indent + branch glyphs
	// (default: the first column).
	TreeColumn string

	// NodesField names the array field to render as the tree-table when the
	// output is an OBJECT (rather than a bare array) — e.g. a `SyncOutput`
	// whose `tree` field holds the nodes. Empty ⇒ the body is the node array
	// itself (or a single-array-field envelope).
	NodesField string

	// ColumnKinds maps a column name → a rich cell kind: "version" (✓/✎ +
	// value) | "direction" (▶▶/◀◀/═) | "state" (colored label).
	ColumnKinds map[string]string

	// ColumnLabels maps a column name → its header label (default: the field
	// name). Used by table + tree-table headers.
	ColumnLabels map[string]string

	// VersionStates maps a "version"-kind column → the field holding its
	// clean/changed state (drives the ✓ vs ✎ glyph + color).
	VersionStates map[string]string

	// --- Envelope footer (table + tree-table modes) ---
	//
	// A table shows the rows; these three name the ENVELOPE fields that say how
	// the run went. Without them a run that failed every row renders exactly
	// like a read-only listing.

	// SummaryField names an envelope string field printed as a headline after
	// the table (e.g. "push: 0 succeeded, 3 failed"). Empty ⇒ no headline.
	SummaryField string

	// DetailField names a per-row field carrying failure detail (an error
	// message). Rows whose StateField holds an error state are listed under the
	// summary as "<tree-column>: <detail>". Empty ⇒ no detail block.
	DetailField string

	// FailureCountField names an envelope numeric field holding the count of
	// failed items. When > 0 the CLI exits non-zero, so a scripted run can tell
	// a clean sync from a total failure. Empty ⇒ always exit 0.
	FailureCountField string
}

// UICLIFromType extracts and parses the `ui-cli` extension from a Type's
// Extensions slice. Returns (nil, false) when the extension is absent.
// Unknown keys in the payload are silently ignored — additive grammar
// evolution doesn't break existing parsers.
func UICLIFromType(t *Type) (*UICLI, bool) {
	if t == nil {
		return nil, false
	}
	for i := range t.Extensions {
		ext := &t.Extensions[i]
		if ext.Kind != ExtensionUICLI {
			continue
		}
		return parseUICLIPayload(ext.Payload), true
	}
	return nil, false
}

// parseUICLIPayload reads the typed UICLI shape out of the opaque
// payload map. Tolerant of both `[]any` and `[]string` representations
// (the DSL parser yields the former; struct-tag parsing yields the
// latter via comma-splits).
func parseUICLIPayload(p map[string]any) *UICLI {
	out := &UICLI{}
	if p == nil {
		return out
	}
	if v, ok := p["mode"].(string); ok {
		out.Mode = UICLIMode(v)
	}
	if v, ok := p["primary"].(string); ok {
		out.Primary = v
	}
	if v, ok := p["tree-child-field"].(string); ok {
		out.TreeChildField = v
	}
	if v, ok := p["parent-field"].(string); ok {
		out.ParentField = v
	}
	if v, ok := p["id-field"].(string); ok {
		out.IDField = v
	}
	if v, ok := p["state-field"].(string); ok {
		out.StateField = v
	}
	if v, ok := p["tree-column"].(string); ok {
		out.TreeColumn = v
	}
	if v, ok := p["nodes-field"].(string); ok {
		out.NodesField = v
	}
	if v, ok := p["summary-field"].(string); ok {
		out.SummaryField = v
	}
	if v, ok := p["detail-field"].(string); ok {
		out.DetailField = v
	}
	if v, ok := p["failure-count-field"].(string); ok {
		out.FailureCountField = v
	}
	out.Columns = readStringList(p["columns"])
	out.RichOnlyFields = readStringList(p["rich-only-fields"])
	out.Sort = readStringList(p["sort"])
	out.ColumnKinds = readStringMap(p["column-kinds"])
	out.VersionStates = readStringMap(p["version-states"])
	out.ColumnLabels = readStringMap(p["column-labels"])
	return out
}

// readStringMap parses a payload value into a map[string]string. Accepts the
// DSL/JSON `map[string]any` shape OR the struct-tag string form
// `k:v;k:v` (semicolon-separated pairs, colon key/value). Returns nil otherwise.
func readStringMap(v any) map[string]string {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]string, len(x))
		for k, val := range x {
			if s, ok := val.(string); ok {
				out[k] = s
			}
		}
		return out
	case string:
		if x == "" {
			return nil
		}
		out := map[string]string{}
		for _, pair := range splitCommaSpace(x) { // splits on ,/; and trims
			ci := -1
			for i := 0; i < len(pair); i++ {
				if pair[i] == ':' {
					ci = i
					break
				}
			}
			if ci < 0 {
				continue
			}
			out[trimSpace(pair[:ci])] = trimSpace(pair[ci+1:])
		}
		if len(out) == 0 {
			return nil
		}
		return out
	default:
		return nil
	}
}

// readStringList accepts either a comma-separated string (the form
// produced by struct-tag parsing) or a []any / []string (the form
// produced by DSL YAML parsing). Returns an empty slice on any other
// shape so renderers can always range over the result.
func readStringList(v any) []string {
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		if x == "" {
			return nil
		}
		out := []string{}
		for _, p := range splitCommaSpace(x) {
			if p != "" {
				out = append(out, p)
			}
		}
		return out
	case []string:
		return x
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// splitCommaSpace splits on ',' or ';' and trims whitespace. Tolerant
// of either separator so struct-tag authors can use whichever reads
// better.
func splitCommaSpace(s string) []string {
	out := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ',', ';':
			out = append(out, trimSpace(s[start:i]))
			start = i + 1
		}
	}
	out = append(out, trimSpace(s[start:]))
	return out
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}
