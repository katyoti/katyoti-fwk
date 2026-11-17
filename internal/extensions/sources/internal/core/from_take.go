// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// followsBrowse says whether a source's take tab is a from-connector's
// (mock-up v6's Dynatrace source): its connector reads from
// another source's kept items and browses them, in a list.
func followsBrowse(c api.ConnectorDecl) bool {
	return c.From != nil && c.Browse != "" && pickerView(c).Layout != "tree"
}

// fromTake is an opened source's take tab for a connector reading from
// another source as mock-up v6 draws it: the head (the take
// label, what the source takes), the « Lien entre sources » band (the other
// source, what it gives, how this connector matches it, this source), then
// the proposal region in groups - one per value the other source kept, its
// items browsed below it - with the save bar; « Rien à proposer pour
// l'instant » and « Ouvrir la source … » while nothing is kept. A value
// typed by hand is the picker's search (searchTyped). A browse's error shows
// its first line, the rest under « détails ». Nothing here names a connector.
func (s *Service) fromTake(ctx context.Context, cwm cwmapi.CWMService, c api.ConnectorDecl, module, source, name string, chosen []any) (api.ConnectorPanelOutput, error) {
	view := pickerView(c)
	unit := strings.TrimSpace(view.Unit)
	if unit == "" {
		unit = "élément"
	}
	fem := c.Picker.Feminine()
	carrier := carrierOfSource(ctx, cwm, source)
	from, err := fromItems(ctx, cwm, carrier, c)
	if err != nil {
		return api.ConnectorPanelOutput{}, err
	}
	if from == nil {
		from = &api.FromItems{Connector: c.From.First(), Attr: c.From.Attr}
	}
	// The other source is the carrier's own source of a connector it
	// follows - fromItems names the first one found.
	fcName := from.Connector
	fc, _ := s.connector(ctx, fcName)
	fromSys := systemOf(fc, fcName)
	fromUnit := strings.TrimSpace(pickerView(fc).Unit)
	if fromUnit == "" {
		fromUnit = "élément"
	}
	fromFem := fc.Picker.Feminine()
	fromID, fromName := fromSource(ctx, cwm, carrier, fcName)
	// The values the other source kept at this source's last save
	// : a group of a value kept since is new here. A source
	// never saved marks nothing.
	seenAtSave := map[string]bool{}
	if f, _, err := sourceFields(ctx, cwm, source); err == nil {
		for _, v := range f.FromSeen {
			seenAtSave[v] = true
		}
	}

	inChoice := map[string]bool{}
	for _, x := range chosen {
		if m, ok := x.(map[string]any); ok {
			inChoice[fmt.Sprint(m["key"])] = true
		}
	}
	selected := []any{}
	seen := map[string]bool{}
	groups := []any{}
	var chips, problems []string
	match := ""
	if len(from.Values) > 0 {
		top, err := s.SourceBrowse(ctx, api.SourceBrowseInput{ID: source})
		if err != nil {
			problems = append(problems, err.Error())
		}
		match = strings.TrimSpace(top.Match)
		for i, n := range top.Items {
			if i >= api.ProposeMax {
				break
			}
			code := n.Attrs[from.Attr]
			if code == "" {
				code = n.Title
			}
			g := map[string]any{"key": n.Key, "title": from.Title(code), "code": code}
			// Each value's own system: a value another
			// followed source kept carries that source's mark.
			if vName := from.ConnectorOf(code); vName != "" && vName != fcName {
				vc, _ := s.connector(ctx, vName)
				g["badge"] = map[string]any{"label": markOf(vc, vName), "color": vc.Color}
			} else if fromSys != "" {
				g["badge"] = map[string]any{"label": markOf(fc, fcName), "color": fc.Color}
			}
			if n.Detail != "" {
				g["chip"] = n.Detail
				chips = append(chips, n.Detail)
				g["empty"] = strings.TrimSpace(fmt.Sprintf("%s %s ne porte %s%s.", agree(fem, "Aucun", "Aucune"), unit, n.Detail, inSystem(c)))
			} else {
				g["empty"] = fmt.Sprintf("%s %s ici.", agree(fem, "Aucun", "Aucune"), unit)
			}
			added := len(seenAtSave) > 0 && !seenAtSave[code]
			if added {
				g["badges"] = []any{badgeFromAdded}
			}
			groups = append(groups, g)
			if !n.HasChildren {
				continue
			}
			kids, err := s.SourceBrowse(ctx, api.SourceBrowseInput{ID: source, Parent: n.Key})
			if err != nil {
				problems = append(problems, err.Error())
				continue
			}
			for _, k := range kids.Items {
				if seen[k.Key] || strings.TrimSpace(k.Key) == "" {
					continue
				}
				seen[k.Key] = true
				row := browsedRow(k)
				row["group"] = n.Key
				if !inChoice[k.Key] {
					row["off"] = true
					if added {
						row["badges"] = append(rowBadges(row), badgeFromNew)
					}
				}
				selected = append(selected, row)
			}
		}
	}
	for _, x := range chosen {
		m, ok := x.(map[string]any)
		if !ok || seen[fmt.Sprint(m["key"])] {
			continue
		}
		selected = append(selected, chosenAside(m))
	}

	take := strings.TrimSpace(c.Take)
	if take == "" {
		take = api.SourceTakeDefault
	}
	children := []any{
		map[string]any{"kind": "vbox", "gap": 0, "id": "source-take-head", "children": []any{
			map[string]any{"kind": "text", "content": take, "size": "lg", "fw": "bold"},
			map[string]any{"kind": "text", "content": fmt.Sprintf("ce que « %s » prend%s · cochez, puis enregistrez", name, inSystem(c)), "size": "sm", "color": "dimmed"},
		}},
		linkBand(c, fc, from, fromSys, fromUnit, fromFem, fromID, fromName, unit, name, match, chips),
	}
	if len(problems) > 0 {
		children = append(children, takeProblem(problems))
	}
	if len(from.Values) == 0 {
		empty := []any{
			map[string]any{"kind": "text", "content": "Rien à proposer pour l'instant.", "fw": "bold"},
			map[string]any{"kind": "text", "size": "sm", "color": "dimmed",
				"content": fmt.Sprintf("Cette source propose les %s des %s %s dans la source %s : il n'y en a pas encore.",
					unitPlural(unit), unitPlural(fromUnit), agree(fromFem, "cochés", "cochées"), fromSys)},
		}
		if fromID != "" {
			label := "Ouvrir la source " + fromSys
			empty = append(empty, map[string]any{"kind": "button", "id": "source-take-open-from", "label": label, "size": "xs",
				"action": map[string]any{"label": label, "navigate": map[string]any{"object": fromID}}})
		}
		children = append(children, render.Card(render.Node{"kind": "vbox", "gap": "xs", "children": empty}).With("id", "source-take-empty"))
		return api.ConnectorPanelOutput{DSLTree: map[string]any{"kind": "vbox", "gap": "md", "children": children},
			Data: map[string]any{"selected": selected}, Title: name}, nil
	}

	chosenOne, chosenMany := agree(fem, "choisi", "choisie"), agree(fem, "choisis", "choisies")
	picker := pickerNode(c, module)
	picker["badges"] = fromBadges(view.Badges, fromSys, fromFem, fem)
	picker["tickable"] = true
	picker["search"] = api.SearchURI
	picker["hint"] = fmt.Sprintf("Tapez le %s puis Entrée.", from.Attr)
	picker["proposals"] = map[string]any{
		"title": fmt.Sprintf("Proposé d'après les %s de la source %s (%s)", plural(len(from.Values), fromUnit), fromSys, from.Attr),
		"sub": map[string]any{"kind": "text", "size": "xs", "color": "dimmed",
			"content": fmt.Sprintf("chaque %s donne son %s ; %s renvoie les %s qui %s. Les mots-clés de l'étude ne servent pas ici.",
				fromUnit, from.Attr, systemOf(c, c.Name), unitPlural(unit), matchVerb(match))},
		"groups": groups,
		"none":   fmt.Sprintf("%s %s à proposer.", agree(fem, "Aucun", "Aucune"), unit),
	}
	picker["savebar"] = map[string]any{"one": unit + " " + chosenOne, "many": unitPlural(unit) + " " + chosenMany,
		"idle": s.readLine(ctx, cwm, source), "empty": "Rien de coché : cochez des propositions"}
	picker["labels"] = map[string]any{"apply": "Enregistrer et lire", "empty": "Rien de sélectionné. Cochez des propositions.",
		"search": fmt.Sprintf("Ajouter %s %s par son %s", agree(fromFem, "un", "une"), fromUnit, from.Attr)}
	children = append(children, picker)
	return api.ConnectorPanelOutput{DSLTree: map[string]any{"kind": "vbox", "gap": "md", "children": children},
		Data: map[string]any{"selected": selected}, Title: name}, nil
}

// The badges a from-connector's take tab adds (mock-up v6
// dtGroup): on the head of a group whose value the other source kept since
// this source's last save, and on that group's unticked rows.
const (
	badgeFromAdded = "from-added"
	badgeFromNew   = "from-new"
)

// fromBadges are the picker's badges with the two a from-connector's take tab
// adds: « ajouté(e) dans <System> » agreeing with the other source's unit,
// « nouveau/nouvelle » with this one's - drawn only while its row is
// unticked.
func fromBadges(own map[string]api.PickerBadge, fromSys string, fromFem, fem bool) map[string]api.PickerBadge {
	out := make(map[string]api.PickerBadge, len(own)+2)
	for k, v := range own {
		out[k] = v
	}
	out[badgeFromAdded] = api.PickerBadge{Label: agree(fromFem, "ajouté", "ajoutée") + " dans " + fromSys, Color: "orange",
		Description: agree(fromFem, "Coché", "Cochée") + " dans la source " + fromSys + " depuis le dernier enregistrement de cette source"}
	out[badgeFromNew] = api.PickerBadge{Label: agree(fem, "nouveau", "nouvelle"), Color: "blue", Unticked: true,
		Description: agree(fem, "Proposé", "Proposée") + " depuis le dernier enregistrement de cette source"}
	return out
}

// rowBadges are the badges a proposed row already carries.
func rowBadges(row map[string]any) []any {
	var out []any
	switch b := row["badges"].(type) {
	case []any:
		out = append(out, b...)
	case []string:
		for _, x := range b {
			out = append(out, x)
		}
	}
	return out
}

// matchVerb is what the proposal's sub-line says the items do: « portent
// <match> » when the connector says how it matches, else « en dépendent ».
func matchVerb(match string) string {
	if match != "" {
		return "portent la " + match
	}
	return "en dépendent"
}

// linkBand is mock-up v6's « Lien entre sources » (linkBand): the source it
// reads from (its system's mark as a badge, its name - a link opening it -, how many
// values it kept and which), the arrow « donne le <attr> » over how the
// connector matches them, this source with what it searches for (« rien à
// chercher » before anything is kept), and why it reads so.
func linkBand(c, fc api.ConnectorDecl, from *api.FromItems, fromSys, fromUnit string, fromFem bool, fromID, fromName, unit, name, match string, chips []string) map[string]any {
	badge := func(label, color string) map[string]any {
		return map[string]any{"kind": "badge", "text": label, "variant": "filled", "color": color}
	}
	fromTitle, fromLine := "Aucune source "+fromSys, "à ajouter"
	if fromID != "" {
		fromTitle = fromName
		values := strings.Join(from.Values, ", ")
		if values == "" {
			values = "—"
		}
		fromLine = plural(len(from.Values), fromUnit) + " · " + values
	}
	fromNode := map[string]any{"kind": "hbox", "gap": "xs", "id": "source-take-link-from", "children": []any{
		badge(markOf(fc, from.Connector), fc.Color),
		map[string]any{"kind": "vbox", "gap": 0, "children": []any{
			map[string]any{"kind": "text", "content": fromTitle, "size": "sm", "fw": "bold"},
			map[string]any{"kind": "text", "content": fromLine, "size": "xs", "color": "dimmed"},
		}},
	}}
	if fromID != "" {
		// Its name opens it (mock-up v6: the node is a button).
		fromNode["children"].([]any)[1].(map[string]any)["children"].([]any)[0] = map[string]any{"kind": "link", "text": fromTitle,
			"action": map[string]any{"label": "Ouvrir " + fromName, "navigate": map[string]any{"object": fromID}}}
	}
	arrow := []any{map[string]any{"kind": "text", "content": "donne le " + from.Attr, "size": "xs"},
		map[string]any{"kind": "text", "content": "→", "size": "sm", "color": "dimmed"}}
	if match != "" {
		arrow = append(arrow, map[string]any{"kind": "text", "content": match, "size": "xs", "color": "dimmed"})
	}
	searched := strings.Join(chips, " · ")
	if searched == "" {
		searched = "rien à chercher"
	}
	here := map[string]any{"kind": "hbox", "gap": "xs", "id": "source-take-link-here", "children": []any{
		badge(markOf(c, c.Name), c.Color),
		map[string]any{"kind": "vbox", "gap": 0, "children": []any{
			map[string]any{"kind": "text", "content": name, "size": "sm", "fw": "bold"},
			map[string]any{"kind": "text", "content": searched, "size": "xs", "color": "dimmed"},
		}},
	}}
	why := fmt.Sprintf("Cette source lit depuis la source %s, par les %ss des %s, jamais par les mots-clés de l'étude (un %s est exact, un mot-clé ne l'est pas). %s %s %s là-bas propose ici ses %s ; %s %s %s retire les %s.",
		fromSys, from.Attr, unitPlural(fromUnit), from.Attr,
		agree(fromFem, "Un", "Une"), fromUnit, agree(fromFem, "coché", "cochée"), unitPlural(unit),
		agree(fromFem, "un", "une"), fromUnit, agree(fromFem, "retiré", "retirée"), agree(fromFem, "siens", "siennes"))
	return render.Card(render.Node{"kind": "vbox", "gap": "sm", "children": []any{
		map[string]any{"kind": "hbox", "gap": "md", "align": "center", "children": []any{
			fromNode,
			map[string]any{"kind": "vbox", "gap": 0, "align": "center", "children": arrow},
			here,
		}},
		map[string]any{"kind": "text", "content": why, "size": "xs", "color": "dimmed"},
	}}).With("id", "source-take-link").With("label", "Lien entre sources")
}

// takeProblem is what a take tab met: each problem's first line in red, the
// rest of them under « détails ».
func takeProblem(problems []string) map[string]any {
	var heads, rest []string
	for _, p := range problems {
		head, more, _ := strings.Cut(strings.TrimSpace(p), "\n")
		heads = append(heads, strings.TrimSpace(head))
		if more = strings.TrimSpace(more); more != "" {
			rest = append(rest, more)
		}
	}
	children := []any{map[string]any{"kind": "text", "size": "sm", "color": "red", "content": strings.Join(heads, " ; ")}}
	if len(rest) > 0 {
		children = append(children, render.Accordion([]render.Fold{{ID: "source-take-problem-details", Label: "détails",
			Content: render.Node{"kind": "text", "size": "xs", "content": strings.Join(rest, "\n\n")}}}))
	}
	return map[string]any{"kind": "vbox", "gap": "xs", "id": "source-take-problem", "children": children}
}

// browsedRow is a browsed item as the proposal region draws it.
func browsedRow(it api.BrowseItem) map[string]any {
	return proposedRow(api.ProposedItem{PickItem: api.PickItem{Key: it.Key, Title: it.Title, Kind: it.Kind, Badges: it.Badges,
		Detail: it.Detail, Attrs: it.Attrs, Path: it.Path}})
}

// fromSource is the carrier's own source of a connector - its id and name -
// the source a from-connector's source reads from; none when it has none.
func fromSource(ctx context.Context, cwm cwmapi.CWMService, carrier, connector string) (string, string) {
	if carrier == "" {
		return "", ""
	}
	srcs, err := sourcesOf(ctx, cwm, carrier)
	if err != nil {
		return "", ""
	}
	for _, src := range srcs {
		if src.Via == "child" && src.Connector == connector {
			return src.ID, src.Name
		}
	}
	return "", ""
}

// searchTyped is a from-connector's search: the query is a
// value of the source it reads from typed by hand (an application's code):
// the connector's browse lists that value's node in place of what the other
// source kept, then the items below it - the same two-level browse as the
// take tab's groups, so sources never forms a connector's key.
func (s *Service) searchTyped(ctx context.Context, c api.ConnectorDecl, in api.SearchInput) (api.ConnectorSearchOutput, error) {
	out := api.ConnectorSearchOutput{Items: []api.PickItem{}}
	q := strings.TrimSpace(in.Query)
	bin := api.SourceBrowseInput{ID: in.Source}
	if in.Source == "" {
		bin = api.SourceBrowseInput{Connector: c.Name, Object: in.Object}
	}
	top, err := s.sourceBrowse(ctx, bin, q)
	if err != nil {
		return out, err
	}
	for _, n := range top.Items {
		if !n.HasChildren {
			continue
		}
		below := bin
		below.Parent = n.Key
		kids, err := s.sourceBrowse(ctx, below, q)
		if err != nil {
			return out, err
		}
		for _, k := range kids.Items {
			out.Items = append(out.Items, api.PickItem{Key: k.Key, Title: k.Title, Kind: k.Kind, URI: k.URI, Badges: k.Badges,
				Parent: n.Key, Path: []string{n.Title}, Detail: k.Detail, Attrs: k.Attrs})
		}
	}
	return out, nil
}

// fromNews is what a from-connector source's card says when the other
// source kept values since this source's last save (mock-up
// v6 srcState): how many items those values propose that this source does
// not keep yet, and the titles of the values they come from. A source never
// saved, or one the values it follows did not change for, has none.
func (s *Service) fromNews(ctx context.Context, cwm cwmapi.CWMService, src api.SourceSummary) (int, []string) {
	f, _, err := sourceFields(ctx, cwm, src.ID)
	if err != nil {
		return 0, nil
	}
	added := src.From.AddedSince(f.FromSeen)
	if len(added) == 0 {
		return 0, nil
	}
	isNew := make(map[string]bool, len(added))
	for _, v := range added {
		isNew[v] = true
	}
	kept := map[string]bool{}
	for _, k := range src.Kept {
		kept[k.Key] = true
	}
	top, err := s.SourceBrowse(ctx, api.SourceBrowseInput{ID: src.ID})
	if err != nil {
		return 0, nil
	}
	n, seen := 0, map[string]bool{}
	var titles []string
	for _, g := range top.Items {
		code := g.Attrs[src.From.Attr]
		if code == "" {
			code = g.Title
		}
		if !isNew[code] || !g.HasChildren {
			continue
		}
		kids, err := s.SourceBrowse(ctx, api.SourceBrowseInput{ID: src.ID, Parent: g.Key})
		if err != nil {
			continue
		}
		found := 0
		for _, k := range kids.Items {
			if k.Key == "" || seen[k.Key] || kept[k.Key] {
				continue
			}
			seen[k.Key] = true
			found++
		}
		if found > 0 {
			n += found
			titles = append(titles, src.From.Title(code))
		}
	}
	return n, titles
}

// fromNewsCard turns a card into mock-up v6's « n propositions » (blue,
// waiting on the person) with « k <unit>s · n nouvelles d'après <titles>,
// ajoutée(s) dans <System> » - the words agreeing with each connector's unit.
func fromNewsCard(u *api.ConnectorUse, c, fc api.ConnectorDecl, kept, n int, titles []string) {
	unit := strings.TrimSpace(pickerView(c).Unit)
	if unit == "" {
		unit = "élément"
	}
	fem := c.Picker.Feminine()
	fresh := agree(fem, "nouveau", "nouvelle")
	if n > 1 {
		fresh += "x"
		if fem {
			fresh = "nouvelles"
		}
	}
	addedWord := agree(fc.Picker.Feminine(), "ajouté", "ajoutée")
	if len(titles) > 1 {
		addedWord += "s"
	}
	u.State = &api.CardState{Text: plural(n, "proposition"), Color: "blue", Kind: api.CardNews}
	u.Meta = fmt.Sprintf("%s · %d %s d'après %s, %s dans %s", plural(kept, unit), n, fresh, strings.Join(titles, ", "), addedWord, systemOf(fc, fc.Name))
}
