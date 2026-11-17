// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	paneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// : the « Sources » board draws one card per connector - its
// state in a word and a colour, a meta line, the titles of the items its
// source keeps, a note saying what to do, the connector's colour, its source
// - and the panel lays the board out as cards with « Choisir… », « ← Toutes
// les sources », « et {n} autres » and « Actualiser » / « Retirer » on a used
// card only.
func TestTheSourcesBoardDrawsOneCardPerSource(t *testing.T) {
	ctx, cwm, svc, _, _ := knowledgeWorld(t)
	used, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: "wiki-page"})
	if err != nil || used.Source == "" {
		t.Fatalf("use wiki-page: %v %+v", err, used)
	}
	set := func(state string, f func(*api.InformationSourceFields)) {
		t.Helper()
		got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: used.Source})
		if err != nil {
			t.Fatal(err)
		}
		var fields api.InformationSourceFields
		_ = json.Unmarshal(got.Object.FieldsJSON(), &fields)
		f(&fields)
		raw, _ := json.Marshal(fields)
		got.Object.SetFieldsJSON(raw)
		if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "m", Object: got.Object}); err != nil {
			t.Fatal(err)
		}
		_, err = cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: used.Source, To: state, By: cwmapi.Actor{ID: "sources", Kind: cwmapi.ActorAgent}})
		if err != nil && !strings.Contains(err.Error(), "no transition from "+state+" to "+state) {
			t.Fatal(err)
		}
	}
	cards := func() map[string]api.ConnectorUse {
		t.Helper()
		of, err := svc.SourcesOf(ctx, api.SourcesOfInput{ID: "m"})
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]api.ConnectorUse{}
		for _, c := range of.Connectors {
			out[c.Name] = c
		}
		return out
	}
	stateOf := func(c api.ConnectorUse) string {
		if c.State == nil {
			return "<none>"
		}
		return c.State.Text + "/" + c.State.Color
	}

	// Used, never read, locating and picking nothing: « à configurer »
	//
	got := cards()
	wiki, folder := got["wiki-page"], got["local-folder"]
	if stateOf(wiki) != "à configurer/gray" || wiki.Note != "Choisir… pour dire quoi lire" || wiki.Meta != "jamais lue" || wiki.Color != "grape" || wiki.Source != used.Source {
		t.Errorf("a used source never read: %+v state %s", wiki, stateOf(wiki))
	}
	// An unused connector: « non utilisée », its description, gray, no source.
	if stateOf(folder) != "non utilisée/gray" || folder.Meta == "" || folder.Meta != folder.Description || folder.Color != "gray" || folder.Source != "" || folder.Note != "" || len(folder.Items) != 0 {
		t.Errorf("an unused connector's card: %+v state %s", folder, stateOf(folder))
	}
	// The list layout's chips stay as they were.
	if strings.Join(wiki.Chips, ",") != "utilisée" || strings.Join(folder.Chips, ",") != "non utilisée" {
		t.Errorf("the chips changed: %v %v", wiki.Chips, folder.Chips)
	}

	read := "2026-11-02T09:30:00Z"
	at, _ := time.Parse(time.RFC3339, read)
	readMeta := "12 éléments · lu le " + at.Local().Format("02/01 15:04")
	set(api.SourceStateSynced, func(f *api.InformationSourceFields) {
		f.LastSynced, f.Items = read, 12
		f.Held = []api.SelectedItem{{Key: "erp", Title: "ERP Acme"}, {Key: "crm"}}
	})
	wiki = cards()["wiki-page"]
	if stateOf(wiki) != "à jour/green" || wiki.Meta != readMeta || strings.Join(wiki.Items, ",") != "ERP Acme,crm" || wiki.Note != "" {
		t.Errorf("a synced source: %+v state %s, want à jour/green, %q, its kept titles (the key when untitled)", wiki, stateOf(wiki), readMeta)
	}

	set(api.SourceStateStale, func(*api.InformationSourceFields) {})
	if wiki = cards()["wiki-page"]; stateOf(wiki) != "changée/yellow" || !strings.Contains(wiki.Note, "Actualiser") {
		t.Errorf("a stale source: %+v state %s", wiki, stateOf(wiki))
	}

	set(api.SourceStateFailed, func(f *api.InformationSourceFields) {
		f.Error = "Demandez l'accès à l'espace."
		f.States = map[string]api.ItemState{"erp": {State: api.ItemFailed, Why: "403"}, "crm": {State: api.ItemFailed, Why: "403"}, "x": {State: api.ItemRead}}
	})
	if wiki = cards()["wiki-page"]; stateOf(wiki) != "2 erreurs/red" || wiki.Note != "Demandez l'accès à l'espace." {
		t.Errorf("a source with 2 failed items: %+v state %s", wiki, stateOf(wiki))
	}

	set(api.SourceStateFailed, func(f *api.InformationSourceFields) {
		f.Error, f.States = "Le serveur ne répond pas.", nil
	})
	if wiki = cards()["wiki-page"]; stateOf(wiki) != "injoignable/red" || wiki.Note != "Le serveur ne répond pas." {
		t.Errorf("a source whose whole read failed: %+v state %s", wiki, stateOf(wiki))
	}

	// The panel: the board laid out as cards, its labels, the two actions
	// on the row's source, shown on a used card only - `when` kept as the
	// panel is served.
	panels, err := mbapi.QueryTyped[paneapi.PanelDecl](ctx, mbapi.Q{Reach: mbapi.ReachSelf, Kind: paneapi.ObjectPanelKind})
	if err != nil {
		t.Fatal(err)
	}
	var tab *paneapi.PanelDecl
	for i := range panels {
		if strings.HasSuffix(panels[i].URI, "/sources") {
			tab = &panels[i].Body
		}
	}
	if tab == nil || tab.Content == nil {
		t.Fatal("no Sources panel")
	}
	board := findNode(tab.Content.DSLTree, "sources-board")
	if board == nil {
		t.Fatal("no sources-board in the Sources panel")
	}
	for k, want := range map[string]string{"layout": "cards", "openLabel": "Choisir…", "backLabel": "← Toutes les sources", "moreLabel": "et {n} autres"} {
		if board[k] != want {
			t.Errorf("sources-board %s = %v, want %q", k, board[k], want)
		}
	}
	acts, _ := board["actions"].([]any)
	labels := []string{}
	for _, a := range acts {
		m, _ := a.(map[string]any)
		args, _ := m["args"].(map[string]any)
		if m["when"] != "used" || args["id"] != "${item.source}" {
			t.Errorf("the board's action %v is not on the row's source, used only", m)
		}
		labels = append(labels, m["label"].(string))
	}
	if strings.Join(labels, ",") != "Actualiser,Retirer" {
		t.Errorf("the board's actions = %v, want Actualiser, Retirer", labels)
	}
	if err := render.ValidatePage(tab.Content.DSLTree); err != nil {
		t.Errorf("the Sources panel does not validate: %v", err)
	}
}

// findNode is the node of a page tree with the id.
func findNode(n any, id string) map[string]any {
	switch v := n.(type) {
	case map[string]any:
		if v["id"] == id {
			return v
		}
		for _, c := range v {
			if got := findNode(c, id); got != nil {
				return got
			}
		}
	case []any:
		for _, c := range v {
			if got := findNode(c, id); got != nil {
				return got
			}
		}
	}
	return nil
}
