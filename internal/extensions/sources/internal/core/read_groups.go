// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	schemaast "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// readGroupKey is where the rows of the n-th kind group of « Contenu lu »
// are bound in the detail's data ($.detail.read_<n>).
const readGroupKey = "read_%d"

// readGroupColumnsMax bounds the columns drawn from a data-type that
// declares no table of its own.
const readGroupColumnsMax = 6

// readGroup is one kind of item a source brought, drawn as its own table
// : its kind, its label in the plural, its columns - the
// fields of the kind's data-type - and its rows.
type readGroup struct {
	kind    string
	label   string
	lead    []api.ReadColumn
	columns []api.ReadColumn
	rows    []any
}

// groupRead lays « Contenu lu » out by kind: the items of
// the unit the source picks (an application) and any kind with no
// data-type stay in the first table, under « <Unit>s (n) » with the unit's
// slots as columns (its code); every other kind its connector types by a
// data-type has a table of its own, « <Kind>s (n) », its columns the
// data-type's fields led by the kind's id column its source chose
// , each row opening its item. The head counts them
// « 3 applications, 6 interfaces, lues le … ». A connector bringing one
// kind only keeps the one table. Nothing here names a connector or a kind.
func (s *Service) groupRead(ctx context.Context, detail map[string]any, conn api.ConnectorDecl, idColumns map[string]string, root string) []readGroup {
	items, _ := detail["items"].([]any)
	unit := ""
	if conn.Picker != nil {
		unit = strings.TrimSpace(conn.Picker.Unit)
	}
	byKind := map[string][]any{}
	var first []any
	for _, it := range items {
		row := it.(map[string]any)
		kind, _ := row["kind_key"].(string)
		if kind != "" && kind != unit && strings.TrimSpace(conn.Items[kind]) != "" {
			byKind[kind] = append(byKind[kind], row)
			continue
		}
		first = append(first, row)
	}
	if len(byKind) == 0 {
		return nil
	}
	lead := s.unitLead(ctx, conn, idColumns)
	kinds := make([]string, 0, len(byKind))
	for k := range byKind {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)

	// The unit's table: its rows keep the shared columns, its slots added
	// from each item's data.
	var slots []api.ReadColumn
	for _, sl := range conn.Kinds[unit].Slots {
		if strings.TrimSpace(sl.Attr) != "" {
			slots = append(slots, api.ReadColumn{Attr: sl.Attr, Label: sl.Label})
		}
	}
	for _, it := range first {
		fillColumns(it.(map[string]any), append(append([]api.ReadColumn{}, lead...), slots...), root)
	}
	unitLabel := kindLabel(conn, unit)
	if unit == "" {
		unitLabel = "élément"
	}
	groups := []readGroup{{kind: unit, label: unitPlural(unitLabel), lead: lead, columns: slots, rows: first}}
	for _, k := range kinds {
		cols := s.dataTypeColumns(ctx, conn.Items[k], idColumns[k])
		for _, it := range byKind[k] {
			fillColumns(it.(map[string]any), cols, root)
		}
		groups = append(groups, readGroup{kind: k, label: unitPlural(kindLabel(conn, k)), columns: cols, rows: byKind[k]})
	}

	var counts []string
	for _, g := range groups {
		if len(g.rows) > 0 {
			counts = append(counts, fmt.Sprintf("%d %s", len(g.rows), strings.ToLower(g.label)))
		}
	}
	head := strings.Join(counts, ", ")
	if at := lastRead(items); at != "" {
		head += ", lues le " + at
	}
	detail["read_head"] = head
	for i, g := range groups {
		detail[fmt.Sprintf(readGroupKey, i)] = g.rows
	}
	return groups
}

// kindLabel is a kind as its connector names it, capitalised; the kind
// itself when it names none.
func kindLabel(conn api.ConnectorDecl, kind string) string {
	label := strings.TrimSpace(conn.Kinds[kind].Label)
	if label == "" {
		label = kind
	}
	r := []rune(label)
	if len(r) == 0 {
		return label
	}
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

// lastRead is the latest read date of the items, "" when none was read.
func lastRead(items []any) string {
	last := ""
	for _, it := range items {
		if at, _ := it.(map[string]any)["read_at"].(string); at > last {
			last = at
		}
	}
	return last
}

// fillColumns sets each column's cell on a row from the item's data file -
// a dotted attribute reading into a record (lifecycle.asString).
func fillColumns(row map[string]any, cols []api.ReadColumn, root string) {
	if len(cols) == 0 {
		return
	}
	file, _ := row["data_file"].(string)
	raw, ok := projectFile(root, file)
	if !ok {
		return
	}
	var data map[string]any
	if json.Unmarshal(raw, &data) != nil {
		return
	}
	for _, c := range cols {
		var v any = data
		for _, part := range strings.Split(c.Attr, ".") {
			m, ok := v.(map[string]any)
			if !ok {
				v = nil
				break
			}
			v = m[part]
		}
		if t := cellText(v); t != "" {
			row[columnID(c.Attr)] = html.UnescapeString(t)
		}
	}
}

// columnID is a column's id in a group's table: its attribute kept clear of
// the table's own columns, a dotted path made one word.
func columnID(attr string) string {
	return readColumnPrefix + strings.ReplaceAll(attr, ".", "_")
}

// kindType is a kind's data-type root, its default table (its ui-table
// extension) with it; nil when the data-type cannot be read.
func (s *Service) kindType(ctx context.Context, uri string) *schemaast.Type {
	schemas, err := application.UseService[schemaapi.SchemaService](ctx)
	if err != nil {
		return nil
	}
	got, err := schemas.GetDataTypeWithExtensions(ctx, schemaapi.GetDataTypeWithExtensionsInput{URI: uri,
		Extensions: []schemaapi.ExtensionRef{{Type: "ui-table", Name: "default"}}})
	if err != nil || got.Schema == nil {
		return nil
	}
	return got.Schema.Types[got.Schema.Root]
}

// fieldLabel is a field of a data-type root as it labels it, its name when
// it labels none.
func fieldLabel(rootType *schemaast.Type, name string) string {
	if f := rootType.Fields[name]; f != nil && f.Label != "" {
		return f.Label
	}
	return name
}

// technicalIDs are the fields of a data-type root a person never reads as
// an id: its id field and `id`.
func technicalIDs(rootType *schemaast.Type) map[string]bool {
	out := map[string]bool{"id": true}
	for _, n := range rootType.IDField {
		out[n] = true
	}
	return out
}

// tableColumns are the columns of a data-type's own default table (its
// ui-table extension), none when it declares none.
func tableColumns(rootType *schemaast.Type) []api.ReadColumn {
	for _, ext := range rootType.Extensions {
		if ext.Kind != "ui-table" {
			continue
		}
		if name, _ := ext.Payload["name"].(string); name != "" && name != "default" {
			continue
		}
		cols, _ := ext.Payload["columns"].([]any)
		var out []api.ReadColumn
		for _, c := range cols {
			m, _ := c.(map[string]any)
			id, _ := m["id"].(string)
			if id == "" {
				continue
			}
			l, _ := m["label"].(string)
			if l == "" {
				l = fieldLabel(rootType, id)
			}
			out = append(out, api.ReadColumn{Attr: id, Label: l})
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

// leadAttr is the id column a kind's table leads with: the
// attribute its source chose; with no choice or IDColumnUID the entity's
// uid - the data-type's id field, else its `id` -; none for IDColumnNone.
func leadAttr(rootType *schemaast.Type, idColumn string) string {
	switch chosen := strings.TrimSpace(idColumn); chosen {
	case api.IDColumnNone:
		return ""
	case "", api.IDColumnUID:
	default:
		return chosen
	}
	for _, n := range rootType.IDField {
		if n = strings.TrimSpace(n); n != "" {
			return n
		}
	}
	if rootType.Fields["id"] != nil {
		return "id"
	}
	return ""
}

// unitLead is the id column the table of the unit a source picks leads
// with, read from the unit's data-type as any kind's
// none when the connector types no unit or its data-type cannot be read.
func (s *Service) unitLead(ctx context.Context, conn api.ConnectorDecl, idColumns map[string]string) []api.ReadColumn {
	if conn.Picker == nil {
		return nil
	}
	unit := strings.TrimSpace(conn.Picker.Unit)
	uri := strings.TrimSpace(conn.Items[unit])
	if unit == "" || uri == "" {
		return nil
	}
	rootType := s.kindType(ctx, uri)
	if rootType == nil {
		return nil
	}
	if attr := leadAttr(rootType, idColumns[unit]); attr != "" {
		return []api.ReadColumn{{Attr: attr, Label: fieldLabel(rootType, attr)}}
	}
	return nil
}

// dataTypeColumns are the columns of a kind's table, read from its
// data-type: the columns of its own default table when it declares one,
// else its plain fields - its title, then the others by name, at most
// readGroupColumnsMax, a description left out. The kind's id column
// (leadAttr: the attribute its source chose, by default the entity's uid)
// leads them; IDColumnNone draws none, and a technical id (the data-type's
// id field, `id`) is no other column. None when the data-type cannot be
// read: the table draws its state alone.
func (s *Service) dataTypeColumns(ctx context.Context, uri, idColumn string) []api.ReadColumn {
	rootType := s.kindType(ctx, uri)
	if rootType == nil {
		return nil
	}
	chosen := leadAttr(rootType, idColumn)
	technical := technicalIDs(rootType)
	keep := func(attr string) bool { return attr != chosen && !technical[attr] }
	var out []api.ReadColumn
	if chosen != "" {
		out = append(out, api.ReadColumn{Attr: chosen, Label: fieldLabel(rootType, chosen)})
	}
	if cols := tableColumns(rootType); len(cols) > 0 {
		for _, c := range cols {
			if keep(c.Attr) {
				out = append(out, c)
			}
		}
		return out
	}
	var lead, rest []string
	seen := map[string]bool{}
	for _, n := range append(append([]string{}, rootType.TitleField...), "displayName", "name", "title") {
		if f := rootType.Fields[n]; f != nil && isPlain(f) && keep(n) {
			lead, seen[n] = append(lead, n), true
			break
		}
	}
	for n, f := range rootType.Fields {
		if !seen[n] && n != "description" && isPlain(f) && keep(n) {
			rest = append(rest, n)
		}
	}
	sort.Strings(rest)
	for _, n := range append(lead, rest...) {
		if len(out) == readGroupColumnsMax {
			break
		}
		out = append(out, api.ReadColumn{Attr: n, Label: fieldLabel(rootType, n)})
	}
	return out
}

// idColumnOptions are the choices of a kind's « Colonne identifiant » row
// : by default (the entity's uid, IDColumnUID), none, then
// the columns of the data-type's own table and its plain fields, each once,
// by label - an attribute named as one of the two choices left out.
func (s *Service) idColumnOptions(ctx context.Context, uri string) []any {
	out := []any{
		map[string]any{"value": api.IDColumnUID, "label": "Par défaut (identifiant)"},
		map[string]any{"value": api.IDColumnNone, "label": "Aucune"},
	}
	rootType := s.kindType(ctx, uri)
	if rootType == nil {
		return out
	}
	var cols []api.ReadColumn
	seen := map[string]bool{api.IDColumnUID: true, api.IDColumnNone: true}
	for _, c := range tableColumns(rootType) {
		if !seen[c.Attr] {
			seen[c.Attr] = true
			cols = append(cols, c)
		}
	}
	for n, f := range rootType.Fields {
		if !seen[n] && n != "description" && isPlain(f) {
			seen[n] = true
			cols = append(cols, api.ReadColumn{Attr: n, Label: fieldLabel(rootType, n)})
		}
	}
	sort.Slice(cols, func(i, j int) bool {
		if cols[i].Label != cols[j].Label {
			return cols[i].Label < cols[j].Label
		}
		return cols[i].Attr < cols[j].Attr
	})
	for _, c := range cols {
		out = append(out, map[string]any{"value": c.Attr, "label": c.Label})
	}
	return out
}

// typedKinds are the kinds a connector types by a data-type, the unit its
// picker picks included - each drawn as a table of its own in « Contenu
// lu », each with its « Colonne identifiant » -, by name.
func typedKinds(conn api.ConnectorDecl) []string {
	var out []string
	for k, uri := range conn.Items {
		if k != "" && strings.TrimSpace(uri) != "" {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// isPlain is a field a table cell shows as it is: a text, a number, a yes
// or no.
func isPlain(f *schemaast.Type) bool {
	return f.Kind == schemaast.KindString || f.Kind == schemaast.KindNumber || f.Kind == schemaast.KindBoolean
}

// readGroupsTree draws « Contenu lu » grouped by kind: the source's state,
// meta line and the head counting each kind, what to do when it failed or
// changed, then a table per kind under « <Kind>s (n) » - the first the
// unit's, with the shared columns and its slots; each other kind's with its
// data-type's columns and its state - each row opening the item read.
func readGroupsTree(detail map[string]any, groups []readGroup) map[string]any {
	children := []any{map[string]any{"kind": "hbox", "gap": "sm", "children": []any{
		map[string]any{"kind": "badge", "text": "", "dataField": "$.detail.state", "color": detail["state_color"]},
		map[string]any{"kind": "text", "content": "", "dataField": "$.detail.meta", "size": "xs", "color": "dimmed"},
		map[string]any{"kind": "text", "id": "source-read-head", "content": "", "dataField": "$.detail.read_head", "size": "xs", "color": "dimmed"},
	}}}
	if _, ok := detail["note"]; ok {
		tone := "yellow"
		if detail["state_color"] == "red" {
			tone = "red"
		}
		children = append(children, map[string]any{"kind": "box", "id": "source-detail-note", "callout": tone, "children": []any{
			map[string]any{"kind": "text", "content": "", "dataField": "$.detail.note", "size": "sm"},
		}})
	}
	open := map[string]any{"label": "Ouvrir", "icon": "eye", "when": "doc", "navigate": map[string]any{"object": "${item.doc}"}}
	for i, g := range groups {
		if len(g.rows) == 0 {
			continue
		}
		var schema map[string]any
		id := "source-read-" + g.kind
		if i == 0 {
			cols, _ := detail["read_columns"].([]api.ReadColumn)
			schema = readSchemaLed(g.lead, append(append([]api.ReadColumn{}, cols...), g.columns...))
			id = "source-detail-items"
		} else {
			schema = kindSchema(g.columns)
		}
		children = append(children,
			map[string]any{"kind": "text", "id": id + "-title", "content": fmt.Sprintf("%s (%d)", g.label, len(g.rows)), "size": "sm", "fw": "bold"},
			render.Table(schema, nil).With("id", id).With("dataField", fmt.Sprintf("$.detail."+readGroupKey, i)).With("open", open))
	}
	return map[string]any{"kind": "section", "id": "source-detail", "size": "sm", "title": "Ce que la source a apporté",
		"icon": "list-details", "color": "teal", "children": children}
}

// kindRow is a row of a kind's table: its open mark, its state and why;
// the data-type's columns are laid before (kindSchema).
type kindRow struct {
	Open  string `json:"open,omitempty"`
	State string `json:"state_label"`
	Why   string `json:"why,omitempty"`
}

// kindSchema is a kind's table: the data-type's columns, the first with the
// open mark before it, then the item's state - each sortable, the first
// searched.
func kindSchema(cols []api.ReadColumn) map[string]any {
	schema := render.InlineSchema[kindRow]("ReadKindItem", map[string]string{"state_label": "État"})
	types, _ := schema["types"].(map[string]any)
	root, _ := types["ReadKindItem"].(map[string]any)
	fields, _ := root["fields"].(map[string]any)
	var columns []any
	var filters []any
	for i, c := range cols {
		label := c.Label
		if label == "" {
			label = c.Attr
		}
		id := columnID(c.Attr)
		if fields != nil {
			fields[id] = map[string]any{"type": "string", "label": label, "optional": true}
		}
		col := map[string]any{"id": id, "name": id, "label": label, "sortable": true}
		if i == 0 {
			col["prepend"] = []any{"open"}
			filters = append(filters, id)
		}
		columns = append(columns, col)
	}
	state := map[string]any{"id": "state_label", "name": "state_label", "label": "État", "sortable": true, "append": []any{"why"}}
	if len(columns) == 0 {
		state["prepend"] = []any{"open"}
	}
	columns = append(columns, state)
	if root != nil {
		ext := map[string]any{"type": "ui-table", "name": "default", "columns": columns, "sortable": true, "pageSize": 50}
		if len(filters) > 0 {
			ext["filters"] = filters
		}
		root["extensions"] = []any{ext}
	}
	return schema
}
