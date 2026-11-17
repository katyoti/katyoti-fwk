// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/internal/anchor"
)

// extractRunes bounds a document's « Extrait »: the start of its text.
const extractRunes = 600

// DocumentPage is a source-document opened as a knowledge-base item
// (mock-up v6 view 17): « Base de connaissance · élément »
// its title, its kind and state and what it is; « Vient de », the source it
// came from as a card opening it - its connector, its name, its connector,
// id and when it was read; for an item found through another source (its
// connector declares from) how the link found it; a record's fields as
// rows (« Champs »), a document's start (« Extrait ») with the stale note
// when its source changed it since it was read; the objects reading it;
// « Ouvrir dans <système> » (the item's address in its system) and
// « Ouvrir la source ». What it draws is DocumentDetail's answer - the
// source's fetch, else what is stored -, the same for every connector.
func (s *Service) DocumentPage(ctx context.Context, in api.DocumentDetailInput) (render.Def, error) {
	detail, err := s.DocumentDetail(ctx, in)
	if err != nil {
		return render.Def{}, err
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return render.Def{}, fmt.Errorf("sources: resolve CWMService: %w", err)
	}
	module, err := cwmapi.ModuleOfObjectID(in.ID)
	if err != nil {
		return render.Def{}, err
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: in.ID})
	if err != nil {
		return render.Def{}, err
	}
	doc := got.Object
	var f documentFields
	_ = json.Unmarshal(doc.FieldsJSON(), &f)
	body, _ := anchor.DecodeBody(doc.FieldsJSON())

	kind, state := kbKindOf(f), kbStateOf(body)
	if len(detail.Data) > 0 {
		kind = api.KBKindRecord
	}
	title := strings.TrimSpace(detail.Item.Title)
	if title == "" {
		title = doc.Name
	}
	if title == "" {
		title = localID(doc.ID)
	}
	data := map[string]any{"title": title, "kind_label": kbKindWords[kind], "state_label": kbStateWords[state],
		"note": detail.Note, "fields": []any{}, "extract": "", "stale_note": "", "found_by": "",
		"source": []any{}, "open_url": "", "open_label": "", "used": []any{}}

	// The source it came from: its card, how a link found it, its system.
	var conn api.ConnectorDecl
	system := ""
	srcID := ""
	if doc.Parent != "" {
		if sf, _, err := sourceFields(ctx, cwm, doc.Parent); err == nil {
			srcID = doc.Parent
			conn, _ = s.connector(ctx, sf.Connector)
			label := conn.Label
			if label == "" {
				label = sf.Connector
			}
			system = strings.TrimSpace(conn.System)
			if system == "" {
				system = label
			}
			name := ""
			if o, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: doc.Parent}); err == nil {
				name = o.Object.Name
			}
			if name == "" {
				name = localID(doc.Parent)
			}
			line := label + " · " + doc.Parent
			if ago := agoFR(sf.LastSynced, time.Now()); ago != "" {
				line += " · lu " + ago
			}
			color := conn.Color
			if color == "" {
				color = "gray"
			}
			data["source"] = []any{map[string]any{"id": doc.Parent, "name": name, "label": label, "mark": system, "color": color, "line": line}}
			data["found_by"] = s.foundBy(ctx, conn, system)
		}
	}
	data["sub"] = kbSub(conn.Kinds, f, body)
	if system == "" {
		system = "son système"
	}

	if kind == api.KBKindRecord {
		data["fields"] = fieldRows(detail.Data)
	} else {
		data["extract"] = extractOf(detail.Text)
		if state == api.KBStateStale {
			data["stale_note"] = "Modifié dans " + system + " depuis la dernière lecture : « Mettre à jour » sa source le relit."
		}
	}
	if u := strings.TrimSpace(detail.Item.URI); strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://") {
		data["open_url"], data["open_label"] = u, "Ouvrir dans "+system
	}
	var used []any
	for _, u := range detail.Used {
		name := u.Name
		if name == "" {
			name = u.ID
		}
		used = append(used, map[string]any{"id": u.ID, "name": name})
	}
	if used != nil {
		data["used"] = used
	}
	return render.Def{Kind: "page", Title: title, DSLTree: documentPageTree(data, srcID, kind), Data: data}, nil
}

// foundBy is how an item of a connector reading from another source was
// found (« Trouvé par le lien : … »): the other source's item gives its
// attribute, looked for in this one's system; empty for a connector reading
// from none.
func (s *Service) foundBy(ctx context.Context, c api.ConnectorDecl, system string) string {
	name := c.From.First()
	if name == "" {
		return ""
	}
	other, err := s.connector(ctx, name)
	if err != nil {
		other = api.ConnectorDecl{Name: name}
	}
	otherSys := strings.TrimSpace(other.System)
	if otherSys == "" {
		otherSys = other.Label
	}
	if otherSys == "" {
		otherSys = other.Name
	}
	unit, fem := "élément", false
	if other.Picker != nil {
		if u := strings.TrimSpace(other.Picker.Unit); u != "" {
			unit = u
		}
		fem = other.Picker.Gender == "feminine"
	}
	article := agree(fem, "le ", "la ")
	if strings.ContainsAny(unit[:1], "aeiouhéèêAEIOUHÉ") {
		article = "l'"
	}
	return fmt.Sprintf("Trouvé par le lien : %s%s de la source %s donne son %s, cherché dans %s.", article, unit, otherSys, attrLabel(other, c.From.Attr), system)
}

// fieldRows are a record's fields as rows, by name: each value a line of
// text - a list's values joined, an object written whole -, empty ones left
// out.
func fieldRows(data map[string]any) []any {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	rows := []any{}
	for _, k := range keys {
		v := fieldText(data[k])
		if v == "" {
			continue
		}
		rows = append(rows, map[string]any{"key": k, "value": v})
	}
	return rows
}

// fieldText is a field's value as one line.
func fieldText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(x)
	case bool:
		return agree(x, "non", "oui")
	case []any:
		var parts []string
		for _, e := range x {
			if t := fieldText(e); t != "" {
				parts = append(parts, t)
			}
		}
		return strings.Join(parts, ", ")
	case map[string]any:
		raw, _ := json.Marshal(x)
		return string(raw)
	default:
		return fmt.Sprint(x)
	}
}

var (
	mdImage = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	mdLink  = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	mdMarks = regexp.MustCompile("(?m)^\\s*(#+|[-*>]|\\d+\\.)\\s+|[*_`|]")
	spaces  = regexp.MustCompile(`\s+`)
)

// extractOf is the start of a document's text as plain words: its images,
// link targets and markdown marks dropped, cut at extractRunes.
func extractOf(text string) string {
	t := mdImage.ReplaceAllString(text, "")
	t = mdLink.ReplaceAllString(t, "$1")
	t = mdMarks.ReplaceAllString(t, "")
	t = strings.TrimSpace(spaces.ReplaceAllString(t, " "))
	if r := []rune(t); len(r) > extractRunes {
		t = strings.TrimSpace(string(r[:extractRunes])) + "…"
	}
	return t
}

// documentPageTree draws an opened item as mock-up v6 view 17's drawer.
func documentPageTree(data map[string]any, source, kind string) map[string]any {
	kindColors := map[string]any{kbKindWords[api.KBKindDocument]: "blue", kbKindWords[api.KBKindRecord]: "grape"}
	stateColors := map[string]any{kbStateWords[api.KBStateRead]: "teal", kbStateWords[api.KBStateStale]: "orange"}
	children := []any{
		map[string]any{"kind": "vbox", "id": "source-document-head", "gap": 4, "children": []any{
			map[string]any{"kind": "text", "content": "Base de connaissance · élément", "size": "xs", "color": "dimmed"},
			map[string]any{"kind": "heading", "text": "", "dataField": "$.title", "level": 3},
			map[string]any{"kind": "hbox", "gap": "xs", "align": "center", "children": []any{
				map[string]any{"kind": "badge", "text": "", "dataField": "$.kind_label", "colors": kindColors, "color": "gray", "keepCase": true},
				map[string]any{"kind": "badge", "text": "", "dataField": "$.state_label", "colors": stateColors, "color": "gray", "keepCase": true},
				map[string]any{"kind": "text", "content": "", "dataField": "$.sub", "size": "xs", "color": "dimmed"},
			}},
		}},
	}
	if s, _ := data["note"].(string); s != "" {
		children = append(children, map[string]any{"kind": "text", "content": "", "dataField": "$.note", "size": "xs", "color": "yellow"})
	}
	if source != "" {
		src := data["source"].([]any)[0].(map[string]any)
		children = append(children, map[string]any{"kind": "vbox", "gap": 4, "children": []any{
			map[string]any{"kind": "text", "content": "Vient de", "size": "xs", "fw": "bold", "color": "dimmed"},
			map[string]any{"kind": "list", "id": "source-document-source", "dataField": "$.source", "variant": "cards", "gap": "xs",
				"item": map[string]any{"kind": "hbox", "gap": "sm", "align": "center", "children": []any{
					map[string]any{"kind": "badge", "text": "", "dataField": "$.item.mark", "colors": map[string]any{src["mark"].(string): src["color"]}, "color": "gray", "keepCase": true},
					map[string]any{"kind": "vbox", "gap": 0, "children": []any{
						map[string]any{"kind": "text", "content": "", "dataField": "$.item.name", "size": "sm", "fw": "bold"},
						map[string]any{"kind": "text", "content": "", "dataField": "$.item.line", "size": "xs", "color": "dimmed"},
					}},
				}},
				"open": map[string]any{"label": "Ouvrir la source", "navigate": map[string]any{"object": "${item.id}"}}},
		}})
		if s, _ := data["found_by"].(string); s != "" {
			children = append(children, map[string]any{"kind": "box", "id": "source-document-found-by", "callout": "gray", "children": []any{
				map[string]any{"kind": "text", "content": "", "dataField": "$.found_by", "size": "sm"},
			}})
		}
	}
	if kind == api.KBKindRecord {
		children = append(children, render.Card(render.Node{"kind": "vbox", "gap": "xs", "children": []any{
			map[string]any{"kind": "text", "content": "Champs", "size": "xs", "fw": "bold", "color": "dimmed"},
			map[string]any{"kind": "list", "id": "source-document-fields", "dataField": "$.fields", "gap": 2, "empty": "Aucun champ gardé.",
				"item": map[string]any{"kind": "grid", "cols": 2, "gap": "sm", "children": []any{
					map[string]any{"kind": "text", "content": "", "dataField": "$.item.key", "size": "sm", "color": "dimmed"},
					map[string]any{"kind": "text", "content": "", "dataField": "$.item.value", "size": "sm"},
				}}},
		}}).With("id", "source-document-record"))
	} else {
		extract := []any{
			map[string]any{"kind": "text", "content": "Extrait", "size": "xs", "fw": "bold", "color": "dimmed"},
		}
		if s, _ := data["extract"].(string); s != "" {
			extract = append(extract, map[string]any{"kind": "text", "content": "", "dataField": "$.extract", "size": "sm"})
		} else {
			extract = append(extract, map[string]any{"kind": "text", "content": "Aucun texte gardé.", "size": "sm", "color": "dimmed"})
		}
		if s, _ := data["stale_note"].(string); s != "" {
			extract = append(extract, map[string]any{"kind": "box", "id": "source-document-stale", "callout": "orange", "children": []any{
				map[string]any{"kind": "text", "content": "", "dataField": "$.stale_note", "size": "sm"},
			}})
		}
		children = append(children, render.Card(render.Node{"kind": "vbox", "gap": "xs", "children": extract}).With("id", "source-document-extract"))
	}
	if used, _ := data["used"].([]any); len(used) > 0 {
		children = append(children, map[string]any{"kind": "vbox", "gap": 4, "children": []any{
			map[string]any{"kind": "text", "content": "Lu par", "size": "xs", "fw": "bold", "color": "dimmed"},
			map[string]any{"kind": "list", "id": "source-document-used", "dataField": "$.used", "layout": "horizontal", "gap": "xs",
				"item": map[string]any{"kind": "badge", "text": "", "dataField": "$.item.name", "color": "gray", "keepCase": true},
				"open": map[string]any{"label": "Ouvrir", "navigate": map[string]any{"object": "${item.id}"}}},
		}})
	}
	var foot []any
	if u, _ := data["open_url"].(string); u != "" {
		foot = append(foot, map[string]any{"kind": "link", "id": "source-document-open-system", "text": data["open_label"], "href": u, "external": true, "size": "sm"})
	}
	if source != "" {
		foot = append(foot, map[string]any{"kind": "button", "id": "source-document-open-source", "label": "Ouvrir la source", "size": "xs", "variant": "default",
			"action": map[string]any{"label": "Ouvrir la source", "navigate": map[string]any{"object": source}}})
	}
	if len(foot) > 0 {
		children = append(children, map[string]any{"kind": "hbox", "id": "source-document-foot", "gap": "sm", "align": "center", "justify": "space-between", "children": foot})
	}
	return map[string]any{"kind": "vbox", "id": "source-document", "gap": "md", "children": children}
}
