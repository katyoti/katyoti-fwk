// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"sort"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// The words an item's state takes in a source's detail, as
// mock-up v2 says them, each with its colour.
var itemStateWords = map[string]struct{ label, color string }{
	api.ItemRead:    {"synchronisé", "green"},
	api.ItemWaiting: {"à lire", "blue"},
	api.ItemRemoved: {"à retirer", "yellow"},
	api.ItemFailed:  {"en échec", "red"},
}

// detailStates are the item states in the order the detail counts them.
var detailStates = []string{api.ItemRead, api.ItemWaiting, api.ItemRemoved, api.ItemFailed}

// SourceDetail is one information source's detail as a page region
// - what an opened source card shows, the same for every
// source whatever its connector: its state in a word and its meta line (the
// card's own), what to do when it failed or changed, then
// each item it brought or keeps with its state - synchronisé, à lire (kept,
// not read yet), à retirer, en échec with why -, an item read opening its
// source-document.
func (s *Service) SourceDetail(ctx context.Context, in api.SourceDetailInput) (api.ConnectorPanelOutput, error) {
	if strings.TrimSpace(in.ID) == "" {
		return api.ConnectorPanelOutput{}, fmt.Errorf("%w: sources.SourceDetail: id required", cwmapi.ErrInvalidInput)
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.ConnectorPanelOutput{}, fmt.Errorf("sources: resolve CWMService: %w", err)
	}
	detail, err := s.sourceDetail(ctx, cwm, in.ID)
	if err != nil {
		return api.ConnectorPanelOutput{}, err
	}
	return api.ConnectorPanelOutput{DSLTree: sourceDetailTree(detail, true), Data: map[string]any{"detail": detail}, Title: fmt.Sprint(detail["label"])}, nil
}

// sourceDetail is the detail's data: the source's state, meta line, note,
// counts and items.
func (s *Service) sourceDetail(ctx context.Context, cwm cwmapi.CWMService, id string) (map[string]any, error) {
	module, err := cwmapi.ModuleOfObjectID(id)
	if err != nil {
		return nil, err
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
	if err != nil {
		return nil, err
	}
	if got.Object.Type != api.InformationSourceType {
		return nil, fmt.Errorf("%w: %q is a %s, not an %s", cwmapi.ErrInvalidInput, id, got.Object.Type, api.InformationSourceType)
	}
	src := summaryOf(got.Object, "child")
	var u api.ConnectorUse
	card(&u, src)
	label := src.Name
	conn, cerr := s.connector(ctx, src.Connector)
	if cerr == nil && conn.Label != "" {
		label = conn.Label
	}

	// What it keeps: each item picked and each held, by its key, with its
	// state once applied - a picked item not applied yet, or held, is « à
	// lire ».
	f, _, err := sourceFields(ctx, cwm, id)
	if err != nil {
		return nil, err
	}
	items := []any{}
	keyed := map[string]map[string]any{}
	add := func(key, title, state, why string) map[string]any {
		// A title as its source wrote it, entities decoded (« l'existant »,
		// never « l&#39;existant »).
		title = html.UnescapeString(title)
		if title == "" {
			title = key
		}
		if _, ok := itemStateWords[state]; !ok {
			state = api.ItemWaiting
		}
		w := itemStateWords[state]
		row := map[string]any{"key": key, "title": title, "state": state, "state_label": w.label, "color": w.color}
		if why != "" {
			row["why"] = why
		}
		items = append(items, row)
		return row
	}
	for _, it := range api.SelectedItems(f.Selection) {
		if keyed[it.Key] == nil {
			st := f.States[it.Key]
			keyed[it.Key] = add(it.Key, it.Title, st.State, st.Why)
		}
	}
	for _, it := range f.Held {
		if keyed[it.Key] == nil {
			keyed[it.Key] = add(it.Key, it.Title, api.ItemWaiting, "retenu, pas encore lu")
		}
	}
	// What it drops: an item the choice no longer has is « à retirer » - its
	// removal waiting on a whole sync says why - titled by its document
	// below, else by its key.
	dropped := map[string]bool{}
	var gone []string
	for k := range f.States {
		if keyed[k] == nil {
			gone = append(gone, k)
		}
	}
	sort.Strings(gone)
	for _, k := range gone {
		st := f.States[k]
		if st.State == api.ItemWaiting {
			st.State = api.ItemRemoved
		}
		keyed[k] = add(k, "", st.State, st.Why)
		dropped[k] = true
	}
	// What it brought: each item a sync anchored as a source-document under
	// it, matched to its item by the key it was brought for - read; a
	// document with no key (an older anchor, a connector that says none)
	// matches an item by its title.
	docs, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: module, Parent: id, Type: api.SourceDocumentType})
	if err != nil {
		return nil, fmt.Errorf("sources: the items %s brought: %w", id, err)
	}
	root := projectRoot(ctx, cwm, module)
	titled := map[string]map[string]any{}
	for _, it := range items {
		row := it.(map[string]any)
		if t := row["title"].(string); titled[t] == nil {
			titled[t] = row
		}
	}
	for _, d := range docs.Objects {
		name := html.UnescapeString(d.Name)
		if name == "" {
			name = localID(d.ID)
		}
		// A listed object carries no fields: the key, kind, read date and
		// data file are read from the document.
		var df documentFields
		if got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: d.ID}); err == nil {
			_ = json.Unmarshal(got.Object.FieldsJSON(), &df)
		}
		row := keyed[df.ItemKey]
		if df.ItemKey == "" {
			row = titled[name]
		}
		switch {
		case row != nil && row["doc"] == nil:
			if dropped[df.ItemKey] && row["title"] == row["key"] {
				row["title"] = name
			}
		case row != nil && dropped[df.ItemKey]:
			// Another document of a dropped item (one read with its
			// children) leaves with it.
			why, _ := row["why"].(string)
			row = add(d.ID, name, row["state"].(string), why)
		default:
			row = add(d.ID, name, api.ItemRead, "")
		}
		row["doc"] = d.ID
		readColumns(row, df, conn, root)
	}

	count := map[string]int{}
	for _, it := range items {
		count[it.(map[string]any)["state"].(string)]++
	}
	var counts []string
	for _, st := range detailStates {
		if n := count[st]; n > 0 {
			word := itemStateWords[st].label
			if st == api.ItemRead && n > 1 {
				word += "s"
			}
			counts = append(counts, fmt.Sprintf("%d %s", n, word))
		}
	}
	state := map[string]any{"text": "", "color": "gray"}
	if u.State != nil {
		state = map[string]any{"text": u.State.Text, "color": u.State.Color}
	}
	out := map[string]any{"id": id, "label": label, "state": state["text"], "state_color": state["color"],
		"meta": u.Meta, "counts": strings.Join(counts, " · "), "items": items}
	if len(conn.Read) > 0 {
		out["read_columns"] = conn.Read
	}
	if u.Note != "" {
		out["note"] = u.Note
	}
	return out, nil
}

// sourceDetailTree draws the detail: the connector's declared label when
// withLabel (it names the source where nothing else does - its own pane's
// « Source » tab, an opened card is already titled by it
// ), the source's state badge, its meta line and counts, what
// to do in a callout when it failed or changed, then its items as a table
// - sortable by column, filtered by its title, a row
// opening the item read, its open mark before its title.
func sourceDetailTree(detail map[string]any, withLabel bool) map[string]any {
	head := []any{}
	if withLabel {
		head = append(head, map[string]any{"kind": "badge", "text": "", "dataField": "$.detail.label", "color": "gray"})
	}
	head = append(head,
		map[string]any{"kind": "badge", "text": "", "dataField": "$.detail.state", "color": detail["state_color"]},
		map[string]any{"kind": "text", "content": "", "dataField": "$.detail.meta", "size": "xs", "color": "dimmed"},
		map[string]any{"kind": "text", "content": "", "dataField": "$.detail.counts", "size": "xs", "color": "dimmed"},
	)
	children := []any{map[string]any{"kind": "hbox", "gap": "sm", "children": head}}
	if _, ok := detail["note"]; ok {
		tone := "yellow"
		if detail["state_color"] == "red" {
			tone = "red"
		}
		children = append(children, map[string]any{"kind": "box", "id": "source-detail-note", "callout": tone, "children": []any{
			map[string]any{"kind": "text", "content": "", "dataField": "$.detail.note", "size": "sm"},
		}})
	}
	if items, _ := detail["items"].([]any); len(items) == 0 {
		children = append(children, map[string]any{"kind": "text", "id": "source-detail-items",
			"content": "Rien d'apporté pour l'instant : choisissez des éléments, puis « Actualiser ».", "size": "sm", "color": "dimmed"})
	} else {
		cols, _ := detail["read_columns"].([]api.ReadColumn)
		lead, _ := detail["read_lead"].([]api.ReadColumn)
		children = append(children, render.Table(readSchemaLed(lead, cols), nil).With("id", "source-detail-items").With("dataField", "$.detail.items").
			With("open", map[string]any{"label": "Ouvrir", "icon": "eye", "when": "doc", "navigate": map[string]any{"object": "${item.doc}"}}))
	}
	return map[string]any{"kind": "section", "id": "source-detail", "size": "sm", "title": "Ce que la source a apporté",
		"icon": "list-details", "color": "teal", "children": children}
}

// readRow is a row of « Contenu lu » as its table draws it: the open mark
// (an icon) and title, the item's kind, its state and why, when it was read; the
// connector's own columns are laid beside (readSchema).
type readRow struct {
	Open   string `json:"open,omitempty" jsonschema:"content=icon"`
	Title  string `json:"title"`
	Kind   string `json:"kind,omitempty"`
	State  string `json:"state_label"`
	Why    string `json:"why,omitempty"`
	ReadAt string `json:"read_at,omitempty"`
}

// readOpenIcon is a read row's open mark before its title: display's icon
// content draws it (string@icon); a row not read yet carries none and does
// not open (its table's open is when: doc).
const readOpenIcon = "external-link"

// readColumnPrefix keeps a connector's column clear of the table's own.
const readColumnPrefix = "x_"

// readSchema is « Contenu lu »'s table: the columns every source shows, then
// those its connector declares (api.ConnectorDecl.Read), each sortable, the
// title searched, its head kept in view while the rows scroll under it.
func readSchema(cols []api.ReadColumn) map[string]any {
	return readSchemaLed(nil, cols)
}

// readSchemaLed is readSchema led by the unit's id column:
// it comes first, the open mark before it, and is filtered on beside the
// title; a slot or read column on the same attribute is not drawn again.
func readSchemaLed(lead []api.ReadColumn, cols []api.ReadColumn) map[string]any {
	schema := render.InlineSchema[readRow]("ReadItem", map[string]string{
		"title": "Titre", "kind": "Type", "state_label": "État", "read_at": "Lu le"})
	types, _ := schema["types"].(map[string]any)
	root, _ := types["ReadItem"].(map[string]any)
	fields, _ := root["fields"].(map[string]any)
	title := map[string]any{"id": "title", "name": "title", "label": "Titre", "sortable": true, "prepend": []any{"open"}}
	var columns []any
	filters := []any{"title"}
	seen := map[string]bool{}
	if len(lead) > 0 && strings.TrimSpace(lead[0].Attr) != "" {
		label := lead[0].Label
		if label == "" {
			label = lead[0].Attr
		}
		id := columnID(lead[0].Attr)
		seen[id] = true
		if fields != nil {
			fields[id] = map[string]any{"type": "string", "label": label, "optional": true}
		}
		columns = append(columns, map[string]any{"id": id, "name": id, "label": label, "sortable": true, "prepend": []any{"open"}})
		filters = []any{id, "title"}
		delete(title, "prepend")
	}
	columns = append(columns, title,
		map[string]any{"id": "kind", "name": "kind", "label": "Type", "sortable": true},
		map[string]any{"id": "state_label", "name": "state_label", "label": "État", "sortable": true, "append": []any{"why"}},
		map[string]any{"id": "read_at", "name": "read_at", "label": "Lu le", "sortable": true},
	)
	for _, c := range cols {
		if strings.TrimSpace(c.Attr) == "" {
			continue
		}
		label := c.Label
		if label == "" {
			label = c.Attr
		}
		id := columnID(c.Attr)
		if seen[id] {
			continue
		}
		seen[id] = true
		if fields != nil {
			fields[id] = map[string]any{"type": "string", "label": label, "optional": true}
		}
		columns = append(columns, map[string]any{"id": id, "name": id, "label": label, "sortable": true})
	}
	if root != nil {
		root["extensions"] = []any{map[string]any{"type": "ui-table", "name": "default", "columns": columns,
			"sortable": true, "filters": filters, "pageSize": 50, "stickyHeader": true}}
	}
	return schema
}

// readColumns fills a row read from its document: the open mark, its kind as
// its connector names it, when it was read, and each column the connector
// declares from the item's data file.
func readColumns(row map[string]any, df documentFields, conn api.ConnectorDecl, root string) {
	row["open"] = readOpenIcon
	// Its kind and data file as they are, for « Contenu lu » grouped by
	// kind.
	if df.ItemKind != "" {
		row["kind_key"] = df.ItemKind
	}
	if df.DataFile != "" {
		row["data_file"] = df.DataFile
	}
	if df.ItemKind != "" {
		kind := df.ItemKind
		if k, ok := conn.Kinds[kind]; ok && k.Label != "" {
			kind = k.Label
		}
		row["kind"] = kind
	}
	if at, err := time.Parse(time.RFC3339, df.IngestedAt); err == nil {
		row["read_at"] = at.Local().Format("2006-01-02 15:04")
	}
	if len(conn.Read) == 0 {
		return
	}
	raw, ok := projectFile(root, df.DataFile)
	if !ok {
		return
	}
	var data map[string]any
	if json.Unmarshal(raw, &data) != nil {
		return
	}
	for _, c := range conn.Read {
		if v := cellText(data[c.Attr]); v != "" {
			row[columnID(c.Attr)] = html.UnescapeString(v)
		}
	}
}

// cellText is a data value as a table cell reads it: a text as is, a list
// joined, a record by its name, nothing empty.
func cellText(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []any:
		var parts []string
		for _, e := range t {
			if s := cellText(e); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	case map[string]any:
		if n, ok := t["name"].(string); ok {
			return n
		}
		return ""
	default:
		return fmt.Sprint(t)
	}
}
