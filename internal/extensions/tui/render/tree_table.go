// SPDX-License-Identifier: Apache-2.0

package render

import (
	"io"
	"strings"
	"unicode/utf8"

	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
)

// renderTreeTable renders a FLAT node array (linked by cli.ParentField) as an
// indented tree WITH aligned columns — the terminal twin of the GUI tree-table.
// Column `kinds` drive rich cells: version (✓/✎ + value), direction (▶▶/◀◀/═),
// state (colored label). The tree indent rides cli.TreeColumn (default col 0).
func renderTreeTable(ast *schemaapi.Type, decoded any, cli *schemaapi.UICLI, ctx RenderCtx, out io.Writer) error {
	rows := asRowArray(decoded, cli.NodesField)
	if rows == nil {
		// Not a node array — fall back to the flat table renderer.
		return renderTable(ast, decoded, cli, ctx, out)
	}
	if len(rows) == 0 {
		_, _ = io.WriteString(out, "(no rows)\n")
		return nil
	}

	idField := orDefault(cli.IDField, "id")
	parentField := orDefault(cli.ParentField, "parent_id")
	stateField := orDefault(cli.StateField, "state")

	cols := cli.Columns
	if len(cols) == 0 {
		cols = collectAllColumns(arrayItemAST(ast), rows)
	}
	cols = filterRichOnly(cols, cli.RichOnlyFields, ctx)
	treeCol := cli.TreeColumn
	if treeCol == "" && len(cols) > 0 {
		treeCol = cols[0]
	}

	flat := flattenNodes(rows, idField, parentField)

	// Compute plain + colored cell text per (row, column).
	type cell struct{ plain, colored string }
	grid := make([][]cell, len(flat))
	for ri, fr := range flat {
		line := make([]cell, len(cols))
		for ci, c := range cols {
			p, col := renderTreeCell(c, cli, fr, treeCol, stateField, ctx)
			line[ci] = cell{p, col}
		}
		grid[ri] = line
	}

	// Column widths measured on the PLAIN text (color/ANSI excluded).
	widths := make([]int, len(cols))
	for ci, c := range cols {
		widths[ci] = runeWidth(headerLabel(cli, c))
	}
	for _, line := range grid {
		for ci := range cols {
			if w := runeWidth(line[ci].plain); w > widths[ci] {
				widths[ci] = w
			}
		}
	}

	// Header.
	for ci, c := range cols {
		if ci > 0 {
			_, _ = io.WriteString(out, "  ")
		}
		h := padRightRunes(headerLabel(cli, c), widths[ci])
		if ctx.Color {
			h = ansiBold(h, ctx)
		}
		_, _ = io.WriteString(out, h)
	}
	_, _ = io.WriteString(out, "\n")

	// Rows.
	for _, line := range grid {
		for ci := range cols {
			if ci > 0 {
				_, _ = io.WriteString(out, "  ")
			}
			text := line[ci].plain
			if ctx.Color {
				text = line[ci].colored
			}
			if ci == len(cols)-1 {
				_, _ = io.WriteString(out, text) // last column: no trailing pad
			} else {
				_, _ = io.WriteString(out, text)
				_, _ = io.WriteString(out, strings.Repeat(" ", widths[ci]-runeWidth(line[ci].plain)))
			}
		}
		_, _ = io.WriteString(out, "\n")
	}

	writeEnvelopeFooter(out, decoded, rows, cli, treeCol, stateField, ctx)
	return nil
}

// writeEnvelopeFooter prints the run outcome under a table: the `summary-field`
// headline, then one line per error-state row carrying `detail-field`.
//
// Without it a table renders identically whether every row succeeded or every
// row failed — the rows describe WHAT was planned, only the envelope says how it
// went. No-op when the author declared neither field.
func writeEnvelopeFooter(out io.Writer, decoded any, rows []map[string]any, cli *schemaapi.UICLI, treeCol, stateField string, ctx RenderCtx) {
	if cli.SummaryField == "" && cli.DetailField == "" {
		return
	}
	env, _ := decoded.(map[string]any)

	summary := ""
	if cli.SummaryField != "" && env != nil {
		summary = stringifyScalar(env[cli.SummaryField])
	}

	// Failure lines: rows whose state column reads as an error.
	var failures []string
	if cli.DetailField != "" {
		for _, r := range rows {
			if !errorStates[strings.ToLower(stringifyScalar(r[stateField]))] {
				continue
			}
			label := stringifyScalar(r[treeCol])
			detail := stringifyScalar(r[cli.DetailField])
			switch {
			case label != "" && detail != "":
				failures = append(failures, label+": "+detail)
			case detail != "":
				failures = append(failures, detail)
			case label != "":
				failures = append(failures, label)
			}
		}
	}

	if summary == "" && len(failures) == 0 {
		return
	}
	_, _ = io.WriteString(out, "\n")
	if summary != "" {
		color := ansiGreen
		if len(failures) > 0 {
			color = ansiRed
		}
		_, _ = io.WriteString(out, ansiWrap(color, summary, ctx)+"\n")
	}
	for _, f := range failures {
		_, _ = io.WriteString(out, "  "+ansiWrap(ansiRed, "✗", ctx)+" "+f+"\n")
	}
}

// errorStates are the state values that mark a row as FAILED (as opposed to
// merely destructive, e.g. delete-remote, which stateAnsi also colors red).
var errorStates = map[string]bool{
	"failed": true, "error": true, "conflict": true,
}

// flatRow is one node in display order with its depth.
type flatRow struct {
	row   map[string]any
	depth int
}

// flattenNodes derives display order (depth-first) from the flat node list.
// Roots = nodes whose parent is empty or not present.
func flattenNodes(rows []map[string]any, idField, parentField string) []flatRow {
	byID := make(map[string]map[string]any, len(rows))
	for _, r := range rows {
		if id := stringifyScalar(r[idField]); id != "" {
			byID[id] = r
		}
	}
	childrenOf := make(map[string][]map[string]any)
	for _, r := range rows {
		parent := stringifyScalar(r[parentField])
		key := ""
		if parent != "" {
			if _, ok := byID[parent]; ok {
				key = parent
			}
		}
		childrenOf[key] = append(childrenOf[key], r)
	}
	out := make([]flatRow, 0, len(rows))
	// onPath breaks parent cycles (a → b → a): a node is descended into at most
	// once per path, so a cycle terminates instead of recursing forever.
	onPath := make(map[string]bool)
	var walk func(parentKey string, depth int)
	walk = func(parentKey string, depth int) {
		for _, r := range childrenOf[parentKey] {
			out = append(out, flatRow{row: r, depth: depth})
			id := stringifyScalar(r[idField])
			if id == "" || onPath[id] {
				continue
			}
			onPath[id] = true
			walk(id, depth+1)
			delete(onPath, id)
		}
	}
	walk("", 0)
	// Any orphans not reachable from a root (defensive) — append flat.
	if len(out) < len(rows) {
		seen := make(map[string]bool, len(out))
		for _, fr := range out {
			seen[stringifyScalar(fr.row[idField])] = true
		}
		for _, r := range rows {
			if !seen[stringifyScalar(r[idField])] {
				out = append(out, flatRow{row: r, depth: 0})
			}
		}
	}
	return out
}

// renderTreeCell returns (plain, colored) text for one cell, by column kind.
func renderTreeCell(col string, cli *schemaapi.UICLI, fr flatRow, treeCol, stateField string, ctx RenderCtx) (string, string) {
	row := fr.row
	kind := cli.ColumnKinds[col]
	if kind == "" && col == stateField {
		kind = "state"
	}
	val := stringifyScalar(row[col])

	switch kind {
	case "version":
		st := stringifyScalar(row[cli.VersionStates[col]])
		clean := cleanStates[st]
		glyph := "✎"
		color := ansiYellow
		if clean {
			glyph = "✓"
			color = ansiGreen
		}
		plain := glyph
		colored := ansiWrap(color, glyph, ctx)
		if val != "" {
			plain += " " + val
			colored += " " + ansiWrap(ansiDimSeq, val, ctx)
		}
		return plain, colored
	case "direction":
		return directionCell(val, ctx)
	case "state":
		if val == "" {
			return "", ""
		}
		return val, ansiWrap(stateAnsi(val), val, ctx)
	default:
		if col == treeCol {
			indent := strings.Repeat("  ", fr.depth)
			return indent + val, indent + val
		}
		return val, val
	}
}

func directionCell(val string, ctx RenderCtx) (string, string) {
	switch strings.ToLower(val) {
	case "upload", "push", "create":
		return "▶▶", ansiWrap(ansiGreen, "▶▶", ctx)
	case "download", "pull":
		return "◀◀", ansiWrap(ansiBlue, "◀◀", ctx)
	default: // skip / clean / equal
		return "═", ansiWrap(ansiGray, "═", ctx)
	}
}

// cleanStates are the states rendered with a ✓ (green) rather than a ✎ (yellow).
var cleanStates = map[string]bool{
	"": true, "clean": true, "up-to-date": true, "uptodate": true,
	"done": true, "ok": true, "synced": true,
}

// stateAnsi maps a status value to an ANSI color sequence.
func stateAnsi(state string) string {
	switch strings.ToLower(state) {
	case "done", "clean", "up to date", "up-to-date", "synced", "ok", "skip",
		"uploaded", "created", "downloaded", "moved", "renamed", "deleted":
		return ansiGreen
	case "failed", "conflict", "error", "delete-local", "delete-remote":
		return ansiRed
	case "processing", "download", "pull":
		return ansiBlue
	case "upload", "push", "create", "local updated", "updated", "new":
		return ansiYellow
	default:
		return ansiGray
	}
}

// asRowArray coerces decoded into a []map[string]any (the node rows). Accepts a
// direct array, a named `nodesField` array inside an object, or a
// single-array-field object envelope.
func asRowArray(decoded any, nodesField string) []map[string]any {
	arr, ok := decoded.([]any)
	if !ok {
		if obj, isObj := decoded.(map[string]any); isObj {
			if nodesField != "" {
				if a, ok := obj[nodesField].([]any); ok {
					arr = a
				} else {
					return nil
				}
			} else if a := extractSingleArray(obj); a != nil {
				arr = a
			} else {
				return nil
			}
		} else {
			return nil
		}
	}
	rows := make([]map[string]any, 0, len(arr))
	for _, item := range arr {
		if m, ok := item.(map[string]any); ok {
			rows = append(rows, m)
		}
	}
	return rows
}

func headerLabel(cli *schemaapi.UICLI, col string) string {
	if cli.ColumnLabels != nil {
		if l, ok := cli.ColumnLabels[col]; ok {
			return l
		}
	}
	return col
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func runeWidth(s string) int {
	// Strip ANSI defensively, then count runes (approx 1 col each for the
	// glyphs the tree-table emits).
	n := 0
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			if i < len(s) {
				i++
			}
			continue
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		if size == 0 {
			size = 1
		}
		i += size
		n++
	}
	return n
}

func padRightRunes(s string, w int) string {
	n := runeWidth(s)
	if n >= w {
		return s
	}
	return s + strings.Repeat(" ", w-n)
}

// ANSI color sequences (gated on ctx.Color via ansiWrap).
const (
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiBlue   = "\x1b[34m"
	ansiRed    = "\x1b[31m"
	ansiGray   = "\x1b[90m"
	ansiDimSeq = "\x1b[2m"
)

func ansiWrap(code, s string, ctx RenderCtx) string {
	if !ctx.Color {
		return s
	}
	return code + s + ansiResetSeq
}
