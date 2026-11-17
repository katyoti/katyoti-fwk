// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// proposes says whether a source's take tab opens on the proposal region
// (mock-up v6's take tab): its connector proposes from the
// holder's keywords - a propose method, no other source it reads from - and
// lists its items (a tree keeps its check-box tree).
func proposes(c api.ConnectorDecl) bool {
	return c.Propose != "" && c.From == nil && pickerView(c).Layout != "tree"
}

// chosenAside is an item of the source's choice that the take tab no longer
// proposes, as the picker's selected carries it: a copy flagged proposed
// false, so the proposals region draws it only as a chip, gone when unticked,
// never as a proposal row (mock-up v6). The choice itself is left untouched.
func chosenAside(m map[string]any) map[string]any {
	row := make(map[string]any, len(m)+1)
	for k, v := range m {
		row[k] = v
	}
	row["proposed"] = false
	return row
}

// proposalTake is an opened source's take tab as mock-up v6 draws it, for any
// connector that proposes: a head (the take label, what the source takes),
// the tickable picker in proposals mode - « Proposé d'après vos mots-clés »
// with the holder's keywords as chips and « Modifier les mots-clés », one row
// per proposed item with its why, slots, detail and declared badges, the
// search for another, the chosen ones as chips - and the save bar « N
// choisi(e)s », « Annuler », « Enregistrer et lire » (sources/hold with
// read). Proposals not in the source's choice come unticked; the items chosen
// that are no longer proposed follow them, ticked and flagged proposed false
// (chosenAside). The head names the system the connector declares, the search
// its first slot's attribute, and the words agree with the unit's declared
// gender. Nothing here names a connector.
func (s *Service) proposalTake(ctx context.Context, cwm cwmapi.CWMService, c api.ConnectorDecl, module, source, name string, chosen []any) (api.ConnectorPanelOutput, error) {
	view := pickerView(c)
	unit := strings.TrimSpace(view.Unit)
	if unit == "" {
		unit = "élément"
	}
	fem := c.Picker.Feminine()
	holder := sourceHolder(ctx, cwm, source)
	kws := holderKeywords(ctx, cwm, holder)

	inChoice := map[string]bool{}
	for _, x := range chosen {
		if m, ok := x.(map[string]any); ok {
			inChoice[fmt.Sprint(m["key"])] = true
		}
	}
	selected := []any{}
	seen := map[string]bool{}
	var problems []string
	if len(kws) > 0 {
		got, err := s.Propose(ctx, api.ProposeInput{Connector: c.Name, Object: module, Terms: kws})
		if err != nil {
			problems = append(problems, err.Error())
		}
		problems = append(problems, got.Errors...)
		for _, it := range got.Items {
			if seen[it.Key] || strings.TrimSpace(it.Key) == "" {
				continue
			}
			seen[it.Key] = true
			row := proposedRow(it)
			if !inChoice[it.Key] {
				row["off"] = true
			}
			selected = append(selected, row)
		}
	}
	for _, x := range chosen {
		m, ok := x.(map[string]any)
		if !ok || seen[fmt.Sprint(m["key"])] {
			continue
		}
		selected = append(selected, chosenAside(m))
	}

	// The keywords the proposal searched by, and where they are changed:
	// they are the holder's.
	sub := []any{}
	for _, kw := range kws {
		sub = append(sub, map[string]any{"kind": "badge", "text": kw, "variant": "light", "color": "gray"})
	}
	if holder != "" {
		sub = append(sub, map[string]any{"kind": "button", "id": "source-take-keywords", "label": "Modifier les mots-clés", "size": "xs", "variant": "subtle",
			"action": map[string]any{"label": "Modifier les mots-clés", "navigate": map[string]any{"object": holder}}})
	}
	none := fmt.Sprintf("%s %s ne correspond aux mots-clés.", agree(fem, "Aucun", "Aucune"), unit)
	if len(kws) == 0 {
		none = "Aucun mot-clé : rien à proposer."
	}
	search := fmt.Sprintf("Chercher %s %s", agree(fem, "un autre", "une autre"), unit)
	if k, ok := view.Kinds[unit]; ok && len(k.Slots) > 0 && strings.TrimSpace(k.Slots[0].Attr) != "" {
		search += " (nom ou " + strings.TrimSpace(k.Slots[0].Attr) + ")"
	}
	chosenOne, chosenMany := agree(fem, "choisi", "choisie"), agree(fem, "choisis", "choisies")

	picker := pickerNode(c, module)
	picker["tickable"] = true
	picker["hint"] = fmt.Sprintf("Tapez pour chercher parmi les %s, puis Entrée.", unitPlural(unit))
	picker["proposals"] = map[string]any{"title": "Proposé d'après vos mots-clés",
		"sub": map[string]any{"kind": "hbox", "gap": "xs", "children": sub}, "none": none}
	picker["savebar"] = map[string]any{"one": unit + " " + chosenOne, "many": unitPlural(unit) + " " + chosenMany,
		"idle": s.readLine(ctx, cwm, source), "empty": "Rien de coché : cochez des propositions"}
	picker["labels"] = map[string]any{"apply": "Enregistrer et lire", "empty": "Rien de sélectionné. Cherchez et ajoutez.", "search": search}

	take := strings.TrimSpace(c.Take)
	if take == "" {
		take = api.SourceTakeDefault
	}
	children := []any{map[string]any{"kind": "vbox", "gap": 0, "id": "source-take-head", "children": []any{
		map[string]any{"kind": "text", "content": take, "size": "lg", "fw": "bold"},
		map[string]any{"kind": "text", "content": fmt.Sprintf("ce que « %s » prend%s · cochez, puis enregistrez", name, inSystem(c)), "size": "sm", "color": "dimmed"},
	}}}
	if len(problems) > 0 {
		children = append(children, map[string]any{"kind": "text", "id": "source-take-problem", "size": "sm", "color": "red",
			"content": "La proposition n'a pas abouti : " + strings.Join(problems, " ; ")})
	}
	children = append(children, picker)
	return api.ConnectorPanelOutput{DSLTree: map[string]any{"kind": "vbox", "gap": "md", "children": children},
		Data: map[string]any{"selected": selected}, Title: name}, nil
}

// proposedRow is a proposed item as the proposal region draws it: its name,
// kind, slots' attributes, detail line, why, declared badges, path and
// how many items sit under it.
func proposedRow(it api.ProposedItem) map[string]any {
	row := map[string]any{"key": it.Key, "title": it.Title}
	if it.Title == "" {
		row["title"] = it.Key
	}
	if it.Parent != "" {
		row["parent"] = it.Parent
	}
	if it.Kind != "" {
		row["kind"] = it.Kind
	}
	if len(it.Badges) > 0 {
		row["badges"] = it.Badges
	}
	if it.Detail != "" {
		row["detail"] = it.Detail
	}
	if len(it.Attrs) > 0 {
		row["attrs"] = it.Attrs
	}
	if it.Why != "" {
		row["why"] = it.Why
	}
	if len(it.Path) > 0 {
		row["path"] = it.Path
	}
	if it.Below > 0 {
		row["below"] = it.Below
	}
	return row
}

// readLine is the save bar's idle line: when the source was last read
// (« Enregistré · lu il y a 2 h »), « pas encore lu » before its first read.
func (s *Service) readLine(ctx context.Context, cwm cwmapi.CWMService, source string) string {
	f, _, err := sourceFields(ctx, cwm, source)
	if err != nil || strings.TrimSpace(f.LastSynced) == "" {
		return "Enregistré · pas encore lu"
	}
	if ago := agoFR(f.LastSynced, time.Now()); ago != "" {
		return "Enregistré · lu " + ago
	}
	return "Enregistré · pas encore lu"
}

// agoFR says how long ago an RFC 3339 time was, in French (« il y a 2 min »);
// empty when it is not one.
func agoFR(at string, now time.Time) string {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(at))
	if err != nil {
		return ""
	}
	switch d := now.Sub(t); {
	case d < time.Minute:
		return "à l'instant"
	case d < time.Hour:
		return fmt.Sprintf("il y a %d min", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("il y a %d h", int(d.Hours()))
	default:
		return fmt.Sprintf("il y a %d j", int(d.Hours()/24))
	}
}

// inSystem is « dans <system> » after « prend » in the take tab's head - the
// short name its connector declares; nothing when it declares none.
func inSystem(c api.ConnectorDecl) string {
	if sys := strings.TrimSpace(c.System); sys != "" {
		return " dans " + sys
	}
	return ""
}

// agree picks the masculine or the feminine form.
func agree(fem bool, masc, femForm string) string {
	if fem {
		return femForm
	}
	return masc
}

// treeTake is an opened source's take tab for a connector picking in a
// check-box tree (mock-up v6's « Pages prises »): a head - the take label,
// what the source takes and where, « cochez, puis enregistrez » - then the
// connector's tree picker bound to the holder, its selection nested by each
// item's parent. When the connector proposes, the picker carries the
// proposal region above its tree (treeProposals). Nothing here names a
// connector.
func (s *Service) treeTake(ctx context.Context, cwm cwmapi.CWMService, c api.ConnectorDecl, module, source, name string, chosen []any) api.ConnectorPanelOutput {
	take := strings.TrimSpace(c.Take)
	if take == "" {
		take = api.SourceTakeDefault
	}
	where := s.takesWhere(ctx, c, module)
	head := map[string]any{"kind": "vbox", "gap": 0, "id": "source-take-head", "children": []any{
		map[string]any{"kind": "text", "content": take, "size": "lg", "fw": "bold"},
		map[string]any{"kind": "text", "content": fmt.Sprintf("ce que « %s » prend%s · cochez, puis enregistrez", name, where.in()), "size": "sm", "color": "dimmed"},
	}}
	children := []any{head}
	picker := pickerNode(c, module)
	if c.Propose != "" {
		if problems := s.treeProposals(ctx, cwm, c, module, source, where, picker); len(problems) > 0 {
			children = append(children, map[string]any{"kind": "text", "id": "source-take-problem", "size": "sm", "color": "red",
				"content": "La proposition n'a pas abouti : " + strings.Join(problems, " ; ")})
		}
	}
	children = append(children, picker)
	return api.ConnectorPanelOutput{DSLTree: map[string]any{"kind": "vbox", "gap": "md", "children": children},
		Data: map[string]any{"selected": chosen}, Title: name}
}

// treeProposals lays mock-up v6's proposal region on a tree picker
// (« Pages prises » ~1404-1414): « Proposé d'après vos mots-clés » - and
// « et les <units> de la source <system> » for a connector declaring
// propose_from -, the holder's keywords as chips and « Modifier les
// mots-clés », one row per proposed item (its path, why, sub-item count,
// badges, and the other source's pill when that source proposed it),
// « Aucune page ne correspond. » when none is, then « Ou choisissez dans
// l'arbre de l'espace <KEY> » and the search hint. Proposed rows stay out of
// the saved selection: the tree's ticks are the selection. It answers the
// problems the proposal met.
func (s *Service) treeProposals(ctx context.Context, cwm cwmapi.CWMService, c api.ConnectorDecl, module, source string, where takeWhere, picker map[string]any) []string {
	unit := strings.TrimSpace(pickerView(c).Unit)
	if unit == "" {
		unit = "élément"
	}
	fem := c.Picker.Feminine()
	holder := sourceHolder(ctx, cwm, source)
	kws := holderKeywords(ctx, cwm, holder)

	title := "Proposé d'après vos mots-clés"
	var other api.ConnectorDecl
	if c.ProposeFrom != nil {
		// Every system the carrier's sources it proposes from read
		// : « de la source Carto », « des sources Carto et
		// Registre ».
		followed := followedConnectors(ctx, cwm, module, c.ProposeFrom)
		var systems []string
		for i, n := range followed {
			o, _ := s.connector(ctx, n)
			if i == 0 {
				other = o
			}
			systems = append(systems, systemOf(o, n))
		}
		of := " de la source "
		if len(systems) > 1 {
			of = " des sources "
		}
		title += " et les " + unitPlural(pickerView(other).Unit) + of + andList(systems)
	}

	var problems []string
	got, err := s.Propose(ctx, api.ProposeInput{Connector: c.Name, Object: module, Terms: kws})
	if err != nil && !errors.Is(err, api.ErrNoContext) {
		problems = append(problems, err.Error()) // no keyword and nothing kept elsewhere: nothing to propose
	}
	problems = append(problems, got.Errors...)
	rows := []any{}
	seen := map[string]bool{}
	for _, it := range got.Items {
		if seen[it.Key] || strings.TrimSpace(it.Key) == "" {
			continue
		}
		seen[it.Key] = true
		row := proposedRow(it)
		if it.By != "" {
			by := other
			if by.Name != it.By {
				by, _ = s.connector(ctx, it.By)
			}
			byUnit := strings.TrimSpace(pickerView(by).Unit)
			if byUnit == "" {
				byUnit = "élément"
			}
			row["why"] = fmt.Sprintf("%s « %s » de la source %s", byUnit, it.ByItem, systemOf(by, it.By))
			row["pill"] = map[string]any{"label": systemOf(by, it.By), "color": by.Color, "icon": "link"}
		}
		rows = append(rows, row)
	}

	sub := []any{}
	for _, kw := range kws {
		sub = append(sub, map[string]any{"kind": "badge", "text": kw, "variant": "light", "color": "gray"})
	}
	if holder != "" {
		sub = append(sub, map[string]any{"kind": "button", "id": "source-take-keywords", "label": "Modifier les mots-clés", "size": "xs", "variant": "subtle",
			"action": map[string]any{"label": "Modifier les mots-clés", "navigate": map[string]any{"object": holder}}})
	}
	picker["proposals"] = map[string]any{"title": title,
		"sub":    map[string]any{"kind": "hbox", "gap": "xs", "children": sub},
		"items":  rows,
		"none":   fmt.Sprintf("%s %s ne correspond.", agree(fem, "Aucun", "Aucune"), unit),
		"choose": strings.TrimSpace("Ou choisissez dans l'arbre " + where.of())}
	labels, _ := picker["labels"].(map[string]any)
	if labels == nil {
		labels = map[string]any{}
		picker["labels"] = labels
	}
	labels["search"] = strings.TrimSpace(fmt.Sprintf("Chercher %s %s %s", agree(fem, "un", "une"), unit, where.within()))
	return problems
}

// systemOf is the short name a connector declares for its system, else its
// label, else the name it is known by.
func systemOf(c api.ConnectorDecl, name string) string {
	for _, v := range []string{c.System, c.Label} {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return name
}

// markOf is the badge a connector's system is drawn as: its declared mark
// (LX), else its system's name.
func markOf(c api.ConnectorDecl, name string) string {
	if m := strings.TrimSpace(c.Mark); m != "" {
		return m
	}
	return systemOf(c, name)
}

// takeWhere is where a source takes from: its scopes in the unit its
// connector declares for them (« espace », ARCHI), or the system its
// connector declares when it lists no scope.
type takeWhere struct {
	unit   string
	keys   []string
	system string
}

// place is the scope phrase after a preposition, the preposition and
// article contracted: in (« dans ») « dans l'espace ARCHI », of (« de »)
// « de l'espace ARCHI », « du projet X », « des espaces A, B »; keys false
// leaves the keys out (« dans l'espace »). The system (« dans LeanIX ») when
// no scope is known, nothing when neither is.
func (w takeWhere) place(of, keys bool) string {
	list := ""
	if keys {
		list = " " + strings.Join(w.keys, ", ")
	}
	switch {
	case len(w.keys) > 1 && of:
		return "des " + unitPlural(w.unit) + list
	case len(w.keys) > 1:
		return "dans les " + unitPlural(w.unit) + list
	case len(w.keys) == 1 && startsWithVowel(w.unit):
		return agree(of, "dans l'", "de l'") + w.unit + list
	case len(w.keys) == 1:
		return agree(of, "dans le ", "du ") + w.unit + list
	case w.system != "":
		return agree(of, "dans ", "de ") + w.system
	}
	return ""
}

// in is « dans l'espace ARCHI » after « prend » in a take tab's head, with
// its leading space; nothing when no place is known.
func (w takeWhere) in() string {
	if p := w.place(false, true); p != "" {
		return " " + p
	}
	return ""
}

// of is « de l'espace ARCHI » after « Ou choisissez dans l'arbre ».
func (w takeWhere) of() string { return w.place(true, true) }

// within is « dans l'espace » ending the tree's search hint (« Chercher une
// page dans l'espace »).
func (w takeWhere) within() string { return w.place(false, false) }

// takesWhere reads where a source takes from: its connector's scopes for the
// carrier, or the system its connector declares when it lists no scope.
func (s *Service) takesWhere(ctx context.Context, c api.ConnectorDecl, module string) takeWhere {
	w := takeWhere{system: strings.TrimSpace(c.System)}
	if c.Scopes == nil || strings.TrimSpace(c.Scopes.Unit) == "" {
		return w
	}
	got, err := s.Scopes(ctx, api.ScopesInput{Connector: c.Name, Object: module})
	if err != nil || len(got.Scopes) == 0 {
		return w
	}
	w.unit = strings.TrimSpace(c.Scopes.Unit)
	for _, sc := range got.Scopes {
		w.keys = append(w.keys, sc.Key)
	}
	return w
}

// treeLabels are a check-box tree picker's words in its unit, agreeing with
// the gender its connector declares - « avec les sous-pages », « cette page
// seule », « incluse », the help on how sub-items are taken and the notice
// when unticking an included one - over the picker's generic « élément ».
func treeLabels(unit string, fem bool) map[string]any {
	u := strings.TrimSpace(unit)
	if u == "" {
		u, fem = "élément", false
	}
	pl, vowel := unitPlural(u), startsWithVowel(u)
	this := agree(fem, "ce", "cette")
	if vowel && !fem {
		this = "cet"
	}
	self := fmt.Sprintf("%s %s %s", this, u, agree(fem, "seul", "seule"))
	the := agree(fem, "Le ", "La ")
	if vowel {
		the = "L'"
	}
	parent := agree(fem, "son parent", "sa parente")
	toParent := agree(fem, "le parent", "la parente")
	taken := agree(fem, "pris", "prises")
	return map[string]any{
		"withChildren": "avec les sous-" + pl,
		"belowOne":     "avec 1 sous-" + u,
		"belowMany":    "avec {n} sous-" + pl,
		"self":         self,
		"included":     agree(fem, "inclus", "incluse"),
		"treeHelp":     "Comment se prennent les sous-" + pl,
		"treeHelpWith": fmt.Sprintf("Cocher %s %s %s prend avec ses sous-%s, aussi %s plus tard. « %s » ne prend %s.",
			agree(fem, "un", "une"), u, agree(fem, "le", "la"), pl, agree(fem, "ceux ajoutés", "celles ajoutées"), self, agree(fem, "que lui", "qu'elle")),
		"treeHelpIncluded": fmt.Sprintf("Sous-%s %s par %s. %s décocher fait passer %s à « %s » ; ses autres sous-%s restent %s.",
			u, agree(fem, "inclus", "incluse"), parent, agree(fem, "Le", "La"), toParent, self, pl, taken),
		"treeHelpMixed": fmt.Sprintf("%s%s n'est pas %s, mais des %s en dessous le sont.", the, u, agree(fem, "pris", "prise"), pl),
		"treeNotice": fmt.Sprintf("« {removed} » %s : « {parent} » passe à « %s », ses autres sous-%s restent %s.",
			agree(fem, "retiré", "retirée"), self, pl, taken),
	}
}

// startsWithVowel says whether a French word elides the article before it.
func startsWithVowel(w string) bool {
	for _, r := range strings.ToLower(w) {
		return strings.ContainsRune("aeiouyéèêëàâîïôûh", r)
	}
	return false
}
