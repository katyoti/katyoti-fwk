// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// places declares the form of its location: a data-type of its pack, whose
// ui-form extension holds its groups, secrets and advanced fields.
const places = `name: places
label: Places
description: The places of the map.
sync: ktw-apps:places/source-sync
search: ktw-apps:places/search
location: ktw-apps:/data-type/places-location
`

// : a used connector's panel draws
// where its source reads as the generic form of the connector's location
// data-type - nothing in the panel names a field: the groups, the secrets
// and the folded advanced fields are the data-type's hints -, bound to the
// source's location and kept by LocationSave; a connector declaring no
// location draws none.
func TestAConnectorPanelDrawsTheSourcesLocationFromItsConnectorsForm(t *testing.T) {
	ctx, _, svc, _ := pickWorldWith(t, map[string]string{"places.yaml": places})
	used, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: "places"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: "apps"}); err != nil {
		t.Fatal(err)
	}
	panel := func(connector string) (string, map[string]any) {
		t.Helper()
		got, err := svc.ConnectorPanel(ctx, api.ConnectorPanelInput{ID: "m", Connector: connector})
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(got.DSLTree)
		return string(raw), got.Data
	}

	tree, data := panel("places")
	for _, want := range []string{`"kind":"form"`, `"schema":"ktw-apps:/data-type/places-location"`, `"dataField":"$.location"`,
		`"uri":"` + api.LocationSaveURI + `"`, `"id":"` + used.Source + `"`, `"location":"${value}"`} {
		if !strings.Contains(tree, want) {
			t.Errorf("the location form lacks %s: %s", want, tree)
		}
	}
	if loc, ok := data["location"].(map[string]any); !ok || len(loc) != 0 {
		t.Errorf("a source with no location yet binds an empty one: %+v", data["location"])
	}
	if tree, _ := panel("apps"); strings.Contains(tree, `"kind":"form"`) {
		t.Errorf("a connector declaring no location draws no form: %s", tree)
	}

	saved, err := svc.LocationSave(ctx, api.LocationSaveInput{ID: used.Source, Location: map[string]any{"region": "Nord", "token": "s3cret"}})
	if err != nil || !saved.OK {
		t.Fatalf("the form's value is kept: %+v %v", saved, err)
	}
	if _, data = panel("places"); !reflect.DeepEqual(data["location"], map[string]any{"region": "Nord", "token": "s3cret"}) {
		t.Errorf("the panel binds the kept location: %+v", data["location"])
	}
	if _, err := svc.LocationSave(ctx, api.LocationSaveInput{}); err == nil {
		t.Error("a location for no source is refused")
	}
}

// : sources' own local-folder connector declares its location
// form - the folder it mirrors - as a data-type of its pack.
func TestTheLocalFolderDeclaresItsLocationForm(t *testing.T) {
	ctx, _, svc, _ := pickWorld(t)
	list, err := svc.Connectors(ctx, api.ConnectorsInput{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range list.Connectors {
		if c.Name == "local-folder" {
			if c.Location != "ktw-agents:/data-type/local-folder-location" {
				t.Fatalf("local-folder's location form: %q", c.Location)
			}
			return
		}
	}
	t.Fatal("local-folder is not contributed")
}

// : a picker that only declares its words - its unit and
// their gender - keeps the connector's location form in « Réglages » and on
// its panel; a picker that selects (a layout, badges, what is read with each
// item) replaces it.
func TestAWordsOnlyPickerKeepsTheLocationForm(t *testing.T) {
	ctx, _, svc, _ := pickWorldWith(t, map[string]string{
		"spots.yaml": `name: spots
label: Spots
description: The spots a selector names.
sync: ktw-apps:spots/source-sync
location: ktw-apps:/data-type/spots-location
picker: {unit: entité, gender: feminine}
`,
		"chosen.yaml": `name: chosen
label: Chosen
description: The spots one picks.
sync: ktw-apps:chosen/source-sync
search: ktw-apps:chosen/search
location: ktw-apps:/data-type/chosen-location
picker: {unit: entité, gender: feminine, layout: list}
`})
	for _, tc := range []struct {
		connector string
		form      bool
	}{{"spots", true}, {"chosen", false}} {
		used, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: tc.connector})
		if err != nil {
			t.Fatal(err)
		}
		settings, err := svc.SourceTab(ctx, api.SourceTabInput{ID: used.Source, Tab: api.SourceTabSettings})
		if err != nil {
			t.Fatal(err)
		}
		panel, err := svc.ConnectorPanel(ctx, api.ConnectorPanelInput{ID: "m", Connector: tc.connector})
		if err != nil {
			t.Fatal(err)
		}
		for name, out := range map[string]api.ConnectorPanelOutput{"settings": settings, "panel": panel} {
			raw, _ := json.Marshal(out.DSLTree)
			tree := string(raw)
			has := strings.Contains(tree, "Où elle lit") && strings.Contains(tree, `"schema":"ktw-apps:/data-type/`+tc.connector+`-location"`)
			if _, bound := out.Data["location"]; has != tc.form || bound != tc.form {
				t.Errorf("%s %s: location form %v (bound %v), want %v: %s", tc.connector, name, has, bound, tc.form, tree)
			}
		}
	}
	if (*api.ConnectorPicker)(nil).Selects() {
		t.Error("no picker selects nothing")
	}
}
