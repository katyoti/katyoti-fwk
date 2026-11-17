// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	paneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// : an opened source shows three sub-tabs under a holder that
// asks for them - what it takes (the connector's picker bound to the
// source's module, its selection, « Appliquer »), « Contenu lu » (the
// source's detail and its items) and « Réglages » (its settings rows, the
// connector's location form when it has no picker) -, each a panel on information-source when source_sub_tabs(),
// loading sources/source-tab with the source's id and its tab; the one
// « Source » tab stands for them when source_sub_tabs() == false
func TestAnOpenedSourceShowsThreeSubTabs(t *testing.T) {
	ctx, _, svc, _ := pickWorldWith(t, map[string]string{"places.yaml": places})
	places, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: "places"})
	if err != nil {
		t.Fatal(err)
	}
	apps, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: "apps"})
	if err != nil {
		t.Fatal(err)
	}
	tab := func(id, name string) (string, map[string]any) {
		t.Helper()
		out, err := svc.SourceTab(ctx, api.SourceTabInput{ID: id, Tab: name})
		if err != nil {
			t.Fatalf("%s %s: %v", id, name, err)
		}
		if err := render.ValidatePage(out.DSLTree); err != nil {
			t.Errorf("%s %s is no valid page: %v", id, name, err)
		}
		raw, _ := json.Marshal(out.DSLTree)
		return string(raw), out.Data
	}

	// What it takes: the picker bound to the module, with the selection.
	take, data := tab(apps.Source, api.SourceTabTake)
	for _, want := range []string{`"kind":"picker"`, `"object":"m"`, `"connector":"apps"`, `"apply":"ktw-agents:sources/hold"`, `"dataField":"$.selected"`} {
		if !strings.Contains(take, want) {
			t.Errorf("the take tab lacks %s: %s", want, take)
		}
	}
	if _, ok := data["selected"].([]any); !ok || strings.Contains(take, `"source-detail"`) || strings.Contains(take, `"kind":"form"`) {
		t.Errorf("the take tab binds the selection and holds only the picker: %s %+v", take, data)
	}

	// « Contenu lu »: the detail and its items, no picker.
	read, data := tab(apps.Source, api.SourceTabRead)
	if !strings.Contains(read, `"id":"source-detail"`) || !strings.Contains(read, `"id":"source-detail-items"`) || strings.Contains(read, `"kind":"picker"`) {
		t.Errorf("the read tab is the source's detail: %s", read)
	}
	if d, _ := data["detail"].(map[string]any); d == nil || d["items"] == nil {
		t.Errorf("the read tab binds the detail: %+v", data)
	}

	// « Réglages »: the connector's location form, kept by LocationSave; a
	// connector declaring no location says so.
	settings, data := tab(places.Source, api.SourceTabSettings)
	for _, want := range []string{`"kind":"form"`, `"schema":"ktw-apps:/data-type/places-location"`, `"uri":"` + api.LocationSaveURI + `"`, `"id":"` + places.Source + `"`} {
		if !strings.Contains(settings, want) {
			t.Errorf("the settings tab lacks %s: %s", want, settings)
		}
	}
	if _, ok := data["location"].(map[string]any); !ok {
		t.Errorf("the settings tab binds the location: %+v", data)
	}
	if none, _ := tab(apps.Source, api.SourceTabSettings); strings.Contains(none, api.LocationSaveURI) || strings.Contains(none, "Où elle lit") || !strings.Contains(none, `"id":"source-settings"`) {
		t.Errorf("a connector declaring no location draws its settings rows, no location form: %s", none)
	}

	for _, bad := range []api.SourceTabInput{{ID: apps.Source, Tab: "other"}, {Tab: api.SourceTabRead}, {ID: "m", Tab: api.SourceTabRead}} {
		if _, err := svc.SourceTab(ctx, bad); err == nil {
			t.Errorf("%+v is refused", bad)
		}
	}

	// The three panels when source_sub_tabs(), the one « Source » tab -
	// loading source-detail with the source's id - when it is false.
	panels, err := mbapi.QueryTyped[paneapi.PanelDecl](ctx, mbapi.Q{Reach: mbapi.ReachSelf, Kind: paneapi.ObjectPanelKind})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, p := range panels {
		b := p.Body
		if strings.HasSuffix(p.URI, "/information-source") {
			seen["source"] = true
			if strings.Join(b.Filter.Types, ",") != api.InformationSourceType || b.Filter.When != "source_sub_tabs() == false" || b.Content == nil {
				t.Errorf("the « Source » tab: on an information source whose holder does not ask for the sub-tabs: %+v", b.Filter)
				continue
			}
			if err := render.ValidatePage(b.Content.DSLTree); err != nil {
				t.Errorf("the « Source » tab's page: %v", err)
			}
			tree, _ := json.Marshal(b.Content.DSLTree)
			if !strings.Contains(string(tree), `"method":"`+api.SourceDetailURI+`"`) || !strings.Contains(string(tree), `"args":{"id":"${id}"}`) {
				t.Errorf("the « Source » tab's box loads source-detail with the source's id: %s", tree)
			}
			continue
		}
		for _, name := range []string{api.SourceTabTake, api.SourceTabRead, api.SourceTabSettings} {
			if !strings.HasSuffix(p.URI, "/information-source-"+name) {
				continue
			}
			seen[name] = true
			if strings.Join(b.Filter.Types, ",") != api.InformationSourceType || b.Filter.When != "source_sub_tabs()" || b.Content == nil {
				t.Errorf("%s: on an information source whose holder asks for the sub-tabs: %+v", name, b.Filter)
				continue
			}
			tree, _ := json.Marshal(b.Content.DSLTree)
			if !strings.Contains(string(tree), `"method":"`+api.SourceTabURI+`"`) || !strings.Contains(string(tree), `"tab":"`+name+`"`) || !strings.Contains(string(tree), `"id":"${id}"`) {
				t.Errorf("%s loads source-tab with the source's id and its tab: %s", name, tree)
			}
		}
	}
	if len(seen) != 4 {
		t.Errorf("three sub-tabs and the « Source » tab declared, got %v", seen)
	}
}

// : a source opens on its take tab - « Pages prises »
// « Applications », « Entités », a folder's own -, never on « Informations »,
// whatever its connector and wherever it is opened: the pane opens on its
// first tab, and the information-source type lists its main tabs - the take
// tab first, then « Contenu lu » and « Réglages », the one « Source » tab
// under a holder not asking for the sub-tabs, then « Choisir les éléments » - with Information placed after
// them (a listed Information is no longer put first). The list is the type's,
// so it holds for every connector.
func TestASourceOpensOnItsTakeTab(t *testing.T) {
	ctx, cwm, _, _ := sourcesWorld(t)
	got, err := cwm.ObjectTypeGet(ctx, cwmapi.ObjectTypeGetInput{Name: api.InformationSourceType})
	if err != nil {
		t.Fatal(err)
	}
	ps, err := paneapi.PaneOf(got.ObjectType)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"information-source-" + api.SourceTabTake, "information-source-" + api.SourceTabRead, "information-source-" + api.SourceTabSettings, api.InformationSourceType, "source-pick"}
	if strings.Join(ps.Tabs, ",") != strings.Join(want, ",") {
		t.Errorf("an information source's main tabs are %v, want %v: the take tab first, Information behind « Plus… »", ps.Tabs, want)
	}
	if ps.Only || len(ps.Variants) > 0 {
		t.Errorf("the source's tabs are its main tabs, the same under every holder: %+v", ps)
	}
}

// : only a holder carrying source-sub-tabs - the as-is study
// v2 - opens its sources on the three sub-tabs; a module and a v1 study keep
// the one « Source » tab, as before IN-1158. source_sub_tabs(), the
// knowledge capability's function, answers it over the source; an object
// that is no source answers false.
func TestOnlyAHolderAskingForThemShowsTheSubTabs(t *testing.T) {
	ctx, cwm, svc, _ := sourcesWorld(t)
	holder := func(typ, local string) string {
		t.Helper()
		h := cwmapi.NewContainerObject("m", local, typ, "m", local, "")
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: h}); err != nil {
			t.Fatal(err)
		}
		return h.ID
	}
	source := func(parent, local string) string {
		t.Helper()
		obj := cwmapi.NewContainerObject("m", local, api.InformationSourceType, parent, local, "")
		obj.Fields = map[string]any{"connector": api.FolderConnector}
		out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: obj})
		if err != nil {
			t.Fatal(err)
		}
		return out.Object.ID
	}
	subTabs := func(id string) bool {
		t.Helper()
		got, err := svc.SourceSubTabsFunction(ctx, cwmapi.FunctionCall{Function: "source_sub_tabs", Object: id})
		if err != nil {
			t.Fatalf("source_sub_tabs() on %s: %v", id, err)
		}
		return got
	}
	v2 := holder("study-v2", "ventes")
	for name, c := range map[string]struct {
		id   string
		want bool
	}{
		"a v2 study's source":         {source(v2, "ventes-folder"), true},
		"a v1 study's source":         {source(holder("study-v1", "ventes-v1"), "v1-folder"), false},
		"a module's source":           {source("m", "m-folder"), false},
		"an object that is no source": {v2, false},
	} {
		if got := subTabs(c.id); got != c.want {
			t.Errorf("%s: source_sub_tabs() = %v, want %v", name, got, c.want)
		}
	}
}

// : an opened source's « what it takes » tab is named by its
// connector, as mock-up v6 draws it - the take label the connector declares
// (« Fichiers déposés », « Dossier »), « Ce qu'elle prend » when it declares
// none: the panel's name is source_take(), the knowledge capability's
// function, and carries no translated name the GUI would show instead.
func TestTheTakeTabIsNamedByItsConnector(t *testing.T) {
	ctx, _, svc, _ := pickWorldWith(t, nil)
	take := func(id string) string {
		t.Helper()
		got, err := svc.SourceTakeFunction(ctx, cwmapi.FunctionCall{Function: "source_take", Object: id})
		if err != nil {
			t.Fatalf("source_take() on %s: %v", id, err)
		}
		return got
	}
	for connector, want := range map[string]string{"documents": "Fichiers déposés", "local-folder": "Dossier", "apps": api.SourceTakeDefault} {
		used, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: connector})
		if err != nil {
			t.Fatalf("%s: %v", connector, err)
		}
		if got := take(used.Source); got != want {
			t.Errorf("a %s source's take tab is named %q, want %q", connector, got, want)
		}
	}
	if got := take("m"); got != api.SourceTakeDefault {
		t.Errorf("an object that is no source answers the default, got %q", got)
	}

	panels, err := mbapi.QueryTyped[paneapi.PanelDecl](ctx, mbapi.Q{Reach: mbapi.ReachSelf, Kind: paneapi.ObjectPanelKind})
	if err != nil {
		t.Fatal(err)
	}
	named := false
	for _, p := range panels {
		if strings.HasSuffix(p.URI, "/information-source-"+api.SourceTabTake) {
			named = p.Body.Name == "${source_take()}"
		}
	}
	if !named {
		t.Error("the take panel's name is ${source_take()}")
	}
	i18n, err := os.ReadFile(filepath.Join("..", "contrib", "embed", "cwm", "object-panels", "information-source-take.i18n.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(i18n), "name:") {
		t.Error("the take panel declares no translated name: the GUI's would replace the connector's")
	}
}

// : a source whose connector picks its items never asks again
// in « Réglages » for what the picker chose - no location form duplicating
// the selection, even when the connector declares a location - and shows how
// it searches and reads, as mock-up v6 draws it (vSourceSettings): what it
// reads, the study's keywords it searches by, the proposal bound, what it
// takes.
func TestAPickerSourcesSettingsNeverAskAgainForItsSelection(t *testing.T) {
	ctx, _, svc, _ := pickWorldWith(t, map[string]string{"picks.yaml": `name: picks
label: Applications choisies
description: Les applications et le texte de leurs interfaces.
sync: ktw-apps:picks/source-sync
search: ktw-apps:picks/search
propose: ktw-apps:picks/propose
location: ktw-apps:/data-type/picks-location
picker: {unit: application, layout: list}
`})
	used, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: "picks"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := svc.SourceTab(ctx, api.SourceTabInput{ID: used.Source, Tab: api.SourceTabSettings})
	if err != nil {
		t.Fatal(err)
	}
	if err := render.ValidatePage(out.DSLTree); err != nil {
		t.Errorf("the settings tab is no valid page: %v", err)
	}
	raw, _ := json.Marshal(out.DSLTree)
	settings := string(raw)
	for _, bad := range []string{"Où elle lit", api.LocationSaveURI, "-location\""} {
		if strings.Contains(settings, bad) {
			t.Errorf("a picker source's settings ask again for its selection (%s): %s", bad, settings)
		}
	}
	if _, ok := out.Data["location"]; ok {
		t.Errorf("a picker source's settings bind no location: %+v", out.Data)
	}
	if set, _ := out.Data["settings"].(api.SourceSettings); set.Takes != "Les applications et le texte de leurs interfaces." || set.Reads != "Applications choisies" || set.ProposeMax != api.ProposeMax {
		t.Errorf("the settings bind what it reads, takes and its bound: %+v", out.Data["settings"])
	}
	for _, want := range []string{`"id":"source-settings"`, "Ce qu'elle lit", "Cherche d'après", "une recherche bornée par source", "Ce qu'elle prend"} {
		if !strings.Contains(settings, want) {
			t.Errorf("the settings lack %s: %s", want, settings)
		}
	}

	// The board's panel of the same connector draws no location form either.
	panel, err := svc.ConnectorPanel(ctx, api.ConnectorPanelInput{ID: "m", Connector: "picks"})
	if err != nil {
		t.Fatal(err)
	}
	if raw, _ := json.Marshal(panel.DSLTree); strings.Contains(string(raw), `"kind":"form"`) {
		t.Errorf("the board's panel asks again for the selection: %s", raw)
	}
}
