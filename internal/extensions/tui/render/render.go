// SPDX-License-Identifier: Apache-2.0

// Package render is the terminal-display renderer for AutoKTSP CLI
// output. Given a Type AST (the output type of a slot method) plus a
// JSON body, it emits text bytes formatted per the type's `ui-cli`
// extension — table rows, key/value text, indented trees — applying
// TTY / NO_COLOR / width rules.
//
// The renderer is consumed by `internal/kts/protocol/cli/generate.go`
// at AutoKTSP-generated command run time. It is also the reference
// implementation the polyglot framework will mirror in TypeScript /
// Python for non-Go sidecars.
//
// Layering: this package depends on `internal/extensions/schema/api`
// for the typed AST view (UICLIFromType, Type, etc.); it does NOT
// depend on the schema parser (`schema/internal/parser`) — sidecars
// import only the parent transport/nats package plus schema/api, both
// of which are lightweight per the import-discipline
// guards.
//
// Design ADR pointer: modules/display/tui/concepts.md §2
// (the `ui-cli` renderer surface).
package render

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/term"

	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
)

// RenderCtx controls TTY / color / width / mode-override behavior for a
// single Render call. Build it from a Cobra command via RenderCtxFromIO
// or hand-fill the fields for unit tests.
type RenderCtx struct {
	// IsTTY is true when the output writer is a terminal. Renderers gate
	// ANSI escapes on this — piped output stays plain.
	IsTTY bool

	// Color enables ANSI color when IsTTY is also true. Honors NO_COLOR
	// env per https://no-color.org.
	Color bool

	// Width is the terminal column count. Zero = unknown; renderers
	// default to 80 in that case.
	Width int

	// ModeOverride forces a specific output mode regardless of what the
	// ui-cli extension says. The "" zero value defers to the extension.
	// Recognized values: "", "json", "text", "table", "tree". Anything
	// else falls back to JSON.
	ModeOverride string
}

// RenderCtxFromWriter probes a writer for TTY-ness and looks up width
// + color settings. Use from Cobra commands as:
//
//	ctx := render.RenderCtxFromWriter(cmd.OutOrStdout())
//	ctx.ModeOverride = outFlag  // forward --out
func RenderCtxFromWriter(w io.Writer) RenderCtx {
	ctx := RenderCtx{Width: 80}
	if f, ok := w.(*os.File); ok {
		fd := int(f.Fd())
		if term.IsTerminal(fd) {
			ctx.IsTTY = true
			if cols, _, err := term.GetSize(fd); err == nil && cols > 0 {
				ctx.Width = cols
			}
		}
	}
	ctx.Color = ctx.IsTTY && os.Getenv("NO_COLOR") == "" && os.Getenv("KTW_NO_RICH") == ""
	return ctx
}

// Render writes a representation of body (typed by ast) to out. When
// ast is nil or carries no `ui-cli` extension and no ModeOverride is
// set, it falls back to pretty JSON — matching the AutoKTSP today's
// behavior.
//
// Single-array-field envelope: when ast is an object with exactly one
// array field (the common `Output { Items []Item }` shape that AutoKTSP
// methods produce), the renderer descends into the array's AST so an
// `ui-cli` tag on `Items` is honored. This means data-type authors can
// opt in by tagging the array field directly without restructuring
// their output types.
//
// Never panics. On any malformed input it logs nothing and writes raw
// JSON (or the original bytes) so command output stays usable.
func Render(ast *schemaapi.Type, body json.RawMessage, ctx RenderCtx, out io.Writer) error {
	// Promote single-array-field envelopes to the array's AST so a
	// ui-cli tag on the array field drives rendering.
	effectiveAST := unwrapSingleArrayField(ast)

	mode := resolveMode(effectiveAST, ctx)
	if mode == schemaapi.UICLIModeJSON {
		return writeJSON(body, out)
	}

	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		// Body isn't JSON — pass through verbatim.
		_, werr := out.Write(body)
		if !endsWithNewline(body) {
			_, _ = io.WriteString(out, "\n")
		}
		return werr
	}

	cli, _ := resolveUICLI(effectiveAST)
	if cli == nil {
		cli = &schemaapi.UICLI{}
	}

	switch mode {
	case schemaapi.UICLIModeTable:
		return renderTable(effectiveAST, decoded, cli, ctx, out)
	case schemaapi.UICLIModeTree:
		return renderTree(effectiveAST, decoded, cli, ctx, out)
	case schemaapi.UICLIModeTreeTable:
		return renderTreeTable(effectiveAST, decoded, cli, ctx, out)
	case schemaapi.UICLIModeText:
		return renderText(effectiveAST, decoded, cli, ctx, out)
	default:
		return writeJSON(body, out)
	}
}

// FailureCount reports how many items a rendered body declares as failed, read
// from the envelope field named by the ui-cli `failure-count-field`.
//
// ok is false when the output type declares no such field — callers must keep
// "declared zero failures" and "does not report failures" apart, since only the
// former justifies a non-zero exit.
func FailureCount(ast *schemaapi.Type, body json.RawMessage) (n int, ok bool) {
	cli, found := resolveUICLI(ast)
	if !found || cli.FailureCountField == "" {
		return 0, false
	}
	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return 0, false
	}
	env, isObj := decoded.(map[string]any)
	if !isObj {
		return 0, false
	}
	switch v := env[cli.FailureCountField].(type) {
	case float64: // every JSON number decodes as float64
		return int(v), true
	case int:
		return v, true
	default:
		return 0, false
	}
}

// HasUICLI reports whether ast (or its single-array-field descendant
// per the pagination-envelope heuristic) carries a `ui-cli` extension.
// AutoKTSP's cli/generate.go calls this to decide whether to dispatch
// to the renderer or fall back to today's plain-JSON writeOutput path.
func HasUICLI(ast *schemaapi.Type) bool {
	effective := unwrapSingleArrayField(ast)
	if effective == nil {
		return false
	}
	_, ok := resolveUICLI(effective)
	return ok
}

// resolveUICLI returns the ui-cli extension for an AST: the root's own
// extension, or — for an OBJECT root without one — the first field that
// carries a ui-cli extension (with that field name defaulted as NodesField, so
// tree-table/table modes render the designated nested array). Enables method
// outputs with multiple arrays (e.g. SyncOutput{entries, tree, warnings}) to
// tag ONE field as the rich renderable.
func resolveUICLI(ast *schemaapi.Type) (*schemaapi.UICLI, bool) {
	if cli, ok := schemaapi.UICLIFromType(ast); ok {
		return cli, true
	}
	if ast != nil && ast.Kind == schemaapi.KindObject {
		for name, f := range ast.Fields {
			if cli, ok := schemaapi.UICLIFromType(f); ok {
				if cli.NodesField == "" {
					cli.NodesField = name
				}
				return cli, true
			}
		}
	}
	return nil, false
}

// unwrapSingleArrayField returns the array field's AST when the input
// is an object with exactly ONE array-typed field — scalar metadata
// siblings (Total, Truncated, etc.) are tolerated. Returns the
// original AST otherwise.
//
// Covers the common AutoKTSP output shapes:
//
//	struct { Items []Item }                      // envelope
//	struct { Results []R; Total int; Truncated bool }   // paginated envelope
//
// Author opt-in: tag the array field with `ui-cli:"mode=table,..."` to
// drive the table renderer. The renderer composes this with
// `extractSingleArray` at the JSON-body level so author tags on the
// array field flow through the envelope.
func unwrapSingleArrayField(ast *schemaapi.Type) *schemaapi.Type {
	if ast == nil || ast.Kind != schemaapi.KindObject || len(ast.Fields) == 0 {
		return ast
	}
	var arrayField *schemaapi.Type
	count := 0
	for _, f := range ast.Fields {
		if f != nil && f.Kind == schemaapi.KindArray {
			arrayField = f
			count++
		}
	}
	if count == 1 {
		return arrayField
	}
	return ast
}

// resolveMode picks the final render mode from ModeOverride > ui-cli
// extension > AST kind heuristic > json fallback.
func resolveMode(ast *schemaapi.Type, ctx RenderCtx) schemaapi.UICLIMode {
	if ctx.ModeOverride != "" {
		switch schemaapi.UICLIMode(ctx.ModeOverride) {
		case schemaapi.UICLIModeJSON, schemaapi.UICLIModeText,
			schemaapi.UICLIModeTable, schemaapi.UICLIModeTree, schemaapi.UICLIModeTreeTable:
			return schemaapi.UICLIMode(ctx.ModeOverride)
		}
		return schemaapi.UICLIModeJSON
	}
	if ast == nil {
		return schemaapi.UICLIModeJSON
	}
	if cli, ok := resolveUICLI(ast); ok && cli.Mode != "" {
		return cli.Mode
	}
	// Heuristic: arrays default to table; objects default to text; everything else JSON.
	switch ast.Kind {
	case schemaapi.KindArray:
		return schemaapi.UICLIModeTable
	case schemaapi.KindObject:
		return schemaapi.UICLIModeText
	default:
		return schemaapi.UICLIModeJSON
	}
}

// renderText prints `key: value` lines for an object. Falls back to
// JSON for non-object payloads. Honors `primary` (bolded on color
// terminals) and `rich-only-fields` (omitted on non-TTY).
func renderText(ast *schemaapi.Type, decoded any, cli *schemaapi.UICLI, ctx RenderCtx, out io.Writer) error {
	obj, ok := decoded.(map[string]any)
	if !ok {
		// Fall back to JSON when text mode is requested on non-object.
		raw, _ := json.MarshalIndent(decoded, "", "  ")
		return writeJSON(raw, out)
	}

	keys := fieldOrder(ast, obj, cli.Columns)
	keys = filterRichOnly(keys, cli.RichOnlyFields, ctx)

	maxKey := 0
	for _, k := range keys {
		if n := visualWidth(k); n > maxKey {
			maxKey = n
		}
	}

	for _, k := range keys {
		val := stringifyScalar(obj[k])
		label := padRight(k, maxKey)
		if ctx.Color && k == cli.Primary {
			label = ansiBold(padRight(k, maxKey), ctx)
		}
		if _, err := fmt.Fprintf(out, "%s : %s\n", label, val); err != nil {
			return err
		}
	}
	return nil
}

// renderTable prints an aligned table for an array of objects. Falls
// back to text mode when the body is a single object, JSON when not an
// array of records.
func renderTable(ast *schemaapi.Type, decoded any, cli *schemaapi.UICLI, ctx RenderCtx, out io.Writer) error {
	arr, ok := decoded.([]any)
	if !ok {
		// Caller asked for table mode but body is a single object — try
		// to detect a single nested array field (common shape for
		// "list" RPC outputs like `{items: [...]}`).
		if obj, isObj := decoded.(map[string]any); isObj {
			if a := extractSingleArray(obj); a != nil {
				arr = a
			} else {
				// Not an obvious list shape — fall back to text mode.
				return renderText(ast, decoded, cli, ctx, out)
			}
		} else {
			raw, _ := json.MarshalIndent(decoded, "", "  ")
			return writeJSON(raw, out)
		}
	}

	rows := make([]map[string]any, 0, len(arr))
	for _, item := range arr {
		if m, ok := item.(map[string]any); ok {
			rows = append(rows, m)
		}
	}
	if len(rows) == 0 {
		_, _ = io.WriteString(out, "(no rows)\n")
		return nil
	}

	// Resolve item AST for column ordering.
	itemAST := arrayItemAST(ast)
	cols := cli.Columns
	if len(cols) == 0 {
		cols = collectAllColumns(itemAST, rows)
	}
	cols = filterRichOnly(cols, cli.RichOnlyFields, ctx)

	if len(cli.Sort) > 0 {
		sortRows(rows, cli.Sort)
	}

	// Measure widths.
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = visualWidth(c)
	}
	for _, r := range rows {
		for i, c := range cols {
			if n := visualWidth(stringifyScalar(r[c])); n > widths[i] {
				widths[i] = n
			}
		}
	}

	// Header.
	for i, c := range cols {
		if i > 0 {
			_, _ = io.WriteString(out, "  ")
		}
		h := padRight(c, widths[i])
		if ctx.Color {
			h = ansiBold(h, ctx)
		}
		_, _ = io.WriteString(out, h)
	}
	_, _ = io.WriteString(out, "\n")

	// Separator (ASCII for piping safety).
	for i, w := range widths {
		if i > 0 {
			_, _ = io.WriteString(out, "  ")
		}
		_, _ = io.WriteString(out, strings.Repeat("-", w))
	}
	_, _ = io.WriteString(out, "\n")

	// Rows.
	for _, r := range rows {
		for i, c := range cols {
			if i > 0 {
				_, _ = io.WriteString(out, "  ")
			}
			val := stringifyScalar(r[c])
			if i == len(cols)-1 {
				// Last column: no padding, no truncation.
				_, _ = io.WriteString(out, val)
			} else {
				_, _ = io.WriteString(out, padRight(val, widths[i]))
			}
		}
		_, _ = io.WriteString(out, "\n")
	}

	treeCol := cli.TreeColumn
	if treeCol == "" && len(cols) > 0 {
		treeCol = cols[0]
	}
	writeEnvelopeFooter(out, decoded, rows, cli, treeCol, orDefault(cli.StateField, "state"), ctx)
	return nil
}

// renderTree walks recursive objects (TreeChildField points at the
// child list) and prints an indented tree. Falls back to text mode for
// non-recursive shapes.
func renderTree(ast *schemaapi.Type, decoded any, cli *schemaapi.UICLI, ctx RenderCtx, out io.Writer) error {
	if cli.TreeChildField == "" {
		return renderText(ast, decoded, cli, ctx, out)
	}
	switch v := decoded.(type) {
	case map[string]any:
		writeTreeNode(out, v, cli, "", true)
	case []any:
		for i, item := range v {
			if m, ok := item.(map[string]any); ok {
				writeTreeNode(out, m, cli, "", i == len(v)-1)
			}
		}
	default:
		raw, _ := json.MarshalIndent(decoded, "", "  ")
		return writeJSON(raw, out)
	}
	return nil
}

func writeTreeNode(out io.Writer, node map[string]any, cli *schemaapi.UICLI, prefix string, isLast bool) {
	branch := "├── "
	childPrefix := prefix + "│   "
	if isLast {
		branch = "└── "
		childPrefix = prefix + "    "
	}
	label := stringifyScalar(node[cli.Primary])
	if label == "" {
		// Try id-ish fields as fallback.
		for _, k := range []string{"name", "id", "title"} {
			if s := stringifyScalar(node[k]); s != "" {
				label = s
				break
			}
		}
	}
	if label == "" {
		label = "(node)"
	}
	fmt.Fprintf(out, "%s%s%s\n", prefix, branch, label)

	children, _ := node[cli.TreeChildField].([]any)
	for i, c := range children {
		if m, ok := c.(map[string]any); ok {
			writeTreeNode(out, m, cli, childPrefix, i == len(children)-1)
		}
	}
}

// writeJSON pretty-prints body to out with a trailing newline.
func writeJSON(body json.RawMessage, out io.Writer) error {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		// Not JSON — write raw.
		if _, werr := out.Write(body); werr != nil {
			return werr
		}
		if !endsWithNewline(body) {
			_, _ = io.WriteString(out, "\n")
		}
		return nil
	}
	pretty, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if _, err := out.Write(pretty); err != nil {
		return err
	}
	_, _ = io.WriteString(out, "\n")
	return nil
}

// -----------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------

func fieldOrder(ast *schemaapi.Type, obj map[string]any, hint []string) []string {
	if len(hint) > 0 {
		return hint
	}
	if ast != nil && ast.Kind == schemaapi.KindObject && len(ast.Fields) > 0 {
		out := make([]string, 0, len(ast.Fields))
		for name := range ast.Fields {
			if _, has := obj[name]; has {
				out = append(out, name)
			}
		}
		sort.Strings(out)
		return out
	}
	out := make([]string, 0, len(obj))
	for k := range obj {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func collectAllColumns(itemAST *schemaapi.Type, rows []map[string]any) []string {
	if itemAST != nil && itemAST.Kind == schemaapi.KindObject && len(itemAST.Fields) > 0 {
		out := make([]string, 0, len(itemAST.Fields))
		for name := range itemAST.Fields {
			out = append(out, name)
		}
		sort.Strings(out)
		return out
	}
	seen := map[string]struct{}{}
	out := []string{}
	for _, r := range rows {
		for k := range r {
			if _, ok := seen[k]; !ok {
				seen[k] = struct{}{}
				out = append(out, k)
			}
		}
	}
	sort.Strings(out)
	return out
}

func arrayItemAST(ast *schemaapi.Type) *schemaapi.Type {
	if ast == nil || ast.Kind != schemaapi.KindArray || ast.Items == nil {
		return nil
	}
	if ast.Items.Inline != nil {
		return ast.Items.Inline
	}
	return nil
}

func filterRichOnly(cols []string, richOnly []string, ctx RenderCtx) []string {
	if len(richOnly) == 0 || (ctx.IsTTY && ctx.Color) {
		return cols
	}
	drop := map[string]struct{}{}
	for _, f := range richOnly {
		drop[f] = struct{}{}
	}
	out := cols[:0]
	for _, c := range cols {
		if _, omit := drop[c]; !omit {
			out = append(out, c)
		}
	}
	return out
}

func extractSingleArray(obj map[string]any) []any {
	var found []any
	count := 0
	for _, v := range obj {
		if a, ok := v.([]any); ok {
			found = a
			count++
		}
	}
	if count == 1 {
		return found
	}
	return nil
}

func sortRows(rows []map[string]any, keys []string) {
	sort.SliceStable(rows, func(i, j int) bool {
		for _, k := range keys {
			a := stringifyScalar(rows[i][k])
			b := stringifyScalar(rows[j][k])
			if a != b {
				return a < b
			}
		}
		return false
	})
}

func stringifyScalar(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int, int8, int16, int32, int64:
		return fmt.Sprintf("%d", x)
	case []any:
		parts := make([]string, 0, len(x))
		for _, item := range x {
			parts = append(parts, stringifyScalar(item))
		}
		return strings.Join(parts, ",")
	case map[string]any:
		raw, err := json.Marshal(x)
		if err != nil {
			return "{...}"
		}
		return string(raw)
	default:
		return fmt.Sprintf("%v", x)
	}
}

func padRight(s string, w int) string {
	n := visualWidth(s)
	if n >= w {
		return s
	}
	return s + strings.Repeat(" ", w-n)
}

// visualWidth approximates the printed width of s on a terminal. Tabs
// and ANSI escapes are not expected here (caller-supplied); a byte
// count is sufficient for ASCII-heavy strings the CLI emits.
func visualWidth(s string) int {
	// Strip any embedded ANSI escapes defensively — they shouldn't occur
	// at this point but len() would otherwise inflate widths.
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b { // ESC — skip ANSI sequence
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		n++
	}
	return n
}

func endsWithNewline(b []byte) bool {
	return len(b) > 0 && b[len(b)-1] == '\n'
}

const (
	ansiResetSeq = "\x1b[0m"
	ansiBoldSeq  = "\x1b[1m"
)

func ansiBold(s string, ctx RenderCtx) string {
	if !ctx.Color {
		return s
	}
	return ansiBoldSeq + s + ansiResetSeq
}
