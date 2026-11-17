// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// syncOptions are the re-reading modes as « Relire » lists them (mock-up v6).
var syncOptions = []any{
	map[string]any{"value": api.SyncManual, "label": "À la main"},
	map[string]any{"value": api.SyncOnOpen, "label": "À l'ouverture de l'étude"},
	map[string]any{"value": api.SyncDaily, "label": "Chaque jour"},
	map[string]any{"value": api.SyncDailyOnOpen, "label": "Chaque jour, et à l'ouverture de l'étude"},
}

// settingsForm is an opened source's « Réglages » as mock-up v6 draws them
// (vSourceSettings): one form read in rows - a value and its
// pencil per row, edited where it is shown and kept at once (`edit: row`,
// `save: change`) by SettingsSave -, the rows it only reads without one, a
// pencil that opens what it names (the holder's keywords, the take tab, the
// source it reads from), « Tester » on the connection row; then « Supprimer
// la source ». Each row is read from the connector's declaration and the
// source: nothing here names a connector. It answers the card and the value
// its form binds.
func (s *Service) settingsForm(ctx context.Context, cwm cwmapi.CWMService, c api.ConnectorDecl, f api.InformationSourceFields, source string) (map[string]any, api.SourceSettings) {
	view := pickerView(c)
	holder := sourceHolder(ctx, cwm, source)
	value := api.SourceSettings{Reads: view.Label, Keywords: f.Keywords, ProposeMax: api.ProposeBound(f.ProposeMax), Sync: f.Sync}
	if value.Sync == "" {
		value.Sync = api.SyncManual
	}
	var fields []any
	field := func(id, label, hint string, more map[string]any) {
		spec := map[string]any{"id": id, "label": label}
		if hint != "" {
			spec["description"] = hint
		}
		for k, v := range more {
			spec[k] = v
		}
		fields = append(fields, spec)
	}
	readOnly := map[string]any{"readOnly": true}
	opens := func(object, panel string) map[string]any {
		if object == "" {
			return readOnly
		}
		nav := map[string]any{"object": object}
		if panel != "" {
			nav["panel"] = panel
		}
		return map[string]any{"action": map[string]any{"navigate": nav}}
	}
	field("reads", "Ce qu'elle lit", "", map[string]any{"readOnly": true, "read": map[string]any{"mark": brandMark(c)}})
	if c.Scopes != nil {
		value.Scopes = locationScopes(f.Location)
		unit := strings.TrimSpace(c.Scopes.Unit)
		if unit == "" {
			unit = "périmètre"
		}
		var choices map[string]any
		if c.Scopes.List != "" {
			choices = map[string]any{"choices": map[string]any{"uri": api.ScopeOptionsURI, "args": map[string]any{"id": source}}}
		}
		field("scopes", strings.ToUpper(unit[:1])+unit[1:], "", choices)
	}
	if c.Propose != "" || c.Search != "" {
		value.SearchesBy = holderKeywords(ctx, cwm, holder)
		_, phrase := holderWords(ctx, cwm, holder)
		searches := map[string]any{"read": map[string]any{"lead": "les mots-clés " + ofHolder(phrase)}}
		for k, v := range opens(holder, "") {
			searches[k] = v
		}
		field("searches_by", "Cherche d'après", "ce qu'elle propose", searches)
		field("keywords", "Mots-clés en plus", "pour cette source seulement", map[string]any{"placeholder": "Ajouter un mot-clé : tapez puis Entrée"})
		field("propose_max", "Propositions", "une recherche bornée par source", map[string]any{
			"read": map[string]any{"template": "Au plus {value} " + unitPlural(view.Unit)}})
	}
	if c.Picker != nil && strings.TrimSpace(c.Description) != "" {
		value.Takes = strings.TrimSpace(c.Description)
		field("takes", "Ce qu'elle prend", "", opens(source, api.InformationSourceType+"-"+api.SourceTabTake))
	}
	if from := c.From; from != nil {
		name := followedConnector(ctx, cwm, holder, from)
		label := name
		if fc, err := s.connector(ctx, name); err == nil {
			label = pickerView(fc).Label
		}
		value.ReadsFrom = fmt.Sprintf("%s → le %s de chaque élément", label, from.Attr)
		followed := ""
		if srcs, err := sourcesOf(ctx, cwm, holder); err == nil {
			followed = ownSource(srcs, name)
		}
		field("reads_from", "Lit depuis", "une autre source de l'objet", opens(followed, ""))
	}
	if conn := c.Connection; conn != nil && strings.TrimSpace(conn.Label) != "" {
		value.Through = api.ConnectionOf(f.Location)
		field("through", strings.TrimSpace(conn.Label), "", connectionChoices(conn))
	}
	schema := render.InlineSchema[api.SourceSettings]("SourceSettings", nil)
	if rows := s.locationRows(ctx, c); rows != nil {
		value.Location = map[string]any{}
		for _, name := range rows.names {
			if v, ok := f.Location[name]; ok {
				value.Location[name] = v
			}
		}
		rows.composeInto(schema)
		fields = append(fields, rows.fields...)
	}
	if kinds := typedKinds(c); len(kinds) > 0 {
		// « Colonne identifiant » per kind its « Contenu lu » draws as a
		// table of its own: the attribute that table leads
		// with and filters on, or none.
		value.IDColumns = map[string]string{}
		ids := map[string]any{}
		for _, k := range kinds {
			value.IDColumns[k] = f.IDColumns[k]
			if strings.TrimSpace(value.IDColumns[k]) == "" {
				value.IDColumns[k] = api.IDColumnUID // no choice reads as the uid it leads with
			}
			label := unitPlural(kindLabel(c, k))
			ids[k] = map[string]any{"type": "string", "label": label, "optional": true}
			field("id_columns."+k, "Colonne identifiant", label+" de « Contenu lu »", map[string]any{"options": s.idColumnOptions(ctx, c.Items[k])})
		}
		if types, _ := schema["types"].(map[string]any); types != nil {
			if rt, _ := types[schema["root"].(string)].(map[string]any); rt != nil {
				if fields, _ := rt["fields"].(map[string]any); fields != nil {
					fields["id_columns"] = map[string]any{"type": "object", "label": "Colonne identifiant", "optional": true, "fields": ids}
				}
			}
		}
	}
	field("sync", "Relire", "pour voir ce qui a changé", map[string]any{"options": syncOptions})
	if c.Test != "" {
		value.Connection = connectionState(f)
		field("connection", "Connexion", "", map[string]any{"button": map[string]any{"label": "Tester", "uri": api.SettingsTestURI, "args": map[string]any{"id": source}},
			"read": map[string]any{"pill": map[string]any{"path": "state", "tones": connectionTones},
				"after": map[string]any{"path": "tested_at", "as": "relative-time", "prefix": "testée"}}})
	}
	form := map[string]any{"kind": "form", "id": "source-settings-form", "mode": "edit", "hideTitle": true,
		"schema":    schema,
		"dataField": "$.settings",
		"submit":    map[string]any{"label": "Enregistrer", "uri": api.SettingsSaveURI, "args": map[string]any{"id": source, "settings": "${value}"}},
		"ui":        map[string]any{"name": "settings", "layout": "groups", "edit": "row", "save": "change", "groups": []any{map[string]any{"cols": 1, "fields": fields}}}}
	remove := render.Button("Supprimer la source", render.Action{Label: "Supprimer la source", URI: api.RemoveURI, Args: map[string]any{"id": source},
		Confirm: "Supprimer la source ? Ce qu'elle a apporté sort de la base de connaissance.", Dangerous: true}).
		With("id", "source-settings-remove").With("color", "red").With("variant", "subtle").With("size", "compact-xs").With("icon", "trash")
	removeNote := render.Text("ce qu'elle a apporté sort de la base de connaissance").With("size", "xs").With("color", "dimmed").With("id", "source-settings-remove-note")
	card := render.Card(render.Node(form)).With("id", "source-settings")
	return map[string]any{"kind": "vbox", "gap": "sm", "children": []any{card, render.HBox(remove, removeNote).With("gap", "xs").With("align", "center")}}, value
}

// settingsUIForm is the name of the ui-form a connector's location
// data-type declares for the fields « Réglages » edits.
const settingsUIForm = "settings"

// locationSettings are the location fields a connector lets an opened
// source's « Réglages » edit: the fields of its location data-type's
// ui-form named settings, drawn as rows of the settings form by their path
// (location.<field>), the data-type's root typing the form's location.
type locationSettings struct {
	root   map[string]any // the data-type's root type, its extensions left out
	types  map[string]any // the data-type's other types
	fields []any          // the rows, each location.<field>
	names  []string       // the location fields they edit
}

// locationRows are the location fields « Réglages » edits for a connector
// whose picker selects - its selection is never asked again
// ; one without a selecting picker keeps « Où elle lit »
// . None when the data-type declares no settings ui-form
// or cannot be read.
func (s *Service) locationRows(ctx context.Context, c api.ConnectorDecl) *locationSettings {
	if strings.TrimSpace(c.Location) == "" || !c.Picker.Selects() {
		return nil
	}
	schemas, err := application.UseService[schemaapi.SchemaService](ctx)
	if err != nil {
		return nil
	}
	got, err := schemas.GetDataTypeWithExtensions(ctx, schemaapi.GetDataTypeWithExtensionsInput{URI: c.Location,
		Extensions: []schemaapi.ExtensionRef{{Type: "ui-form", Name: settingsUIForm}}})
	if err != nil || got.Schema == nil || got.Schema.Types[got.Schema.Root] == nil {
		return nil
	}
	rootType := got.Schema.Types[got.Schema.Root]
	var groups []any
	for _, ext := range rootType.Extensions {
		if name, _ := ext.Payload["name"].(string); ext.Kind == "ui-form" && name == settingsUIForm {
			groups, _ = ext.Payload["groups"].([]any)
		}
	}
	out := &locationSettings{types: map[string]any{}}
	reserved := map[string]bool{api.LocationScopes: true, api.LocationConnection: true}
	for _, g := range groups {
		gm, _ := g.(map[string]any)
		list, _ := gm["fields"].([]any)
		for _, fl := range list {
			spec := map[string]any{}
			switch v := fl.(type) {
			case string:
				spec["id"] = v
			case map[string]any:
				for k, x := range v {
					spec[k] = x
				}
			}
			id, _ := spec["id"].(string)
			if id = strings.TrimSpace(id); id == "" || reserved[id] || rootType.Fields[id] == nil {
				continue
			}
			if _, ok := spec["label"]; !ok {
				label := rootType.Fields[id].Label
				if label == "" {
					label = id
				}
				spec["label"] = label
			}
			spec["id"] = "location." + id
			out.fields = append(out.fields, spec)
			out.names = append(out.names, id)
		}
	}
	if len(out.fields) == 0 {
		return nil
	}
	raw, _ := json.Marshal(got.Schema.Types)
	_ = json.Unmarshal(raw, &out.types)
	out.root, _ = out.types[got.Schema.Root].(map[string]any)
	delete(out.root, "extensions")
	delete(out.types, got.Schema.Root)
	return out
}

// composeInto types the settings form's location by the data-type: its
// root the location field, its other types beside the form's own.
func (l *locationSettings) composeInto(schema map[string]any) {
	types, _ := schema["types"].(map[string]any)
	root, _ := schema["root"].(string)
	rt, _ := types[root].(map[string]any)
	fields, _ := rt["fields"].(map[string]any)
	if fields == nil {
		return
	}
	loc := map[string]any{}
	for k, v := range l.root {
		loc[k] = v
	}
	loc["optional"] = true
	fields["location"] = loc
	for name, t := range l.types {
		if _, taken := types[name]; !taken {
			types[name] = t
		}
	}
}

// universeConnection is the connection a source reads through when it names
// none: its connector's configuration in the universe.
var universeConnection = map[string]any{"value": "", "label": "La configuration de l'univers"}

// connectionChoices are the choices of the row naming the connection a
// source reads through: the connector's list method asked when the row is
// drawn, the universe's configuration the one option without one or while
// it has not answered.
func connectionChoices(conn *api.ConnectorConnection) map[string]any {
	more := map[string]any{"options": []any{universeConnection}}
	if list := strings.TrimSpace(conn.List); list != "" {
		more["choices"] = map[string]any{"uri": list, "args": map[string]any{}}
	}
	return more
}

// locationScopes are the scopes a source's location keeps, as text.
func locationScopes(location map[string]any) []string {
	var out []string
	switch v := location[api.LocationScopes].(type) {
	case []string:
		out = append(out, v...)
	case []any:
		for _, s := range v {
			if t, ok := s.(string); ok && strings.TrimSpace(t) != "" {
				out = append(out, strings.TrimSpace(t))
			}
		}
	}
	return out
}

// The connection's states in words, and the tone of the pill each is drawn
// as on « Réglages » (mock-up v6: « Joignable » teal).
const (
	connectionUntested    = "Pas encore testée"
	connectionReachable   = "Joignable"
	connectionUnreachable = "Injoignable"
)

var connectionTones = map[string]any{connectionReachable: "teal", connectionUnreachable: "red"}

// connectionState is a source's connection as last tested: its state in
// words and when it was tested (RFC 3339), said from now on the row.
func connectionState(f api.InformationSourceFields) *api.SourceConnection {
	t, err := time.Parse(time.RFC3339, f.TestedAt)
	switch {
	case f.TestedAt == "" || err != nil:
		return &api.SourceConnection{State: connectionUntested}
	case f.Reachable:
		return &api.SourceConnection{State: connectionReachable, TestedAt: t.UTC().Format(time.RFC3339)}
	default:
		return &api.SourceConnection{State: connectionUnreachable, TestedAt: t.UTC().Format(time.RFC3339)}
	}
}

// brandMark is the mark « Ce qu'elle lit » draws before the connector's
// label (mock-up v6): its declared mark (LX), else the monogram of its
// system, on its colour.
func brandMark(c api.ConnectorDecl) map[string]any {
	mark := map[string]any{"color": markColor(c.Color)}
	if m := strings.TrimSpace(c.Mark); m != "" {
		mark["letters"] = m
	} else {
		mark["text"] = systemOf(c, c.Name)
	}
	return mark
}

// markColor is a connector's colour as a brand mark's background: a Mantine
// colour name at its shade 6, any other CSS colour as it is, grey when none.
func markColor(color string) string {
	color = strings.TrimSpace(color)
	switch {
	case color == "":
		return "var(--mantine-color-gray-6)"
	case strings.ContainsAny(color, "#("):
		return color
	default:
		return "var(--mantine-color-" + color + "-6)"
	}
}

// SettingsSave keeps an opened source's « Réglages » as its form edits them
// : its keywords, its proposal bound, its scopes and the
// connection it reads through when its connector names one - in its
// location, the rest of it kept -, the id column of each kind it reads
// (kept when the form sends them, the uid as IDColumnUID) and its re-reading mode, through the
// source's save gate; the rows it only reads are ignored. A daily mode keeps
// the holder's daily timer.
func (s *Service) SettingsSave(ctx context.Context, in api.SettingsSaveInput) (api.SettingsSaveOutput, error) {
	if strings.TrimSpace(in.ID) == "" {
		return api.SettingsSaveOutput{}, fmt.Errorf("%w: sources.SettingsSave: id required", cwmapi.ErrInvalidInput)
	}
	set := in.Settings
	if set.ProposeMax != 0 && (set.ProposeMax < 1 || set.ProposeMax > api.ProposeMaxLimit) {
		return api.SettingsSaveOutput{OK: false, Message: fmt.Sprintf("Propositions : entre 1 et %d.", api.ProposeMaxLimit)}, nil
	}
	switch set.Sync {
	case "", api.SyncManual, api.SyncOnOpen, api.SyncDaily, api.SyncDailyOnOpen:
	default:
		return api.SettingsSaveOutput{OK: false, Message: fmt.Sprintf("Relire : %q n'est pas un mode connu.", set.Sync)}, nil
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.SettingsSaveOutput{}, fmt.Errorf("sources: resolve CWMService: %w", err)
	}
	f, module, err := sourceFields(ctx, cwm, in.ID)
	if err != nil {
		return api.SettingsSaveOutput{}, err
	}
	f.Keywords = nil
	seen := map[string]bool{}
	for _, kw := range set.Keywords {
		if kw = strings.TrimSpace(kw); kw != "" && !seen[strings.ToLower(kw)] {
			seen[strings.ToLower(kw)] = true
			f.Keywords = append(f.Keywords, kw)
		}
	}
	f.ProposeMax = set.ProposeMax
	if f.ProposeMax == api.ProposeMax {
		f.ProposeMax = 0 // the default is not written
	}
	f.Sync = set.Sync
	if f.Sync == api.SyncManual {
		f.Sync = ""
	}
	c, cerr := s.connector(ctx, f.Connector)
	if rows := s.locationRows(ctx, c); cerr == nil && rows != nil && set.Location != nil {
		location := map[string]any{}
		for k, v := range f.Location {
			location[k] = v
		}
		for _, name := range rows.names {
			if v, ok := set.Location[name]; ok && v != nil {
				location[name] = v
			} else {
				delete(location, name)
			}
		}
		f.Location = location
	}
	if cerr == nil && c.Connection != nil {
		if f.Location == nil {
			f.Location = map[string]any{}
		}
		if through := strings.TrimSpace(set.Through); through != "" {
			f.Location[api.LocationConnection] = through
		} else {
			delete(f.Location, api.LocationConnection)
		}
	}
	if cerr == nil && set.IDColumns != nil {
		// A kind with no choice reads IDColumnUID on the form, which sends
		// every row back: it stays without a choice - a carrier may still
		// give it a default - unless a choice was there, then the uid is
		// kept as the person's choice and no carrier's default replaces it.
		before := f.IDColumns
		f.IDColumns = nil
		for _, k := range typedKinds(c) {
			v := strings.TrimSpace(set.IDColumns[k])
			if v == api.IDColumnUID && strings.TrimSpace(before[k]) == "" {
				v = ""
			}
			if v != "" {
				if f.IDColumns == nil {
					f.IDColumns = map[string]string{}
				}
				f.IDColumns[k] = v
			}
		}
	}
	if cerr == nil && c.Scopes != nil {
		if f.Location == nil {
			f.Location = map[string]any{}
		}
		scopes := []any{}
		for _, sc := range set.Scopes {
			if sc = strings.TrimSpace(sc); sc != "" {
				scopes = append(scopes, sc)
			}
		}
		if len(scopes) > 0 {
			f.Location[api.LocationScopes] = scopes
		} else {
			delete(f.Location, api.LocationScopes)
		}
	}
	if err := s.recordSource(ctx, cwm, module, in.ID, f); err != nil {
		return api.SettingsSaveOutput{}, err
	}
	if api.SyncIn(f.Sync, api.SyncDaily) {
		if holder := sourceHolder(ctx, cwm, in.ID); holder != "" {
			if err := ensureDailyTimer(ctx, cwm, holder); err != nil {
				slog.Debug("sources: daily timer", "carrier", holder, "err", err)
			}
		}
	}
	return api.SettingsSaveOutput{OK: true, Message: "Réglages enregistrés."}, nil
}

// SettingsTest tests an opened source's connection - « Tester » of its
// « Réglages » - and records when and whether it answered
// answering in the check grammar a read row draws.
func (s *Service) SettingsTest(ctx context.Context, in api.SettingsTestInput) (api.SettingsTestOutput, error) {
	if strings.TrimSpace(in.ID) == "" {
		return api.SettingsTestOutput{}, fmt.Errorf("%w: sources.SettingsTest: id required", cwmapi.ErrInvalidInput)
	}
	got, err := s.SourceTest(ctx, api.SourceTestInput{ID: in.ID})
	if err != nil {
		got = api.ConnectorTestOutput{Message: err.Error()}
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.SettingsTestOutput{}, fmt.Errorf("sources: resolve CWMService: %w", err)
	}
	f, module, err := sourceFields(ctx, cwm, in.ID)
	if err != nil {
		return api.SettingsTestOutput{}, err
	}
	f.TestedAt, f.Reachable = time.Now().UTC().Format(time.RFC3339), got.OK
	if err := s.recordSource(ctx, cwm, module, in.ID, f); err != nil {
		return api.SettingsTestOutput{}, err
	}
	msg := strings.TrimSpace(got.Message)
	if !got.OK {
		if msg == "" {
			msg = "La source ne répond pas."
		}
		return api.SettingsTestOutput{OK: false, Message: msg}, nil
	}
	if msg == "" {
		msg = "La source répond."
	}
	return api.SettingsTestOutput{OK: true, Text: msg}, nil
}

// searchSettings are a carrier's own source's search settings for a
// connector: its keywords and its proposal bound (0 when it has none).
func (s *Service) searchSettings(ctx context.Context, carrier, connector string) ([]string, int) {
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil || carrier == "" {
		return nil, 0
	}
	srcs, err := sourcesOf(ctx, cwm, carrier)
	if err != nil {
		return nil, 0
	}
	source := ownSource(srcs, connector)
	if source == "" {
		return nil, 0
	}
	f, _, err := sourceFields(ctx, cwm, source)
	if err != nil {
		return nil, 0
	}
	return f.Keywords, f.ProposeMax
}

// withKeywords are the terms and a source's own keywords, each once.
func withKeywords(terms, keywords []string) []string {
	if len(keywords) == 0 {
		return terms
	}
	out := append([]string{}, terms...)
	seen := map[string]bool{}
	for _, t := range terms {
		seen[strings.ToLower(strings.TrimSpace(t))] = true
	}
	for _, kw := range keywords {
		if k := strings.ToLower(strings.TrimSpace(kw)); k != "" && !seen[k] {
			seen[k] = true
			out = append(out, kw)
		}
	}
	return out
}

// bounded cuts a connector's proposal to the bound its source asks for.
func bounded(out api.ConnectorProposeOutput, max int) api.ConnectorProposeOutput {
	if b := api.ProposeBound(max); len(out.Items) > b {
		out.Items = out.Items[:b]
	}
	return out
}
