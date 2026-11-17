// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"fmt"
	"sort"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// LISTS OF OBJECTS, as every panel listing objects draws them — the module's
// contents, a search, a module's goals or features, what challenges an
// object. The object pane owns how an object shows in a list; a panel's
// source (in the pane or in the extension owning a type) reads its rows here
// and draws them with these builders, adding its own columns. The rows come
// from CWM's index; a parent outside them is read by its id (nameParents) -
// never a module's whole objects.

// ObjectRow is one object as a list shows it: its name opening it, its type,
// its lifecycle state coloured as the lifecycle badge colours it, and the
// object it sits under.
type ObjectRow struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	TypeLabel  string `json:"typeLabel"` // the type's label_plural, else its label, else its name
	TypeIcon   string `json:"typeIcon,omitempty"`
	Module     string `json:"module"`
	State      string `json:"state,omitempty"`      // its lifecycle state: stored, else its workflow's initial one
	StateColor string `json:"stateColor,omitempty"` // as the lifecycle badge colours it
	Parent     string `json:"parent,omitempty"`
	ParentName string `json:"parentName,omitempty"`
	// Project is the key of the project holding the object when the universe
	// serves several (the search names it); "" with one.
	Project string `json:"project,omitempty"`
	// Object is the resident record the row was read from.
	Object cwmapi.Object `json:"-"`
}

// ReadRows runs a search (its states asked) and answers each object found as
// a row: its type's label, its state and its parent's name — the parents
// outside the results read by their ids (nameParents).
func ReadRows(ctx context.Context, cwm cwmapi.CWMService, in cwmapi.ObjectSearchInput) ([]ObjectRow, error) {
	in.WithStates = true
	got, err := cwm.ObjectSearch(ctx, in)
	if err != nil {
		return nil, err
	}
	return rowsOf(ctx, cwm, got.Objects, got.Projects, got.States), nil
}

// Under is the search of everything under an object: its subtree, or for a
// project the whole index — a project's modules hang under it through the
// index, not through a parent. The object itself is in a subtree: a caller
// listing what it holds leaves it out.
func Under(ctx context.Context, cwm cwmapi.CWMService, id string) (cwmapi.ObjectSearchInput, error) {
	module, err := cwmapi.ModuleOfObjectID(id)
	if err != nil {
		return cwmapi.ObjectSearchInput{}, err
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
	if err != nil {
		return cwmapi.ObjectSearchInput{}, err
	}
	if isProject(got.Object) {
		return cwmapi.ObjectSearchInput{}, nil
	}
	return cwmapi.ObjectSearchInput{Within: id}, nil
}

// Holding is what an object holds, as rows: everything under it less itself -
// for a project its whole index. The id is bare or qualified; when the
// universe serves several projects the rows are those of the project holding
// the object only (two projects may hold one bare id, and a project object's
// contents are its own project's objects). query is the search's text.
func Holding(ctx context.Context, cwm cwmapi.CWMService, id, query string) ([]ObjectRow, error) {
	return HoldingSearch(ctx, cwm, id, cwmapi.ObjectSearchInput{Query: query})
}

// HoldingSearch is Holding narrowed by search: its text, types, states... run
// in CWM's index (Within is Holding's own). A list of one type passes its
// Types here and never filters afterwards: a project's whole index read over
// the bus passes NATS's payload limit.
func HoldingSearch(ctx context.Context, cwm cwmapi.CWMService, id string, search cwmapi.ObjectSearchInput) ([]ObjectRow, error) {
	module, err := cwmapi.ModuleOfObjectID(id)
	if err != nil {
		return nil, err
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
	if err != nil {
		return nil, err
	}
	search.Within = ""
	if !isProject(got.Object) {
		// Rows and the index name the object bare; its project keeps the rows.
		search.Within = got.Object.ID
	}
	rows, err := ReadRows(ctx, cwm, search)
	if err != nil {
		return nil, err
	}
	out := rows[:0:0]
	for _, r := range rows {
		if got.Project != "" && r.Project != got.Project {
			continue
		}
		if r.ID == got.Object.ID {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

func isProject(o cwmapi.Object) bool {
	_, project := o.Capability(cwmapi.CapProject)
	return project || o.Type == cwmapi.CapProject
}

// Without is rows less the object id - bare, or qualified to leave out only
// that project's row.
func Without(rows []ObjectRow, id string) []ObjectRow {
	key, bare, err := cwmapi.SplitReference(id)
	if err != nil {
		key, bare = "", id
	}
	out := rows[:0:0]
	for _, r := range rows {
		if r.ID == bare && (key == "" || r.Project == "" || r.Project == key) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// ReadRowsOf answers the objects named by ids as rows, in the order given;
// an id that is not an object of the universe's projects is left out. An id
// is bare or qualified: a qualified id answers its project's object, a bare
// one each project's object holding it when the universe serves several
// (two projects may hold one bare id).
func ReadRowsOf(ctx context.Context, cwm cwmapi.CWMService, ids []string) ([]ObjectRow, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	modules := map[string]bool{}
	var mods []string
	for _, id := range ids {
		_, bare, err := cwmapi.SplitReference(id)
		if err != nil {
			continue
		}
		if m, err := cwmapi.ModuleOfObjectID(bare); err == nil && !modules[m] {
			modules[m] = true
			mods = append(mods, m)
		}
	}
	got, err := cwm.ObjectSearch(ctx, cwmapi.ObjectSearchInput{Modules: mods, WithStates: true})
	if err != nil {
		return nil, err
	}
	several := len(got.Projects) == len(got.Objects) && len(got.Projects) > 0
	// Each bare id's objects, one per project, in the search's order.
	byID := map[string][]int{}
	for i, o := range got.Objects {
		if !several && len(byID[o.ID]) > 0 {
			continue
		}
		byID[o.ID] = append(byID[o.ID], i)
	}
	var objs []cwmapi.Object
	var projects []string
	taken := map[int]bool{}
	for _, id := range ids {
		key, bare, err := cwmapi.SplitReference(id)
		if err != nil {
			continue
		}
		for _, i := range byID[bare] {
			if taken[i] || (key != "" && several && got.Projects[i] != key) {
				continue
			}
			taken[i] = true
			objs = append(objs, got.Objects[i])
			if several {
				projects = append(projects, got.Projects[i])
			}
		}
	}
	return rowsOf(ctx, cwm, objs, projects, got.States), nil
}

// rowsOf draws objs as rows. projects, when the universe serves several
// projects, names each object's project in the order of objs: states and the
// parents' names are then keyed by the qualified id (<key>:<module>.<local>) -
// two projects may hold one bare id - else by the bare id.
func rowsOf(ctx context.Context, cwm cwmapi.CWMService, objs []cwmapi.Object, projects []string, states map[string]cwmapi.ObjectState) []ObjectRow {
	if len(projects) != len(objs) {
		projects = nil
	}
	// keyOf is how an object of project p is named in the maps; parentKey
	// how the parent an object of project p names is (a qualified parent
	// names its own project).
	keyOf := func(p, id string) string {
		if projects == nil || p == "" {
			return id
		}
		return cwmapi.QualifiedID(p, id)
	}
	parentKey := func(p, parent string) string {
		if k, bare, err := cwmapi.SplitReference(parent); err == nil && k != "" {
			return keyOf(k, bare)
		}
		return keyOf(p, parent)
	}
	projectOf := func(i int) string {
		if projects == nil {
			return ""
		}
		return projects[i]
	}
	names := map[string]string{}
	for i, o := range objs {
		names[keyOf(projectOf(i), o.ID)] = o.DisplayTitle()
	}
	nameParents(ctx, cwm, objs, projects != nil, projectOf, parentKey, names)
	types := map[string]cwmapi.ObjectType{}
	rows := make([]ObjectRow, 0, len(objs))
	for i, o := range objs {
		t, seen := types[o.Type]
		if !seen {
			if got, err := cwm.ObjectTypeGet(ctx, cwmapi.ObjectTypeGetInput{Name: o.Type}); err == nil {
				t = got.ObjectType
			}
			types[o.Type] = t
		}
		m, _ := cwmapi.ModuleOfObjectID(o.ID)
		r := ObjectRow{
			ID: o.ID, Name: o.DisplayTitle(), Type: o.Type, TypeLabel: firstNonEmpty(t.LabelPlural, t.Label, o.Type),
			TypeIcon: firstNonEmpty(t.IconName, t.Icon), Module: m, Parent: o.ParentID(), Object: o,
		}
		if r.Name == "" {
			r.Name = o.ID
		}
		if r.Parent != "" {
			r.ParentName = firstNonEmpty(names[parentKey(projectOf(i), r.Parent)], r.Parent)
		}
		r.Project = projectOf(i)
		if s, ok := states[keyOf(r.Project, o.ID)]; ok {
			r.State, r.StateColor = s.State, s.Color
		}
		rows = append(rows, r)
	}
	return rows
}

// ParentsByID is how many parents of one module rowsOf reads one by one;
// past it, the module's names alone are listed (ObjectList with Fields
// [name]) - never its whole objects: a module's list runs to megabytes and
// crosses the bus to the sidecars drawing lists (goals, features).
const ParentsByID = 8

// nameParents names in names the parents of objs that are not among them:
// each read by its id (qualified when it names its project), or, when a
// module holds more than ParentsByID of them, its objects' names listed.
// several says the keys are qualified (the universe serves several projects).
func nameParents(ctx context.Context, cwm cwmapi.CWMService, objs []cwmapi.Object, several bool, projectOf func(int) string,
	parentKey func(p, parent string) string, names map[string]string) {
	type missing struct{ key, module string }
	var want []missing
	seen := map[string]bool{}
	perModule := map[string]int{}
	for i, o := range objs {
		p := o.ParentID()
		if p == "" {
			continue
		}
		k := parentKey(projectOf(i), p)
		if _, ok := names[k]; ok || seen[k] {
			continue
		}
		_, bare, err := cwmapi.SplitReference(k)
		if err != nil {
			continue
		}
		m, err := cwmapi.ModuleOfObjectID(bare)
		if err != nil {
			continue
		}
		seen[k] = true
		want = append(want, missing{k, m})
		perModule[m]++
	}
	listed := map[string]bool{}
	for _, w := range want {
		if perModule[w.module] <= ParentsByID {
			if got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: w.module, ID: w.key}); err == nil {
				names[w.key] = got.Object.DisplayTitle()
			}
			continue
		}
		if listed[w.module] {
			continue
		}
		listed[w.module] = true
		got, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: w.module, Limit: -1, Fields: []string{"name"}})
		if err != nil {
			continue
		}
		qualified := len(got.Projects) == len(got.Objects)
		for j, x := range got.Objects {
			k := x.ID
			if several && qualified && got.Projects[j] != "" {
				k = cwmapi.QualifiedID(got.Projects[j], x.ID)
			}
			if _, ok := names[k]; !ok {
				names[k] = x.DisplayTitle()
			}
		}
	}
}

// Ref is the id a link opens the row's object by: qualified with its project
// when the universe serves several, else bare.
func (r ObjectRow) Ref() string {
	if r.Project == "" {
		return r.ID
	}
	return cwmapi.QualifiedID(r.Project, r.ID)
}

// ParentRef is the id a link opens the row's parent by: an already qualified
// parent as written, a bare one qualified with the row's project when the
// universe serves several.
func (r ObjectRow) ParentRef() string {
	if r.Parent == "" || r.Project == "" {
		return r.Parent
	}
	if k, _, err := cwmapi.SplitReference(r.Parent); err == nil && k != "" {
		return r.Parent
	}
	return cwmapi.QualifiedID(r.Project, r.Parent)
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

// StateBadge is a row's state as a badge in its colour (gray when the
// lifecycle badge names none); nil when it follows no workflow.
func StateBadge(r ObjectRow) render.Node {
	if r.State == "" {
		return nil
	}
	return render.Badge(r.State, firstNonEmpty(r.StateColor, "gray")).With("size", "xs").With("icon", "progress")
}

// RowNode is one object on one line: its name opening it and its state at
// the left, then extra (the list's own columns: a reachability, a cost of
// delay…) and the object it sits under, opening it, at the right.
//
// When the universe serves several projects (r.Project set) both links open
// the object by its qualified id: a bare id held by two projects is refused.
func RowNode(r ObjectRow, extra ...render.Node) render.Node {
	left := []render.Node{render.Link(r.Name, render.NavigateTo("", r.Ref(), "")).With("size", "sm").With("fw", 500)}
	if b := StateBadge(r); b != nil {
		left = append(left, b)
	}
	var right []render.Node
	for _, n := range extra {
		if n != nil {
			right = append(right, n)
		}
	}
	if r.Parent != "" {
		right = append(right, render.Link("↳ "+r.ParentName, render.NavigateTo("", r.ParentRef(), "")).
			With("size", "xs").With("color", "dimmed").With("tooltip", r.Parent))
	}
	return render.HBox(
		render.HBox(left...).With("wrap", false).With("align", "center"),
		render.HBox(right...).With("wrap", false).With("align", "center"),
	).With("justify", "space-between").With("wrap", false).With("align", "center")
}

// TypeGroup is the rows of one type, as ByType groups them.
type TypeGroup struct {
	Type  string
	Label string
	Icon  string
	Rows  []ObjectRow
}

// GroupByType groups rows by type — the largest group first, then by label —
// each group's rows by name.
func GroupByType(rows []ObjectRow) []TypeGroup {
	idx := map[string]int{}
	var groups []TypeGroup
	for _, r := range rows {
		i, ok := idx[r.Type]
		if !ok {
			i = len(groups)
			idx[r.Type] = i
			groups = append(groups, TypeGroup{Type: r.Type, Label: r.TypeLabel, Icon: r.TypeIcon})
		}
		groups[i].Rows = append(groups[i].Rows, r)
	}
	for i := range groups {
		rs := groups[i].Rows
		sort.SliceStable(rs, func(a, b int) bool { return rs[a].Name < rs[b].Name })
	}
	sort.SliceStable(groups, func(a, b int) bool {
		if len(groups[a].Rows) != len(groups[b].Rows) {
			return len(groups[a].Rows) > len(groups[b].Rows)
		}
		return groups[a].Label < groups[b].Label
	})
	return groups
}

// FoldRows is how many rows a fold of ByType draws; the rest are counted
// under them — a filter shows them.
const FoldRows = 100

// ByType draws rows as one fold per type — its label, its count, its first
// FoldRows rows — every fold open; row draws one row (RowNode when nil).
func ByType(rows []ObjectRow, row func(ObjectRow) render.Node) render.Node {
	if row == nil {
		row = func(r ObjectRow) render.Node { return RowNode(r) }
	}
	var folds []render.Fold
	var open []string
	for _, g := range GroupByType(rows) {
		lines := make([]render.Node, 0, len(g.Rows))
		for i, r := range g.Rows {
			if i == FoldRows {
				lines = append(lines, render.Dimmed(fmt.Sprintf("+ %d …", len(g.Rows)-FoldRows)))
				break
			}
			lines = append(lines, row(r))
		}
		folds = append(folds, render.Fold{ID: g.Type, Label: g.Label, Icon: firstNonEmpty(g.Icon, "point"), Badge: len(g.Rows),
			Content: render.VBox(lines...).With("gap", "xs")})
		open = append(open, g.Type)
	}
	return render.Accordion(folds, open...)
}

// Chip is one choice of a filter row: its label, how many objects it keeps,
// its colour, whether it is the current one, and the query choosing it runs.
type Chip struct {
	Label  string
	Count  int
	Color  string
	Active bool
	Query  map[string]any
	Tip    string
}

// Chips is a filter row: a small label, then each choice as a badge that
// re-reads the panel's source with its query — filled when it is the current
// one.
func Chips(label string, chips []Chip) render.Node {
	nodes := []render.Node{render.Text(label).With("size", "xs").With("color", "dimmed").With("fw", 500)}
	for _, c := range chips {
		text := c.Label
		if c.Count >= 0 {
			text = fmt.Sprintf("%s · %d", c.Label, c.Count)
		}
		b := render.Badge(text, firstNonEmpty(c.Color, "gray")).With("keepCase", true).
			With("action", render.QueryWith(c.Label, c.Query))
		if c.Active {
			b = b.With("variant", "filled")
		} else {
			b = b.With("variant", "light")
		}
		if c.Tip != "" {
			b = b.With("tooltip", c.Tip)
		}
		nodes = append(nodes, b)
	}
	return render.HBox(nodes...).With("align", "center")
}

// Query is a panel's current filter as the args of a query action: a copy
// of q with key set to v (dropped when v is empty) — a chip changes one
// criterion and keeps the others.
func Query(q map[string]any, key string, v any) map[string]any {
	out := make(map[string]any, len(q)+1)
	for k, x := range q {
		if x != nil && x != "" {
			out[k] = x
		}
	}
	if s, isStr := v.(string); v == nil || (isStr && s == "") {
		delete(out, key)
	} else {
		out[key] = v
	}
	return out
}

// StateCount is how many rows are in one state, and its colour.
type StateCount struct {
	State string
	Color string
	Count int
}

// CountStates counts the rows by state, the most frequent first; rows
// following no workflow are not counted.
func CountStates(rows []ObjectRow) []StateCount {
	idx := map[string]int{}
	var out []StateCount
	for _, r := range rows {
		if r.State == "" {
			continue
		}
		i, ok := idx[r.State]
		if !ok {
			i = len(out)
			idx[r.State] = i
			out = append(out, StateCount{State: r.State, Color: r.StateColor})
		}
		out[i].Count++
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Count != out[b].Count {
			return out[a].Count > out[b].Count
		}
		return out[a].State < out[b].State
	})
	return out
}

// StateChips is a filter row by state over rows: all, then each state with
// its count and colour; current is the chosen one, q the panel's current
// filter (each chip sets its `state`). Nil when no row follows a workflow.
func StateChips(label, all string, rows []ObjectRow, current string, q map[string]any) render.Node {
	states := CountStates(rows)
	if len(states) == 0 {
		return nil
	}
	chips := []Chip{{Label: all, Count: len(rows), Active: current == "", Query: Query(q, "state", "")}}
	for _, st := range states {
		chips = append(chips, Chip{Label: st.State, Count: st.Count, Color: st.Color, Active: current == st.State, Query: Query(q, "state", st.State)})
	}
	return Chips(label, chips)
}

// StateBar is rows by state as one stacked bar, each state in its colour,
// listed under it. Nil when no row follows a workflow.
func StateBar(label string, rows []ObjectRow) render.Node {
	states := CountStates(rows)
	if len(states) == 0 {
		return nil
	}
	segs := make([]render.Segment, 0, len(states))
	total := 0
	for _, st := range states {
		segs = append(segs, render.Segment{Value: float64(st.Count), Color: firstNonEmpty(st.Color, "gray"), Label: st.State})
		total += st.Count
	}
	return render.Stacked(label, segs...).With("max", total).With("valueText", fmt.Sprintf("%d", total))
}
