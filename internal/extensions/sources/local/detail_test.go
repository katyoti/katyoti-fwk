// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// : an opened source shows, the same for every source, its
// state in a word, when it was last read, what to do when it failed, then
// each item it brought or keeps with its state - synchronisé, à lire (kept,
// not read yet), à retirer, en échec with why -, an item read opening its
// source-document. The connector panel of a used source - what an opened
// card shows - holds it.
func TestAnOpenedSourceShowsTheItemsItBroughtWithTheirState(t *testing.T) {
	ctx, cwm, svc, _, _ := knowledgeWorld(t)
	used, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: "wiki-page"})
	if err != nil || used.Source == "" {
		t.Fatalf("use wiki-page: %v %+v", err, used)
	}

	// Never read, locating and picking nothing: « à configurer », « jamais
	// lue ».
	empty, err := svc.SourceDetail(ctx, api.SourceDetailInput{ID: used.Source})
	if err != nil {
		t.Fatal(err)
	}
	if d := empty.Data["detail"].(map[string]any); d["state"] != "à configurer" || d["meta"] != "jamais lue" || len(d["items"].([]any)) != 0 || d["label"] != "Wiki page" {
		t.Errorf("a source never read: %+v", d)
	}

	// Read once: erp read, crm failed (403), old dropped from the choice and
	// removed - no longer in the selection, as Apply leaves it -, new held.
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: used.Source})
	if err != nil {
		t.Fatal(err)
	}
	var f api.InformationSourceFields
	_ = json.Unmarshal(got.Object.FieldsJSON(), &f)
	read := "2026-11-02T09:30:00Z"
	f.LastSynced, f.Items, f.Error = read, 2, "1 élément(s) sur 3 n'ont pas pu être lus : crm: 403"
	f.Selection = map[string]any{api.SelectionItems: []any{
		map[string]any{"key": "erp", "title": "ERP Acme"},
		map[string]any{"key": "crm", "title": "CRM"},
	}}
	f.States = map[string]api.ItemState{"erp": {State: api.ItemRead}, "crm": {State: api.ItemFailed, Why: "403 : droits insuffisants"}, "old": {State: api.ItemRemoved}}
	f.Held = []api.SelectedItem{{Key: "new", Title: "Nouvelle page"}}
	raw, _ := json.Marshal(f)
	got.Object.SetFieldsJSON(raw)
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "m", Object: got.Object}); err != nil {
		t.Fatal(err)
	}
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: used.Source, To: api.SourceStateFailed, By: cwmapi.Actor{ID: "sources", Kind: cwmapi.ActorAgent}}); err != nil {
		t.Fatal(err)
	}
	// What the sync anchored: ERP Acme (a picked item's) and an annex its
	// folder brought.
	for _, doc := range []struct{ id, name string }{{"m.wiki-erp", "ERP Acme"}, {"m.wiki-annexe", "Annexe tarifaire"}} {
		if _, err := svc.Ingest(ctx, api.IngestInput{SourceDocID: doc.id, SourceURI: "wiki://" + doc.id, NewContent: "x", Parent: used.Source, Name: doc.name}); err != nil {
			t.Fatal(err)
		}
	}

	out, err := svc.SourceDetail(ctx, api.SourceDetailInput{ID: used.Source})
	if err != nil {
		t.Fatal(err)
	}
	d := out.Data["detail"].(map[string]any)
	at, _ := time.Parse(time.RFC3339, read)
	if d["state"] != "1 erreur" || d["state_color"] != "red" || d["meta"] != "2 éléments · lu le "+at.Local().Format("02/01 15:04") || d["note"] != f.Error {
		t.Errorf("a source with a failed item: state %v/%v meta %v note %v", d["state"], d["state_color"], d["meta"], d["note"])
	}
	if d["counts"] != "2 synchronisés · 1 à lire · 1 à retirer · 1 en échec" {
		t.Errorf("counts = %q", d["counts"])
	}
	rows := map[string]map[string]any{}
	for _, it := range d["items"].([]any) {
		row := it.(map[string]any)
		rows[row["title"].(string)] = row
	}
	for title, want := range map[string]string{"ERP Acme": "synchronisé", "CRM": "en échec", "old": "à retirer", "Nouvelle page": "à lire", "Annexe tarifaire": "synchronisé"} {
		if rows[title]["state_label"] != want {
			t.Errorf("%s: %v, want %s", title, rows[title], want)
		}
	}
	if len(rows) != 5 {
		t.Errorf("one row per item, the anchored ERP Acme on its picked row: %d rows %v", len(rows), rows)
	}
	if rows["CRM"]["why"] != "403 : droits insuffisants" || rows["Nouvelle page"]["why"] != "retenu, pas encore lu" {
		t.Errorf("why an item failed or waits: %v %v", rows["CRM"], rows["Nouvelle page"])
	}
	if rows["ERP Acme"]["doc"] != "m.wiki-erp" || rows["Annexe tarifaire"]["doc"] != "m.wiki-annexe" || rows["CRM"]["doc"] != nil {
		t.Errorf("an item read opens its source-document, the others none: %v %v %v", rows["ERP Acme"], rows["Annexe tarifaire"], rows["CRM"])
	}
	tree, _ := json.Marshal(out.DSLTree)
	for _, want := range []string{`"id":"source-detail"`, `"dataField":"$.detail.items"`, `"callout":"red"`, `"when":"doc"`, `"object":"${item.doc}"`, `"kind":"table"`} {
		if !strings.Contains(string(tree), want) {
			t.Errorf("the detail's tree lacks %s: %s", want, tree)
		}
	}
	if err := render.ValidatePage(out.DSLTree); err != nil {
		t.Errorf("the detail is no valid page: %v", err)
	}

	// The opened card: the detail first, then the picker.
	panel := func() (string, map[string]any) {
		t.Helper()
		got, err := svc.ConnectorPanel(ctx, api.ConnectorPanelInput{ID: "m", Connector: "wiki-page"})
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(got.DSLTree)
		return string(raw), got.Data
	}
	tree2, data := panel()
	if !strings.Contains(tree2, `"id":"source-detail"`) || strings.Index(tree2, `"source-detail"`) > strings.Index(tree2, `"kind":"picker"`) {
		t.Errorf("the opened card shows the detail before the picker: %s", tree2)
	}
	// : the card is titled by the connector's label, so its
	// detail draws no label badge; the detail on its own still does.
	if strings.Contains(tree2, `"dataField":"$.detail.label"`) {
		t.Errorf("the opened card names its connector twice (title and badge): %s", tree2)
	}
	if !strings.Contains(string(tree), `"dataField":"$.detail.label"`) {
		t.Errorf("the detail on its own lost its label badge: %s", tree)
	}
	if dd, _ := data["detail"].(map[string]any); dd == nil || dd["counts"] != d["counts"] {
		t.Errorf("the panel binds the detail's data: %+v", data["detail"])
	}

	if _, err := svc.SourceDetail(ctx, api.SourceDetailInput{ID: "m"}); err == nil {
		t.Error("the detail of what is no information source is refused")
	}
	if _, err := svc.SourceDetail(ctx, api.SourceDetailInput{}); err == nil {
		t.Error("the detail of no source is refused")
	}
}

// : on the data Hold and Apply write, an item dropped from the
// choice is « à retirer » - titled by its still-anchored document, with no
// « synchronisé » row of its own - and each document is matched to its item
// by the key it was brought for, so two items of one title stay apart.
func TestADroppedItemIsToBeRemovedAndDocumentsMatchByKey(t *testing.T) {
	ctx, _, svc, _ := pickWorld(t)
	detail := func() (map[string]any, map[string]map[string]any) {
		t.Helper()
		of, err := svc.SourcesOf(ctx, api.SourcesOfInput{ID: "m"})
		if err != nil || len(of.Sources) != 1 {
			t.Fatalf("sources: %v %+v", err, of.Sources)
		}
		out, err := svc.SourceDetail(ctx, api.SourceDetailInput{ID: of.Sources[0].ID})
		if err != nil {
			t.Fatal(err)
		}
		d := out.Data["detail"].(map[string]any)
		rows := map[string]map[string]any{}
		for _, it := range d["items"].([]any) {
			row := it.(map[string]any)
			rows[row["key"].(string)] = row
		}
		return d, rows
	}
	apply := func(items ...api.SelectedItem) {
		t.Helper()
		if _, err := svc.Hold(ctx, api.HoldInput{ID: "m", Connector: "apps", Items: items}); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Apply(ctx, api.ApplyInput{ID: "m"}); err != nil {
			t.Fatal(err)
		}
	}
	erp, erp2, stock := api.SelectedItem{Key: "A1", Title: "Acme ERP"}, api.SelectedItem{Key: "A2", Title: "Acme ERP"}, api.SelectedItem{Key: "A3", Title: "Acme Stock"}

	// Two items of one title, each read into its own document.
	apply(erp, erp2, stock)
	d, rows := detail()
	if len(rows) != 3 || d["counts"] != "3 synchronisés" {
		t.Fatalf("one row per item, each read: %v %v", d["counts"], rows)
	}
	if rows["A1"]["doc"] == nil || rows["A2"]["doc"] == nil || rows["A1"]["doc"] == rows["A2"]["doc"] {
		t.Errorf("two items of one title open their own document: %v %v", rows["A1"], rows["A2"])
	}
	stockDoc := rows["A3"]["doc"]

	// Acme Stock dropped while BAD cannot be read: the sync keeps its
	// document until a whole sync - the item is « à retirer », titled by
	// its document, and the document makes no « synchronisé » row.
	apply(erp, erp2, api.SelectedItem{Key: "BAD", Title: "Gone"})
	d, rows = detail()
	a3 := rows["A3"]
	if a3 == nil || a3["state_label"] != "à retirer" || a3["title"] != "Acme Stock" || a3["doc"] != stockDoc || a3["why"] == nil {
		t.Errorf("the dropped item still anchored: %v", a3)
	}
	if rows["BAD"]["state_label"] != "en échec" {
		t.Errorf("the item not read: %v", rows["BAD"])
	}
	if len(rows) != 4 || d["counts"] != "2 synchronisés · 1 à retirer · 1 en échec" {
		t.Errorf("no row of its own for the dropped item's document: %v %v", d["counts"], rows)
	}

	// A whole sync removes them: they leave the detail - no row, no « à
	// retirer » count.
	apply(erp)
	d, rows = detail()
	if len(rows) != 1 || rows["A1"]["state_label"] != "synchronisé" {
		t.Errorf("after a whole sync only the item kept stays: %v", rows)
	}
	if d["counts"] != "1 synchronisé" {
		t.Errorf("counts = %q", d["counts"])
	}
}

// : an item dropped while the sync failed waits « à retirer »
// until a whole sync - a plain one too, not only the next apply - removes it
// from the detail.
func TestADroppedItemLeavesTheDetailOnTheNextWholeSync(t *testing.T) {
	pickVersion = "v1"
	defer func() { pickVersion = "v1" }()
	ctx, _, svc, _ := pickWorld(t)
	apply := func(items ...api.SelectedItem) {
		t.Helper()
		if _, err := svc.Hold(ctx, api.HoldInput{ID: "m", Connector: "apps", Items: items}); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Apply(ctx, api.ApplyInput{ID: "m"}); err != nil {
			t.Fatal(err)
		}
	}
	erp, stock, flaky := api.SelectedItem{Key: "A1", Title: "Acme ERP"}, api.SelectedItem{Key: "A3", Title: "Acme Stock"}, api.SelectedItem{Key: "FLAKY", Title: "Flaky"}
	apply(erp, stock)
	of, err := svc.SourcesOf(ctx, api.SourcesOfInput{ID: "m"})
	if err != nil || len(of.Sources) != 1 {
		t.Fatalf("sources: %v %+v", err, of.Sources)
	}
	id := of.Sources[0].ID
	labels := func() (string, map[string]string) {
		t.Helper()
		out, err := svc.SourceDetail(ctx, api.SourceDetailInput{ID: id})
		if err != nil {
			t.Fatal(err)
		}
		d := out.Data["detail"].(map[string]any)
		rows := map[string]string{}
		for _, it := range d["items"].([]any) {
			row := it.(map[string]any)
			rows[row["key"].(string)] = row["state_label"].(string)
		}
		return d["counts"].(string), rows
	}

	// Acme Stock dropped while the map fails on FLAKY: it waits « à retirer ».
	pickVersion = "flaky"
	apply(erp, flaky)
	if _, rows := labels(); rows["A3"] != "à retirer" {
		t.Fatalf("dropped during a failing sync: %v", rows)
	}

	// The map answers again: a plain whole sync removes it from the detail.
	pickVersion = "v1"
	if r, err := svc.SourceSync(ctx, api.SourceSyncInput{ID: id}); err != nil || !r.OK {
		t.Fatalf("whole sync: %v %+v", err, r)
	}
	counts, rows := labels()
	if _, ok := rows["A3"]; ok || strings.Contains(counts, "à retirer") {
		t.Errorf("after a whole sync the dropped item leaves the detail: %q %v", counts, rows)
	}
}
