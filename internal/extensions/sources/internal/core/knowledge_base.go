// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/internal/anchor"
)

// The words of a knowledge base's kinds and states, as mock-up v6 draws them
// (views 15-17, 26).
var (
	kbKindWords  = map[string]string{api.KBKindDocument: "Document", api.KBKindRecord: "Enregistrement"}
	kbStateWords = map[string]string{api.KBStateRead: "Lu", api.KBStateStale: "À relire"}
)

// kbItem is one item of a knowledge base: a source-document one of the
// holder's sources brought.
type kbItem struct {
	row    map[string]any
	source string
	kind   string
	state  string
}

// KnowledgeBase is a holder's knowledge base (mock-up v6
// views 15-17 and 26): every item its information sources brought - each
// source-document under one of them - once, with the source it came from;
// a record when it brings fields (a data file), a document otherwise; stale
// (« À relire ») when its source changed it since it was read. Counts by kind
// and state, filters by source, kind and state - each a query of the panel
// with every filter over its input -, the sources with stale items to update,
// an item opening its source-document in a drawer. The same for any holder,
// v1 or v2.
func (s *Service) KnowledgeBase(ctx context.Context, in api.KnowledgeBaseInput) (render.Def, error) {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		return render.Def{}, fmt.Errorf("%w: sources.KnowledgeBase: id required", cwmapi.ErrInvalidInput)
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return render.Def{}, fmt.Errorf("sources: resolve CWMService: %w", err)
	}
	if _, err := carrierOf(ctx, cwm, id); err != nil {
		return render.Def{}, err
	}
	srcs, err := sourcesOf(ctx, cwm, id)
	if err != nil {
		return render.Def{}, err
	}
	s.drawn(ctx, srcs)
	kinds := map[string]map[string]api.ItemKind{}
	if list, err := s.Connectors(ctx, api.ConnectorsInput{}); err == nil {
		for _, c := range list.Connectors {
			kinds[c.Name] = c.Kinds
		}
	}

	var all []kbItem
	seen := map[string]bool{}
	sourceColors := map[string]any{}
	for _, src := range srcs {
		module, err := cwmapi.ModuleOfObjectID(src.ID)
		if err != nil {
			continue
		}
		docs, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: module, Parent: src.ID, Type: api.SourceDocumentType})
		if err != nil {
			return render.Def{}, fmt.Errorf("sources: the items %s brought: %w", src.ID, err)
		}
		name := src.Name
		if name == "" {
			name = src.ConnectorLabel
		}
		color := src.ConnectorColor
		if color == "" {
			color = "gray"
		}
		sourceColors[src.ConnectorLabel] = color
		for _, d := range docs.Objects {
			if seen[d.ID] {
				continue
			}
			seen[d.ID] = true
			got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: d.ID})
			if err != nil {
				return render.Def{}, fmt.Errorf("sources: read %s: %w", d.ID, err)
			}
			var f documentFields
			_ = json.Unmarshal(got.Object.FieldsJSON(), &f)
			body, _ := anchor.DecodeBody(got.Object.FieldsJSON())
			it := kbItem{source: src.ID, kind: kbKindOf(f), state: kbStateOf(body)}
			title := got.Object.Name
			if title == "" {
				title = localID(d.ID)
			}
			sub := kbSub(kinds[src.Connector], f, body)
			// The source column says each fact once: a source named as its
			// connector (« Documents », « Documents ») shows its badge alone.
			shownName := name
			if strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(src.ConnectorLabel)) {
				shownName = ""
			}
			it.row = map[string]any{"id": d.ID, "title": title, "sub": sub,
				"source": src.ID, "source_name": shownName, "source_label": src.ConnectorLabel,
				"kind": it.kind, "kind_label": kbKindWords[it.kind], "state": it.state, "state_label": kbStateWords[it.state]}
			all = append(all, it)
		}
	}

	f := kbFilter{source: strings.TrimSpace(in.Source), kind: strings.TrimSpace(in.Kind), state: strings.TrimSpace(in.State)}
	var shown []any
	count := map[string]int{}
	stale := map[string]int{}
	for _, it := range all {
		count["source:"+it.source]++
		count["kind:"+it.kind]++
		count["state:"+it.state]++
		if it.state == api.KBStateStale {
			stale[it.source]++
		}
		if (f.source == "" || f.source == it.source) && (f.kind == "" || f.kind == it.kind) && (f.state == "" || f.state == it.state) {
			shown = append(shown, it.row)
		}
	}
	if shown == nil {
		shown = []any{}
	}

	n := len(all)
	kpis := []any{
		f.chip(fmt.Sprint(n), "éléments", kbFilter{}),
		f.chip(fmt.Sprint(count["kind:"+api.KBKindDocument]), "documents — du texte", kbFilter{kind: api.KBKindDocument}),
		f.chip(fmt.Sprint(count["kind:"+api.KBKindRecord]), "enregistrements — des champs", kbFilter{kind: api.KBKindRecord}),
		f.chip(fmt.Sprint(count["state:"+api.KBStateStale]), "à relire — changés depuis", kbFilter{state: api.KBStateStale}),
	}
	bySource := []any{f.chip("", fmt.Sprintf("Toutes %d", n), f.with("source", ""))}
	var update []any
	for _, src := range srcs {
		if c := count["source:"+src.ID]; c > 0 {
			bySource = append(bySource, f.chip("", fmt.Sprintf("%s %d", src.ConnectorLabel, c), f.with("source", src.ID)))
		}
		if c := stale[src.ID]; c > 0 {
			update = append(update, map[string]any{"source": src.ID, "label": fmt.Sprintf("%s : %d à relire", src.ConnectorLabel, c)})
		}
	}
	if update == nil {
		update = []any{}
	}
	filters := []any{
		map[string]any{"group": "Source", "chips": bySource},
		map[string]any{"group": "Nature", "chips": []any{
			f.chip("", "Toutes", f.with("kind", "")),
			f.chip("", fmt.Sprintf("Documents %d", count["kind:"+api.KBKindDocument]), f.with("kind", api.KBKindDocument)),
			f.chip("", fmt.Sprintf("Enregistrements %d", count["kind:"+api.KBKindRecord]), f.with("kind", api.KBKindRecord)),
		}},
		map[string]any{"group": "État", "chips": []any{
			f.chip("", "Tous", f.with("state", "")),
			f.chip("", fmt.Sprintf("Lus %d", count["state:"+api.KBStateRead]), f.with("state", api.KBStateRead)),
			f.chip("", fmt.Sprintf("À relire %d", count["state:"+api.KBStateStale]), f.with("state", api.KBStateStale)),
		}},
	}
	showing := "Tous les éléments"
	var showAll []any
	if f != (kbFilter{}) {
		showing = fmt.Sprintf("%d élément(s) sur %d", len(shown), n)
		showAll = []any{kbFilter{}.args()}
	}
	if showAll == nil {
		showAll = []any{}
	}
	sub := "tout ce que l'IA consulte pour écrire, avec la source d'où il vient"
	if n > 0 {
		sub += fmt.Sprintf(" · %d élément(s)", n)
	}
	data := map[string]any{"sub": sub, "empty": n == 0, "update": update, "kpis": kpis, "filters": filters,
		"showing": showing, "show_all": showAll, "items": shown}
	return render.Def{Kind: "page", Title: "Base de connaissance", DSLTree: knowledgeBaseTree(n == 0, sourceColors), Data: data}, nil
}

// kbKindOf is a knowledge-base item's kind: a record when it brings fields
// (a data file), a document otherwise.
func kbKindOf(f documentFields) string {
	if f.DataFile != "" {
		return api.KBKindRecord
	}
	return api.KBKindDocument
}

// kbStateOf is a knowledge-base item's state: stale (« À relire ») when its
// source changed it since it was read, read otherwise.
func kbStateOf(body anchor.SourceDocBody) string {
	if body.AnchorStatus == anchor.AnchorStatusStale || body.AnchorStatus == anchor.AnchorStatusMissing {
		return api.KBStateStale
	}
	return api.KBStateRead
}

// kbSub is an item's sub line: what it is in its connector's words (its
// kind's declared label), then where it sits in its document.
func kbSub(kinds map[string]api.ItemKind, f documentFields, body anchor.SourceDocBody) string {
	sub := kbKindWords[kbKindOf(f)]
	if k, ok := kinds[f.ItemKind]; ok && k.Label != "" {
		sub = k.Label
	} else if f.ItemKind != "" {
		sub = strings.ToUpper(f.ItemKind[:1]) + f.ItemKind[1:]
	}
	if len(body.SectionPath) > 0 {
		sub += " · " + strings.Join(body.SectionPath, " › ")
	}
	return sub
}

// kbFilter is a knowledge base's filters: each empty for all.
type kbFilter struct{ source, kind, state string }

// with is the filter with one of its parts changed.
func (f kbFilter) with(part, value string) kbFilter {
	switch part {
	case "source":
		f.source = value
	case "kind":
		f.kind = value
	case "state":
		f.state = value
	}
	return f
}

// args is the query that shows the filter: every part, so a query sets them
// all over the panel's input.
func (f kbFilter) args() map[string]any {
	return map[string]any{"source": f.source, "kind": f.kind, "state": f.state}
}

// chip is a count or a filter chip: its value and label, the query it runs
// and whether it is the filter shown (its tone).
func (f kbFilter) chip(value, label string, to kbFilter) map[string]any {
	tone := "off"
	if to == f {
		tone = "on"
	}
	c := map[string]any{"label": label, "tone": tone}
	for k, v := range to.args() {
		c[k] = v
	}
	if value != "" {
		c["value"] = value
	}
	return c
}

// knowledgeBaseTree draws the knowledge base: its heading, the sources to
// update, the counts, the filters, how many are shown and the items - each
// its title and what it is, its source in its connector's colour, its kind
// and state -, an item opening its source-document in a drawer.
func knowledgeBaseTree(empty bool, sourceColors map[string]any) map[string]any {
	query := map[string]any{"label": "Filtrer", "query": map[string]any{"args": map[string]any{"source": "${item.source}", "kind": "${item.kind}", "state": "${item.state}"}}}
	toned := func(n map[string]any) map[string]any {
		n["variant"] = "cards"
		n["toneField"] = "tone"
		n["toneColors"] = map[string]any{"on": "blue", "off": "gray"}
		return n
	}
	head := map[string]any{"kind": "vbox", "gap": 2, "children": []any{
		map[string]any{"kind": "heading", "text": "Base de connaissance", "level": 3},
		map[string]any{"kind": "text", "content": "", "dataField": "$.sub", "size": "sm", "color": "dimmed"},
	}}
	if empty {
		return map[string]any{"kind": "vbox", "id": "knowledge-base", "gap": "md", "children": []any{head,
			map[string]any{"kind": "box", "id": "kb-empty", "callout": "gray", "children": []any{
				map[string]any{"kind": "text", "content": "La base est vide : rien n'est encore lu.", "size": "sm", "fw": "bold"},
				map[string]any{"kind": "text", "content": "Chaque source y verse ce qu'elle lit dès « Enregistrer et lire ». Cochez d'abord ce que les sources proposent.", "size": "sm", "color": "dimmed"},
			}},
		}}
	}
	return map[string]any{"kind": "vbox", "id": "knowledge-base", "gap": "md", "children": []any{
		head,
		map[string]any{"kind": "list", "id": "kb-update", "dataField": "$.update", "gap": "xs", "empty": "",
			"item":    map[string]any{"kind": "text", "content": "", "dataField": "$.item.label", "size": "sm"},
			"actions": []any{map[string]any{"label": "Mettre à jour", "icon": "refresh", "uri": "ktw-agents:sources/source-sync", "args": map[string]any{"id": "${item.source}"}}}},
		toned(map[string]any{"kind": "list", "id": "kb-kpis", "dataField": "$.kpis", "layout": "grid", "columns": 4, "gap": "sm",
			"item": map[string]any{"kind": "vbox", "gap": 0, "children": []any{
				map[string]any{"kind": "text", "content": "", "dataField": "$.item.value", "size": "xl", "fw": "bold"},
				map[string]any{"kind": "text", "content": "", "dataField": "$.item.label", "size": "xs", "color": "dimmed"},
			}},
			"open": query}),
		map[string]any{"kind": "list", "id": "kb-filters", "dataField": "$.filters", "gap": "xs",
			"item": map[string]any{"kind": "hbox", "gap": "sm", "align": "center", "children": []any{
				map[string]any{"kind": "text", "content": "", "dataField": "$.item.group", "size": "xs", "fw": "bold", "color": "dimmed"},
				toned(map[string]any{"kind": "list", "id": "kb-filter", "dataField": "$.item.chips", "layout": "horizontal", "gap": "xs",
					"item": map[string]any{"kind": "text", "content": "", "dataField": "$.item.label", "size": "xs"},
					"open": query}),
			}}},
		map[string]any{"kind": "hbox", "gap": "sm", "align": "center", "children": []any{
			map[string]any{"kind": "text", "content": "", "dataField": "$.showing", "size": "sm", "color": "dimmed"},
			map[string]any{"kind": "list", "id": "kb-show-all", "dataField": "$.show_all", "layout": "horizontal", "empty": "",
				"item": map[string]any{"kind": "text", "content": "Tout montrer", "size": "sm", "color": "blue"},
				"open": query},
		}},
		map[string]any{"kind": "list", "id": "kb-items", "dataField": "$.items", "gap": "xs", "variant": "cards",
			"empty":     "Aucun élément ici : rien ne correspond à ces filtres.",
			"toneField": "state_label", "toneColors": map[string]any{kbStateWords[api.KBStateRead]: "teal", kbStateWords[api.KBStateStale]: "orange"},
			"item": map[string]any{"kind": "grid", "cols": 3, "gap": "sm", "children": []any{
				map[string]any{"kind": "vbox", "gap": 0, "children": []any{
					map[string]any{"kind": "text", "content": "", "dataField": "$.item.title", "fw": "bold", "size": "sm"},
					map[string]any{"kind": "text", "content": "", "dataField": "$.item.sub", "size": "xs", "color": "dimmed"},
				}},
				map[string]any{"kind": "hbox", "gap": "xs", "align": "center", "children": []any{
					map[string]any{"kind": "badge", "text": "", "dataField": "$.item.source_label", "colors": sourceColors, "color": "gray", "keepCase": true},
					map[string]any{"kind": "text", "content": "", "dataField": "$.item.source_name", "size": "sm"},
				}},
				map[string]any{"kind": "hbox", "gap": "xs", "children": []any{
					map[string]any{"kind": "badge", "text": "", "dataField": "$.item.kind_label", "colors": map[string]any{kbKindWords[api.KBKindDocument]: "blue", kbKindWords[api.KBKindRecord]: "grape"}, "color": "gray", "keepCase": true},
					map[string]any{"kind": "badge", "text": "", "dataField": "$.item.state_label", "colors": map[string]any{kbStateWords[api.KBStateRead]: "teal", kbStateWords[api.KBStateStale]: "orange"}, "color": "gray", "keepCase": true},
				}},
			}},
			"open": map[string]any{"label": "Ouvrir", "drawer": map[string]any{"object": "${item.id}", "panel": "source-document"}}},
		map[string]any{"kind": "text", "content": "Chaque élément vit une fois, dans la source qui l'a lu : changer ce que prend une source change cette liste.", "size": "sm", "color": "dimmed"},
	}}
}
