// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
)

// SearchLimit is how many objects a search shows; the rest are counted.
const SearchLimit = 300

// Search is a search across the universe (or one module, the target), as the
// page the Objects panel shows: one box taking a code or words and the types
// it keeps, the panel's saved views as chips, the criteria in
// force beyond the box as chips that remove them, the full form folded as
// Advanced (hidden at the Essentiel display level), and the objects kept -
// the ones whose id the box names first, then by type. A view's criteria
// replace the typed ones; with neither, nothing is searched. Read from CWM's
// index only (ObjectSearch, its ObjectFilter and its states).
func (s *Service) Search(ctx context.Context, in api.SearchInput) (render.Def, error) {
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return render.Def{}, fmt.Errorf("object-pane: resolve CWMService: %w", err)
	}
	crit, view := in.SearchCriteria, (*api.SavedView)(nil)
	if in.View != "" {
		for i := range in.Views {
			if in.Views[i].Name == in.View {
				view = &in.Views[i]
				crit = view.SearchCriteria
			}
		}
		if view == nil {
			return render.Def{}, fmt.Errorf("%w: search: no saved view %q", cwmapi.ErrInvalidInput, in.View)
		}
	}
	var rows []api.ObjectRow
	total, searched := 0, !crit.Empty()
	if searched {
		q := cwmapi.ObjectSearchInput{Query: codeQuery(crit.Text), Types: crit.Types, States: crit.States}
		if in.Target != "" {
			q.Modules = []string{in.Target}
		}
		if len(crit.Capabilities)+len(crit.Incoming)+len(crit.Outgoing) > 0 {
			q.Filter = &cwmapi.ObjectFilter{Capabilities: crit.Capabilities, Incoming: crit.Incoming, Outgoing: crit.Outgoing}
		}
		found, err := cwm.ObjectSearch(ctx, q)
		if err != nil {
			return render.Def{}, err
		}
		total = found.Total
		q.Limit = SearchLimit
		if rows, err = api.ReadRows(ctx, cwm, q); err != nil {
			return render.Def{}, err
		}
		if in.Level != "expert" {
			rows, total = withoutSystem(ctx, cwm, rows, total)
		}
	}
	types := []string{}
	if got, err := cwm.ObjectTypeList(ctx, cwmapi.ObjectTypeListInput{}); err == nil {
		for _, t := range got.ObjectTypes {
			if !t.Abstract {
				types = append(types, t.Name)
			}
		}
	}
	tree := searchTree(in, view, crit, rows, total, searched, types)
	return render.Def{Kind: "page", DSLTree: tree, Data: map[string]any{"filter": crit}}, nil
}

// withoutSystem leaves out the rows of system modules - a module carrying the
// system-module capability, or an object in one: below the Expert level a
// search never offers them.
func withoutSystem(ctx context.Context, cwm cwmapi.CWMService, rows []api.ObjectRow, total int) ([]api.ObjectRow, int) {
	sys, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Capability: cwmapi.CapSystemModule})
	if err != nil || len(sys.Objects) == 0 {
		return rows, total
	}
	system := make(map[string]bool, len(sys.Objects))
	for _, o := range sys.Objects {
		system[o.ID] = true
	}
	kept := rows[:0]
	for _, r := range rows {
		module, _ := cwmapi.ModuleOfObjectID(r.ID)
		if system[r.ID] || system[module] {
			total--
			continue
		}
		kept = append(kept, r)
	}
	return kept, total
}

// shortCode is a code as a person types it: letters, a dash and a number
// (F-31, rq-28) - the id's part after its module.
var shortCode = regexp.MustCompile(`^([A-Za-z]+)-0*(\d+)$`)

// codeQuery is the box's text as the index searches it: a short code gets its
// number on three digits and its letters in capitals (rq-28 → RQ-028), as ids
// write them; anything else as typed.
func codeQuery(text string) string {
	text = strings.TrimSpace(text)
	if m := shortCode.FindStringSubmatch(text); m != nil {
		n, _ := strconv.Atoi(m[2])
		return fmt.Sprintf("%s-%03d", strings.ToUpper(m[1]), n)
	}
	return text
}

// The ranks of a row against the box's text: its id named exactly, its code
// (the id after its module) named, its id begun, else none.
const (
	rankExactID = iota
	rankCode
	rankPrefix
	rankNone
)

// rankOf is how well a row's id answers the box's text.
func rankOf(id, text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return rankNone
	}
	lid, lt := strings.ToLower(id), strings.ToLower(text)
	if lid == lt {
		return rankExactID
	}
	code := lid
	if i := strings.Index(lid, "."); i >= 0 {
		code = lid[i+1:]
	}
	if q := strings.ToLower(codeQuery(text)); code == q || strings.HasPrefix(code, q+"-") {
		return rankCode
	}
	if strings.HasPrefix(lid, lt) || strings.HasPrefix(code, lt) {
		return rankPrefix
	}
	return rankNone
}

// bestMatches are the rows whose id the box names - exact, then by code, then
// begun - in that order; nothing when the text names no id.
func bestMatches(rows []api.ObjectRow, text string) []api.ObjectRow {
	var best []api.ObjectRow
	for _, r := range rows {
		if rankOf(r.ID, text) < rankNone {
			best = append(best, r)
		}
	}
	sort.SliceStable(best, func(i, j int) bool { return rankOf(best[i].ID, text) < rankOf(best[j].ID, text) })
	return best
}

// searchTree draws the search: the quick box, the saved views, the criteria
// in force, Advanced, then the results.
func searchTree(in api.SearchInput, view *api.SavedView, crit api.SearchCriteria, rows []api.ObjectRow, total int, searched bool, types []string) render.Node {
	lang := in.Language
	var blocks []render.Node
	typeEnum := func(schema map[string]any, root string) {
		if len(types) == 0 {
			return
		}
		r := schema["types"].(map[string]any)[root].(map[string]any)
		if f, ok := r["fields"].(map[string]any)["types"].(map[string]any); ok {
			if items, ok := f["items"].(map[string]any); ok {
				enum := make([]any, len(types))
				for i, t := range types {
					enum[i] = t
				}
				items["enum"] = enum
			}
		}
	}

	// The quick box: a code or words, and the types it keeps. It keeps the
	// criteria set beyond it (shown as chips below).
	quick := render.InlineSchema[api.QuickSearch]("QuickSearch", map[string]string{
		"text": tr(lang, "quickText"), "types": tr(lang, "quickTypes"),
	})
	typeEnum(quick, "QuickSearch")
	keep := map[string]any{"text": "${value.text}", "types": "${value.types}",
		"capabilities": crit.Capabilities, "states": crit.States, "incoming": crit.Incoming, "outgoing": crit.Outgoing}
	blocks = append(blocks, render.Card(render.Form(quick, render.QueryWith(tr(lang, "search"), keep)).
		With("dataField", "$.filter").With("mode", "edit")).With("p", "sm"))

	if len(in.Views) > 0 {
		chips := make([]api.Chip, 0, len(in.Views))
		for _, v := range in.Views {
			label := v.Labels[lang]
			if label == "" {
				label = firstOf(v.Label, v.Name)
			}
			chips = append(chips, api.Chip{Label: label, Count: -1, Color: "violet", Active: view != nil && view.Name == v.Name,
				Query: map[string]any{"view": v.Name}, Tip: v.Description})
		}
		blocks = append(blocks, api.Chips(tr(lang, "savedViews"), chips))
	}

	// Each criterion in force beyond the box is a chip; clicking it removes
	// it, so a narrowed result never looks like a missing object.
	if view == nil {
		if inForce := inForceChips(crit, lang); len(inForce) > 0 {
			blocks = append(blocks, api.Chips(tr(lang, "inForce"), inForce))
		}
	}

	// Advanced: the full form, folded; not offered at the Essentiel level.
	if in.Level != "essential" {
		schema := render.InlineSchema[api.SearchCriteria]("SearchCriteria", map[string]string{
			"text": tr(lang, "fText"), "types": tr(lang, "fTypes"), "capabilities": tr(lang, "fCapabilities"),
			"states": tr(lang, "fStates"), "incoming": tr(lang, "fIncoming"), "outgoing": tr(lang, "fOutgoing"),
		})
		typeEnum(schema, "SearchCriteria")
		form := render.Form(schema, render.QueryWith(tr(lang, "search"), nil)).With("dataField", "$.filter").With("mode", "edit")
		blocks = append(blocks, render.Accordion([]render.Fold{{ID: "advanced", Label: tr(lang, "advanced"), Icon: "adjustments", Content: form}}))
	}

	if !searched {
		blocks = append(blocks, render.Dimmed(tr(lang, "pickOrSearch")))
		return render.VBox(blocks...).With("gap", "md")
	}
	row := func(r api.ObjectRow) render.Node {
		return api.RowNode(r, render.Badge(r.Module, "gray").With("size", "xs").With("variant", "outline").With("mono", true))
	}
	if best := bestMatches(rows, crit.Text); len(best) > 0 {
		nodes := make([]render.Node, 0, len(best))
		for _, r := range best {
			nodes = append(nodes, row(r))
		}
		blocks = append(blocks, render.Section(tr(lang, "bestMatch"), "target").With("size", "md").With("color", "teal").
			With("children", []render.Node{render.VBox(nodes...).With("gap", "xs")}))
	}
	title := tr(lang, "results")
	if view != nil {
		title = firstOf(view.Labels[lang], view.Label, view.Name)
	}
	sec := render.Section(title, "search").With("size", "md").With("color", "violet").With("badge", total)
	if view != nil && view.Description != "" {
		sec = sec.With("description", view.Description)
	}
	if total > len(rows) {
		sec = sec.Aside(render.Badge(fmt.Sprintf(tr(lang, "firstOf"), len(rows), total), "orange").With("size", "xs").With("keepCase", true))
	}
	if len(rows) == 0 {
		sec = sec.Empty(tr(lang, "nothingMatches"), "")
	} else {
		sec["children"] = []render.Node{api.ByType(rows, row)}
	}
	return render.VBox(append(blocks, sec)...).With("gap", "md")
}

// inForceChips are the criteria set beyond the quick box (capabilities,
// states, link kinds), one chip each, whose query is the search without it.
func inForceChips(crit api.SearchCriteria, lang string) []api.Chip {
	base := map[string]any{"text": crit.Text, "types": crit.Types, "capabilities": crit.Capabilities,
		"states": crit.States, "incoming": crit.Incoming, "outgoing": crit.Outgoing}
	var out []api.Chip
	add := func(field, label string, values []string) {
		for _, v := range values {
			q := map[string]any{}
			for k, x := range base {
				q[k] = x
			}
			rest := []string{}
			for _, o := range values {
				if o != v {
					rest = append(rest, o)
				}
			}
			q[field] = rest
			out = append(out, api.Chip{Label: label + " : " + v + " ✕", Count: -1, Color: "orange", Active: true, Query: q, Tip: tr(lang, "removeCriterion")})
		}
	}
	add("capabilities", tr(lang, "fCapabilities"), crit.Capabilities)
	add("states", tr(lang, "fStates"), crit.States)
	add("incoming", tr(lang, "fIncoming"), crit.Incoming)
	add("outgoing", tr(lang, "fOutgoing"), crit.Outgoing)
	return out
}
