// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// NewSourcePage is the information-source type's create page under its
// holder (mock-up v6 view 11): « Ce qu'elle lit », one choice
// card per contributed connector - its initials on its colour, its label, its
// description -, one chosen at a time; a connector the holder already reads
// through is greyed « Une seule par <holder> » with the pill « déjà dans … »
// and why on hover - only under a holder that asks for one source per
// connector (api.OnePerConnectorCapability); any other holder may add one
// more source of any connector. Choosing one names the source after it (« Nom », changed
// at will); a connector a carrier adds scopes to asks its first one (its
// unit: « Espace »); under the cards, the chosen one's note says what the
// source will propose (connectorNote). « Créer » - disabled until a connector and a name are
// set - makes it the holder's source (sources/use), which then opens on its
// take tab, the dialog the page is drawn in closing; « Annuler » closes it.
func (s *Service) NewSourcePage(ctx context.Context, in api.NewSourcePageInput) (api.ConnectorPanelOutput, error) {
	parent := strings.TrimSpace(in.Parent)
	if parent == "" {
		return api.ConnectorPanelOutput{}, fmt.Errorf("%w: sources.NewSourcePage: parent required", cwmapi.ErrInvalidInput)
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.ConnectorPanelOutput{}, fmt.Errorf("sources: resolve CWMService: %w", err)
	}
	if _, err := carrierOf(ctx, cwm, parent); err != nil {
		return api.ConnectorPanelOutput{}, err
	}
	list, err := s.Connectors(ctx, api.ConnectorsInput{})
	if err != nil {
		return api.ConnectorPanelOutput{}, err
	}
	srcs, err := sourcesOf(ctx, cwm, parent)
	if err != nil {
		return api.ConnectorPanelOutput{}, err
	}
	held := map[string]bool{}
	onePer := onePerConnector(ctx, cwm, parent)
	for _, src := range srcs {
		if onePer && src.Via == "child" {
			held[src.Connector] = true
		}
	}
	word, phrase := holderWords(ctx, cwm, parent)
	byName := map[string]api.ConnectorDecl{}
	for _, c := range list.Connectors {
		byName[c.Name] = c
	}
	cards := make([]render.ChoiceCard, 0, len(list.Connectors))
	var scopeFields, notes []any
	for _, c := range list.Connectors {
		label := c.Label
		if label == "" {
			label = c.Name
		}
		card := render.ChoiceCard{Value: c.Name, Label: label, Description: c.Description, Color: c.Color,
			Sets: map[string]string{"name": label}}
		if held[c.Name] {
			card.Disabled = true
			card.Description = "Une seule par " + word
			card.Pill = "déjà dans " + phrase
			card.Tooltip = "Une seule source par connecteur : ouvrez celle " + ofHolder(phrase) + " pour changer ce qu'elle prend."
		}
		cards = append(cards, card)
		if !held[c.Name] {
			notes = append(notes, map[string]any(connectorNote(ctx, cwm, parent, phrase, c, byName, srcs)))
		}
		if c.Scopes != nil && c.Scopes.Add && !held[c.Name] {
			unit := strings.TrimSpace(c.Scopes.Unit)
			if unit == "" {
				unit = "portée"
			}
			r := []rune(unit)
			field := render.TextField("scope", strings.ToUpper(string(r[:1]))+string(r[1:]), "").ShowWhen("connector="+c.Name).
				With("description", fmt.Sprintf("Où elle cherche d'abord dans %s ; d'autres s'ajoutent dans son onglet.", label))
			// The scopes joined in the connector's connection, a choice of them;
			// none listed (or no answer), the scope is typed.
			if opts := joinedScopes(ctx, c); len(opts) > 0 {
				field = field.With("options", opts).With("value", opts[0]["value"]).
					With("description", fmt.Sprintf("Les %ss joints dans la connexion %s.", unit, label))
			}
			scopeFields = append(scopeFields, map[string]any(field))
		}
	}
	children := []any{
		render.Node{"kind": "text", "content": "Ce qu'elle lit", "size": "sm", "weight": 600},
		render.ChoiceCards("connector", "Ce qu'elle lit", cards...).With("id", "new-source-connectors").With("cols", 1),
	}
	children = append(children, notes...)
	if len(held) > 0 {
		children = append(children, render.Node{"kind": "text", "content": "Une seule source par connecteur : celles déjà dans " + phrase + " sont grisées ; pour changer ce qu'elles prennent, ouvrez-les.", "size": "xs", "color": "dimmed"})
	}
	children = append(children, render.TextField("name", "Nom", "").With("placeholder", "Choisissez d'abord ce qu'elle lit").With("required", true))
	children = append(children, scopeFields...)
	children = append(children,
		render.Node{"kind": "text", "content": "Elle s'ouvre sur ses propositions : vous cochez, puis « Enregistrer et lire ».", "size": "sm", "color": "dimmed"},
		render.HBox(render.Button("Annuler", render.CloseHere("Annuler")).
			With("id", "new-source-cancel").With("variant", "default").With("size", "sm"),
			render.Button("Créer", render.Action{Label: "Créer", Icon: "plus", URI: "ktw-agents:sources/use",
				Args: map[string]any{"id": parent, "connector": "${selection.connector}", "name": "${selection.name}", "scope": "${selection.scope}"}}).
				With("id", "new-source-create").With("variant", "filled").With("size", "sm").
				Requires("Choisissez ce qu'elle lit et donnez-lui un nom.", "connector", "name")).With("justify", "flex-end"),
	)
	tree := render.Node{"kind": "vbox", "gap": "sm", "children": children}
	return api.ConnectorPanelOutput{DSLTree: tree, Data: map[string]any{}, Title: "Nouvelle source d'entrée"}, nil
}

// newScopesTimeout bounds the scopes list a create page waits on: the
// configuration's scopes, read without reaching the system.
const newScopesTimeout = 10 * time.Second

// joinedScopes are the scopes a connector's configuration joins (its scopes
// list with no carrier location), as a field's options; none when it lists
// none or does not answer in time.
func joinedScopes(ctx context.Context, c api.ConnectorDecl) []map[string]string {
	if c.Scopes == nil || c.Scopes.List == "" {
		return nil
	}
	got, err := call[api.ConnectorScopesOutput](ctx, c.Scopes.List, newScopesTimeout, api.ConnectorScopesInput{})
	if err != nil {
		return nil
	}
	var opts []map[string]string
	for _, sc := range got.Scopes {
		if k := strings.TrimSpace(sc.Key); k != "" {
			label := k
			if t := strings.TrimSpace(sc.Title); t != "" && t != k {
				label = k + " · " + t
			}
			opts = append(opts, map[string]string{"value": k, "label": label})
		}
	}
	return opts
}

// holderWords names the holder as the page says it: its type's label's first
// word, lowercased (« étude » for an Étude de l'existant, « module »), and
// with its article (« l'étude », « le module ») - read from the type, never
// named here.
func holderWords(ctx context.Context, cwm cwmapi.CWMService, id string) (word, phrase string) {
	word = "objet"
	module, err := cwmapi.ModuleOfObjectID(id)
	if err == nil {
		if got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id}); err == nil {
			if t, err := cwm.ObjectTypeGet(ctx, cwmapi.ObjectTypeGetInput{Name: got.Object.Type}); err == nil {
				label := t.ObjectType.Label
				if label == "" {
					label = t.ObjectType.Name
				}
				if f := strings.Fields(strings.ReplaceAll(label, "-", " ")); len(f) > 0 {
					word = strings.ToLower(f[0])
				}
			}
		}
	}
	if r := []rune(word); len(r) > 0 && strings.ContainsRune("aeiouyéèêàâîïôû", unicode.ToLower(r[0])) {
		return word, "l'" + word
	}
	return word, "le " + word
}

// ofHolder is « de » before the holder's phrase: « de l'étude », « du module ».
func ofHolder(phrase string) string {
	if w, ok := strings.CutPrefix(phrase, "le "); ok {
		return "du " + w
	}
	return "de " + phrase
}

// connectorNote is the note under a chosen connector's card (mock-up v6 view
// 11): a tinted callout box with its icon, drawn only while its card is
// chosen - read from its declaration, never its name: a connector reading
// from another source names the holder's source of that connector and the
// values it follows (link, blue; info, gray while the holder has none); one
// that proposes or searches proposes from the holder's keywords, named
// (sparkle, blue); any other is where files are dropped (info, gray).
func connectorNote(ctx context.Context, cwm cwmapi.CWMService, parent, phrase string, c api.ConnectorDecl, byName map[string]api.ConnectorDecl, srcs []api.SourceSummary) render.Node {
	note := func(content, icon string, gray bool) render.Node {
		tone := "blue"
		if gray {
			tone = "gray"
		}
		return render.Node{"kind": "box", "id": "new-source-note-" + c.Name, "data-element": "text:new-source-note-" + c.Name, "callout": tone,
			"children": []any{render.Node{"kind": "hbox", "gap": "xs", "align": "flex-start", "wrap": false, "children": []any{
				render.Node{"kind": "badge", "text": "", "icon": icon, "color": tone, "variant": "transparent", "size": "lg"},
				render.Node{"kind": "text", "content": content, "size": "sm"},
			}}}}.ShowWhen("connector=" + c.Name)
	}
	if c.From != nil {
		from, otherName := "", c.From.First()
		for _, src := range srcs {
			if src.Via == "child" && c.From.Follows(src.Connector) {
				from, otherName = src.Name, src.Connector
				break
			}
		}
		other := byName[otherName]
		otherLabel := other.Label
		if otherLabel == "" {
			otherLabel = otherName
		}
		var values []string
		if from != "" {
			if got, err := fromItems(ctx, cwm, parent, c); err == nil && got != nil {
				values = got.Values
			}
		}
		if from != "" && len(values) == 0 {
			return note(fmt.Sprintf("Elle lira depuis « %s » : elle suivra les %s que celle-ci prend ; celle-ci n'a encore rien pris. Elle ne cherche jamais par les mots-clés.", from, unitsOf(other)), "info-circle", true)
		}
		if from == "" {
			return note(fmt.Sprintf("Elle lit depuis une source %s %s : elle proposera d'après ce que celle-ci tient. Il n'y en a pas encore.", otherLabel, ofHolder(phrase)), "info-circle", true)
		}
		shown := values
		if len(shown) > 5 {
			shown = append(append([]string{}, shown[:5]...), "…")
		}
		return note(fmt.Sprintf("Elle lira depuis « %s » : le %s de ses %s (%s) est ce qu'elle suit. Elle ne cherche jamais par les mots-clés.",
			from, attrLabel(other, c.From.Attr), unitsOf(other), strings.Join(shown, ", ")), "link", false)
	}
	if c.Propose != "" || c.Search != "" {
		if kw := holderKeywords(ctx, cwm, parent); len(kw) > 0 {
			quoted := make([]string, len(kw))
			for i, k := range kw {
				quoted[i] = "« " + k + " »"
			}
			return note(fmt.Sprintf("Dès sa création, elle propose des %s d'après vos mots-clés %s : vous cochez ce qui convient.", unitsOf(c), strings.Join(quoted, " · ")), "sparkles", false)
		}
		return note(fmt.Sprintf("Dès sa création, elle propose des %s d'après les mots-clés %s : vous cochez ce qui convient.", unitsOf(c), ofHolder(phrase)), "sparkles", false)
	}
	return note("Vous y déposez vos fichiers : elle ne propose rien.", "info-circle", true)
}

// holderKeywords are the keywords the holder carries - a `keywords` list of
// any of its capabilities (an as-is study's), in the order kept; none when it
// carries none.
func holderKeywords(ctx context.Context, cwm cwmapi.CWMService, id string) []string {
	module, err := cwmapi.ModuleOfObjectID(id)
	if err != nil {
		return nil
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
	if err != nil {
		return nil
	}
	return keywordsOf(got.Object)
}

// keywordsOf reads an object's keywords from its capability groups (sorted
// by capability, each keyword once, trimmed).
func keywordsOf(o cwmapi.Object) []string {
	caps := make([]string, 0, len(o.Capabilities))
	for id := range o.Capabilities {
		caps = append(caps, id)
	}
	sort.Strings(caps)
	seen := map[string]bool{}
	var out []string
	add := func(k string) {
		if k = strings.TrimSpace(k); k != "" && !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	for _, id := range caps {
		switch kw := o.Capabilities[id]["keywords"].(type) {
		case []any:
			for _, k := range kw {
				if s, ok := k.(string); ok {
					add(s)
				}
			}
		case []string:
			for _, k := range kw {
				add(k)
			}
		}
	}
	return out
}

// unitsOf is what a connector's items are in its words, plural
// (« applications »), « éléments » when it declares no unit.
func unitsOf(c api.ConnectorDecl) string {
	unit := "élément"
	if c.Picker != nil && strings.TrimSpace(c.Picker.Unit) != "" {
		unit = strings.TrimSpace(c.Picker.Unit)
	}
	if strings.HasSuffix(unit, "s") || strings.HasSuffix(unit, "x") {
		return unit
	}
	return unit + "s"
}

// attrLabel names an attribute of a connector's items as its kinds' slots
// label it (« code applicatif »), the attribute itself when none does.
func attrLabel(c api.ConnectorDecl, attr string) string {
	for _, k := range c.Kinds {
		for _, sl := range k.Slots {
			if sl.Attr == attr && strings.TrimSpace(sl.Label) != "" {
				return strings.ToLower(strings.TrimSpace(sl.Label))
			}
		}
	}
	return attr
}
