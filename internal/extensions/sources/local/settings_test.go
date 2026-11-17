// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
	schemalocal "github.com/katyoti/katyoti-fwk/internal/extensions/schema/local"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// : an opened source's « Réglages » are one form read in rows
// each value edited in place and kept at once (mock-up v6 vSourceSettings):
// the rows it only reads have no pencil, a pencil opens what it names, the
// connection row carries « Tester », « Supprimer la source » sits under it;
// SettingsSave keeps its own keywords, proposal bound and re-reading mode -
// a proposal then searches its keywords too -; SettingsTest records the
// connection.
func TestAnOpenedSourcesSettingsAreEditedInPlace(t *testing.T) {
	ctx, cwm, svc, _ := pickWorld(t)
	used, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: "apps"})
	if err != nil {
		t.Fatal(err)
	}
	settings := func() (map[string]any, api.SourceSettings, string) {
		t.Helper()
		out, err := svc.SourceTab(ctx, api.SourceTabInput{ID: used.Source, Tab: api.SourceTabSettings})
		if err != nil {
			t.Fatal(err)
		}
		if err := render.ValidatePage(out.DSLTree); err != nil {
			t.Errorf("the settings tab is no valid page: %v", err)
		}
		raw, _ := json.Marshal(out.DSLTree)
		var tree map[string]any
		_ = json.Unmarshal(raw, &tree)
		var form map[string]any
		var walk func(any)
		walk = func(n any) {
			switch v := n.(type) {
			case map[string]any:
				if v["kind"] == "form" && v["id"] == "source-settings-form" {
					form = v
				}
				for _, c := range v {
					walk(c)
				}
			case []any:
				for _, c := range v {
					walk(c)
				}
			}
		}
		walk(tree)
		if form == nil {
			t.Fatalf("no settings form: %s", raw)
		}
		value, _ := out.Data["settings"].(api.SourceSettings)
		return form, value, string(raw)
	}

	form, value, raw := settings()
	ui := form["ui"].(map[string]any)
	if ui["edit"] != "row" || ui["save"] != "change" || form["dataField"] != "$.settings" {
		t.Errorf("the settings are read in rows and kept at each change: %+v", ui)
	}
	submit := form["submit"].(map[string]any)
	if submit["uri"] != api.SettingsSaveURI || submit["args"].(map[string]any)["id"] != used.Source {
		t.Errorf("the form keeps its value by SettingsSave: %+v", submit)
	}
	rows := map[string]map[string]any{}
	var order []string
	for _, f := range ui["groups"].([]any)[0].(map[string]any)["fields"].([]any) {
		spec := f.(map[string]any)
		rows[spec["id"].(string)] = spec
		order = append(order, spec["id"].(string))
	}
	if strings.Join(order, ",") != "reads,searches_by,keywords,propose_max,takes,sync,connection" {
		t.Errorf("the rows of mock-up v6, in its order: %v", order)
	}
	for id, hint := range map[string]string{"searches_by": "ce qu'elle propose", "keywords": "pour cette source seulement",
		"propose_max": "une recherche bornée par source", "sync": "pour voir ce qui a changé"} {
		if rows[id]["description"] != hint {
			t.Errorf("the row %s reads the mock-up's hint %q: %+v", id, hint, rows[id]["description"])
		}
	}
	if rows["reads"]["readOnly"] != true || rows["keywords"]["readOnly"] != nil || rows["propose_max"]["readOnly"] != nil {
		t.Errorf("only what it reads has no pencil: %+v", rows)
	}
	if nav, _ := rows["searches_by"]["action"].(map[string]any)["navigate"].(map[string]any); nav["object"] != "m" {
		t.Errorf("« Cherche d'après » opens the holder: %+v", rows["searches_by"])
	}
	if nav, _ := rows["takes"]["action"].(map[string]any)["navigate"].(map[string]any); nav["object"] != used.Source || nav["panel"] != "information-source-take" {
		t.Errorf("« Ce qu'elle prend » opens the take tab: %+v", rows["takes"])
	}
	if b, _ := rows["connection"]["button"].(map[string]any); b["label"] != "Tester" || b["uri"] != api.SettingsTestURI {
		t.Errorf("the connection row carries « Tester »: %+v", rows["connection"])
	}
	if len(rows["sync"]["options"].([]any)) != 4 {
		t.Errorf("« Relire » lists four modes: %+v", rows["sync"])
	}
	if !strings.Contains(raw, `"id":"source-settings-remove"`) || !strings.Contains(raw, `"uri":"`+api.RemoveURI+`"`) {
		t.Errorf("« Supprimer la source » sits under the settings: %s", raw)
	}
	if !strings.Contains(raw, `"content":"ce qu'elle a apporté sort de la base de connaissance","id":"source-settings-remove-note"`) ||
		!strings.Contains(raw, `"icon":"trash","id":"source-settings-remove"`) || !strings.Contains(raw, `"label":"Supprimer la source","size":"compact-xs"`) || !strings.Contains(raw, `"variant":"subtle"`) {
		t.Errorf("« Supprimer la source » is a quiet trash button, what it does said beside it: %s", raw)
	}
	if value.Reads != "Applications" || value.ProposeMax != api.ProposeMax || value.Sync != api.SyncManual ||
		value.Connection == nil || value.Connection.State != "Pas encore testée" || value.Connection.TestedAt != "" {
		t.Errorf("the value bound: %+v", value)
	}
	// Each value read as mock-up v6 words it: the connector's brand mark
	// before what it reads, the holder's keywords after a muted lead, the
	// bound in the picker's unit, the connection a toned pill and when it
	// was tested said from now.
	read := func(id string) map[string]any {
		r, _ := rows[id]["read"].(map[string]any)
		return r
	}
	if m, _ := read("reads")["mark"].(map[string]any); m["text"] != "Applications" || m["color"] != "var(--mantine-color-gray-6)" {
		t.Errorf("« Ce qu'elle lit » draws the connector's mark: %+v", rows["reads"])
	}
	if lead, _ := read("searches_by")["lead"].(string); !strings.HasPrefix(lead, "les mots-clés d") {
		t.Errorf("« Cherche d'après » leads with the holder's keywords: %+v", rows["searches_by"])
	}
	if read("propose_max")["template"] != "Au plus {value} applications" {
		t.Errorf("« Propositions » reads « Au plus 20 applications »: %+v", rows["propose_max"])
	}
	pill, _ := read("connection")["pill"].(map[string]any)
	after, _ := read("connection")["after"].(map[string]any)
	if tones, _ := pill["tones"].(map[string]any); pill["path"] != "state" || tones["Joignable"] != "teal" ||
		after["path"] != "tested_at" || after["as"] != "relative-time" || after["prefix"] != "testée" {
		t.Errorf("« Connexion » is a pill and its relative time: %+v", rows["connection"])
	}

	// Kept in place: its own keywords (each once), its bound, its mode.
	got, err := svc.SettingsSave(ctx, api.SettingsSaveInput{ID: used.Source, Settings: api.SourceSettings{
		Reads: "ignored", Keywords: []string{" Stock ", "stock", "Acme"}, ProposeMax: 5, Sync: api.SyncDailyOnOpen}})
	if err != nil || !got.OK {
		t.Fatalf("save: %v %+v", err, got)
	}
	_, value, _ = settings()
	if strings.Join(value.Keywords, ",") != "Stock,Acme" || value.ProposeMax != 5 || value.Sync != api.SyncDailyOnOpen || value.Reads != "Applications" {
		t.Errorf("the settings kept: %+v", value)
	}
	for _, bad := range []api.SourceSettings{{ProposeMax: 101}, {ProposeMax: -1}, {Sync: "weekly"}} {
		if got, err := svc.SettingsSave(ctx, api.SettingsSaveInput{ID: used.Source, Settings: bad}); err != nil || got.OK {
			t.Errorf("%+v is refused: %v %+v", bad, err, got)
		}
	}

	// A proposal of the holder searches its keywords too, each once.
	prop, err := svc.Propose(ctx, api.ProposeInput{Connector: "apps", Object: "m", Terms: []string{"Acme"}})
	if err != nil || len(prop.Items) != 1 || prop.Items[0].Why != "named in Acme, Stock" {
		t.Errorf("the proposal searches the source's keywords: %v %+v", err, prop)
	}

	// « Tester » records when and whether it answered.
	tested, err := svc.SettingsTest(ctx, api.SettingsTestInput{ID: used.Source, Value: "Pas encore testée"})
	if err != nil || !tested.OK || tested.Text != "the map answers" {
		t.Errorf("test: %v %+v", err, tested)
	}
	_, value, _ = settings()
	if c := value.Connection; c == nil || c.State != "Joignable" || c.TestedAt == "" {
		t.Errorf("the connection as last tested: %+v", c)
	}
	src, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: used.Source})
	if err != nil {
		t.Fatal(err)
	}
	var f api.InformationSourceFields
	_ = json.Unmarshal(src.Object.FieldsJSON(), &f)
	if f.TestedAt == "" || !f.Reachable || f.ProposeMax != 5 {
		t.Errorf("the source's fields: %+v", f)
	}
}

// : daily-on-open is due both every day and when its carrier
// opens; a proposal bound out of 1..100 is the default.
func TestDailyOnOpenIsDueInBothModes(t *testing.T) {
	for _, c := range []struct {
		sync, mode string
		due        bool
	}{
		{api.SyncDailyOnOpen, api.SyncDaily, true}, {api.SyncDailyOnOpen, api.SyncOnOpen, true},
		{api.SyncDaily, api.SyncOnOpen, false}, {api.SyncOnOpen, api.SyncOnOpen, true}, {"", api.SyncDaily, false},
	} {
		if got := api.SyncIn(c.sync, c.mode); got != c.due {
			t.Errorf("SyncIn(%q, %q) = %v", c.sync, c.mode, got)
		}
	}
	for n, want := range map[int]int{0: api.ProposeMax, 7: 7, 100: 100, 101: api.ProposeMax, -3: api.ProposeMax} {
		if got := api.ProposeBound(n); got != want {
			t.Errorf("ProposeBound(%d) = %d, want %d", n, got, want)
		}
	}
}

// : a connector declaring scopes has its scopes row right
// after « Ce qu'elle lit », as mock-up v6 draws « Espace ».
func TestTheScopesRowFollowsWhatASourceReads(t *testing.T) {
	ctx, _, svc, _ := pickWorldWith(t, map[string]string{"pages.yaml": `name: pages
label: Pages
description: The pages of a wiki.
sync: ktw-apps:pages/source-sync
search: ktw-apps:pages/search
scopes: {unit: espace, list: ktw-apps:pages/source-scopes}
picker: {unit: page, layout: list}
`})
	used, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: "pages"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := svc.SourceTab(ctx, api.SourceTabInput{ID: used.Source, Tab: api.SourceTabSettings})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out.DSLTree)
	var order []string
	var walk func(any)
	walk = func(n any) {
		switch v := n.(type) {
		case map[string]any:
			if v["kind"] == "form" && v["id"] == "source-settings-form" {
				for _, f := range v["ui"].(map[string]any)["groups"].([]any)[0].(map[string]any)["fields"].([]any) {
					order = append(order, f.(map[string]any)["id"].(string))
				}
			}
			for _, c := range v {
				walk(c)
			}
		case []any:
			for _, c := range v {
				walk(c)
			}
		}
	}
	var tree any
	_ = json.Unmarshal(raw, &tree)
	walk(tree)
	if len(order) < 3 || order[0] != "reads" || order[1] != "scopes" || order[2] != "searches_by" {
		t.Errorf("the scopes row right after what it reads: %v", order)
	}
	if strings.Contains(string(raw), "où elle cherche") {
		t.Errorf("the scopes row carries no hint, as mock-up v6 draws « Espace »: %s", raw)
	}
}

// settingsRows are an opened source's « Réglages » rows by id, in order,
// and the value its form binds.
func settingsRows(t *testing.T, ctx context.Context, svc api.SourcesService, source string) (map[string]map[string]any, []string, api.SourceSettings) {
	t.Helper()
	out, err := svc.SourceTab(ctx, api.SourceTabInput{ID: source, Tab: api.SourceTabSettings})
	if err != nil {
		t.Fatal(err)
	}
	if err := render.ValidatePage(out.DSLTree); err != nil {
		t.Errorf("the settings tab is no valid page: %v", err)
	}
	raw, _ := json.Marshal(out.DSLTree)
	var tree any
	_ = json.Unmarshal(raw, &tree)
	rows := map[string]map[string]any{}
	var order []string
	var walk func(any)
	walk = func(n any) {
		switch v := n.(type) {
		case map[string]any:
			if v["kind"] == "form" && v["id"] == "source-settings-form" {
				for _, f := range v["ui"].(map[string]any)["groups"].([]any)[0].(map[string]any)["fields"].([]any) {
					spec := f.(map[string]any)
					rows[spec["id"].(string)] = spec
					order = append(order, spec["id"].(string))
				}
			}
			for _, c := range v {
				walk(c)
			}
		case []any:
			for _, c := range v {
				walk(c)
			}
		}
	}
	walk(tree)
	value, _ := out.Data["settings"].(api.SourceSettings)
	return rows, order, value
}

// : a connector naming the connection a source reads through
// gets its row on « Réglages » under its label, before « Relire » as mock-up
// v6 draws « Environnement », its choices asked of the connector's list; the
// choice is kept in the location's connection field, none deleting it.
func TestAConnectorsConnectionRowChoosesWhatASourceReadsThrough(t *testing.T) {
	ctx, cwm, svc, _ := pickWorldWith(t, map[string]string{"envs.yaml": `name: envs
label: Entities
description: The entities of an environment.
sync: ktw-apps:envs/source-sync
connection: {label: Environnement, list: ktw-apps:envs/connections}
`})
	used, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: "envs"})
	if err != nil {
		t.Fatal(err)
	}
	rows, order, value := settingsRows(t, ctx, svc, used.Source)
	if strings.Join(order, ",") != "reads,through,sync" {
		t.Errorf("the connection row before « Relire »: %v", order)
	}
	row := rows["through"]
	if row["label"] != "Environnement" {
		t.Errorf("the row reads the connector's label: %+v", row)
	}
	if ch, _ := row["choices"].(map[string]any); ch["uri"] != "ktw-apps:envs/connections" {
		t.Errorf("its choices are asked of the connector's list: %+v", row)
	}
	if opts, _ := row["options"].([]any); len(opts) != 1 || opts[0].(map[string]any)["value"] != "" {
		t.Errorf("the universe's configuration until the list answers: %+v", row)
	}
	if value.Through != "" {
		t.Errorf("no connection named yet: %+v", value)
	}
	location := func() map[string]any {
		t.Helper()
		src, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: used.Source})
		if err != nil {
			t.Fatal(err)
		}
		var f api.InformationSourceFields
		_ = json.Unmarshal(src.Object.FieldsJSON(), &f)
		return f.Location
	}
	if got, err := svc.SettingsSave(ctx, api.SettingsSaveInput{ID: used.Source, Settings: api.SourceSettings{Through: " prod "}}); err != nil || !got.OK {
		t.Fatalf("save: %v %+v", err, got)
	}
	if api.ConnectionOf(location()) != "prod" {
		t.Errorf("the choice kept in the location's connection field: %+v", location())
	}
	if _, _, value = settingsRows(t, ctx, svc, used.Source); value.Through != "prod" {
		t.Errorf("the row reads the kept connection: %+v", value)
	}
	if got, err := svc.SettingsSave(ctx, api.SettingsSaveInput{ID: used.Source, Settings: api.SourceSettings{}}); err != nil || !got.OK {
		t.Fatalf("save: %v %+v", err, got)
	}
	if _, set := location()[api.LocationConnection]; set {
		t.Errorf("no connection deletes the field: %+v", location())
	}

	// Without the declaration: no row, and the stored field left alone.
	used, err = svc.Use(ctx, api.UseInput{ID: "m", Connector: "apps"})
	if err != nil {
		t.Fatal(err)
	}
	if rows, _, _ := settingsRows(t, ctx, svc, used.Source); rows["through"] != nil {
		t.Errorf("no connection row for a connector naming none: %+v", rows["through"])
	}
}

// : « Espace » lists its choices from the connector's scopes
// list - the row asks scope-options for the source, which answers each
// scope as an option, its key the value.
func TestTheScopesRowListsTheConnectorsScopes(t *testing.T) {
	ctx, _, svc, _ := pickWorldWith(t, map[string]string{"spaces.yaml": scopedSpaces})
	used, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: "spaces"})
	if err != nil {
		t.Fatal(err)
	}
	rows, _, _ := settingsRows(t, ctx, svc, used.Source)
	ch, _ := rows["scopes"]["choices"].(map[string]any)
	if ch["uri"] != api.ScopeOptionsURI || ch["args"].(map[string]any)["id"] != used.Source {
		t.Errorf("the scopes row asks scope-options for its source: %+v", rows["scopes"])
	}
	if got, err := svc.SettingsSave(ctx, api.SettingsSaveInput{ID: used.Source, Settings: api.SourceSettings{Scopes: []string{"ARCHI"}}}); err != nil || !got.OK {
		t.Fatalf("save: %v %+v", err, got)
	}
	opts, err := svc.ScopeOptions(ctx, api.ScopeOptionsInput{ID: used.Source})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, o := range opts.Options {
		got = append(got, o.Value+"="+o.Label)
	}
	if strings.Join(got, ",") != "AUP=AUP,ARCHI=ARCHI" {
		t.Errorf("the connector's scopes as options, the source's own among them: %v", got)
	}
	if _, err := svc.ScopeOptions(ctx, api.ScopeOptionsInput{}); err == nil {
		t.Error("an id is required")
	}
}

// zonesLocation is a picker connector's location data-type whose settings
// ui-form names the fields « Réglages » edits.
const zonesLocation = `$schema: ktw-schema/v1
name: zones-location
version: "1.0.0"
root: ZonesLocation
types:
  ZonesLocation:
    type: object
    fields:
      kinds:
        type: array
        items: string
        optional: true
        label: Types d'entités
      zone:
        type: string
        optional: true
      selection:
        type: array
        items: string
        optional: true
    extensions:
      - type: ui-form
        name: settings
        groups:
          - fields: [kinds, {id: zone, description: where it reads}]
`

// : a picker connector's location fields its data-type's
// settings ui-form names are rows of « Réglages » (« Types d'entités »),
// after the connection row and before « Relire », typed by the data-type,
// saved into the location - the rest of it kept, a field it does not name
// never written -; no « Où elle lit » form is drawn for it.
func TestAPickerConnectorsLocationFieldsAreRowsOfItsSettings(t *testing.T) {
	ctx, cwm, svc, _ := pickWorldExts(t, map[string]string{"zones.yaml": `name: zones
label: Zones
description: The entities of a zone.
sync: ktw-apps:zones/source-sync
location: ktw-apps:/data-type/zones-location
connection: {label: Environnement}
picker: {unit: entité, gender: feminine, layout: list}
`}, schemalocal.Extension())
	schemas, err := application.UseService[schemaapi.SchemaService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := schemas.RegisterDataType(ctx, schemaapi.RegisterDataTypeInput{URI: "ktw-apps:/data-type/zones-location", Body: []byte(zonesLocation)}); err != nil {
		t.Fatal(err)
	}
	used, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: "zones"})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := svc.LocationSave(ctx, api.LocationSaveInput{ID: used.Source, Location: map[string]any{"zone": "north", "selection": []any{"a"}}}); err != nil || !got.OK {
		t.Fatalf("location: %v %+v", err, got)
	}

	rows, order, value := settingsRows(t, ctx, svc, used.Source)
	if strings.Join(order, ",") != "reads,takes,through,location.kinds,location.zone,sync" {
		t.Errorf("the location rows after the connection, before « Relire »: %v", order)
	}
	if rows["location.kinds"]["label"] != "Types d'entités" || rows["location.zone"]["description"] != "where it reads" {
		t.Errorf("each row as the data-type labels it: %+v %+v", rows["location.kinds"], rows["location.zone"])
	}
	if value.Location["zone"] != "north" || value.Location["selection"] != nil {
		t.Errorf("the form binds the fields it edits only: %+v", value.Location)
	}
	out, err := svc.SourceTab(ctx, api.SourceTabInput{ID: used.Source, Tab: api.SourceTabSettings})
	if err != nil {
		t.Fatal(err)
	}
	tree, _ := json.Marshal(out.DSLTree)
	if !strings.Contains(string(tree), `"location":{`) || strings.Contains(string(tree), `"title":"Où elle lit"`) {
		t.Errorf("the form types its location, no « Où elle lit » form: %s", tree)
	}

	if got, err := svc.SettingsSave(ctx, api.SettingsSaveInput{ID: used.Source, Settings: api.SourceSettings{Through: "prod",
		Location: map[string]any{"kinds": []any{"service", "host"}, "selection": []any{"x"}}}}); err != nil || !got.OK {
		t.Fatalf("save: %v %+v", err, got)
	}
	src, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: used.Source})
	if err != nil {
		t.Fatal(err)
	}
	var f api.InformationSourceFields
	_ = json.Unmarshal(src.Object.FieldsJSON(), &f)
	kinds, _ := f.Location["kinds"].([]any)
	if len(kinds) != 2 || f.Location["zone"] != nil || api.ConnectionOf(f.Location) != "prod" {
		t.Errorf("the edited fields kept, a cleared one removed, the connection kept: %+v", f.Location)
	}
	if sel, _ := f.Location["selection"].([]any); len(sel) != 1 || sel[0] != "a" {
		t.Errorf("a field the settings do not name is never written: %+v", f.Location)
	}
}
