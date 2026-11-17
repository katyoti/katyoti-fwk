// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
	sourcescontrib "github.com/katyoti/katyoti-fwk/internal/extensions/sources/contrib"
	sourceslocal "github.com/katyoti/katyoti-fwk/internal/extensions/sources/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// pageApps and pageProbes are two connectors: applications with a code, and
// probes found from those applications by their code.
const pageApps = `name: apps
label: Applications Atlas
description: The applications of the map.
system: Atlas
color: teal
sync: ktw-atlas:atlas/source-sync
picker: {unit: application, gender: feminine}
kinds:
  app: {label: Application, slots: [{attr: code, label: Code}]}
`

const pageProbes = `name: probes
label: Sondes Probe
description: The probes of the applications.
system: Probe
color: orange
sync: ktw-probe:probe/source-sync
from: {connector: apps, attr: code}
`

// (mock-up v6 view 17): a knowledge-base item opens as
// « Base de connaissance · élément » - its title, kind and state and what it
// is; « Vient de », the source it came from as a card (its connector, name,
// connector label, id and when it was read) opening it; for an item of a
// connector reading from another source, how the link found it; a record's
// fields as rows, a document's extract with the stale note; « Ouvrir dans
// <système> » to the item's address and « Ouvrir la source ».
func TestAKnowledgeBaseItemOpensAsTheMockupsDrawer(t *testing.T) {
	root := t.TempDir()
	bad := false
	foreign := loader.NewEmbedFSLoader(fstest.MapFS{
		"connectors/apps.yaml":   {Data: []byte(pageApps)},
		"connectors/probes.yaml": {Data: []byte(pageProbes)},
	}, ".")
	ctx, cwm := cwmapp.NewAt(t, root,
		sourcescontrib.Pack("ktw-agents"),
		sourcescontrib.DeclareKinds("ktw-agents"),
		sourcescontrib.OwnConnectors("ktw-agents"),
		contributeConnectors("ktw-atlas", foreign, "connectors"),
		cwmapp.Pack(fstest.MapFS{
			"object-types/module.yaml":    {Data: []byte("name: module\ntree: node\n")},
			"capabilities/lifecycle.yaml": {Data: []byte("name: lifecycle\n")},
		}),
		sourceslocal.Extension(),
		application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) {
			return mapsActions{bad: &bad}, nil
		}),
	)
	cwmapp.Module(t, ctx, cwm, "m")
	svc, err := application.UseService[api.SourcesService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	read := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	atlas := createSource(t, ctx, cwm, "atlas", map[string]any{"connector": "apps", "location": map[string]any{"map": "x"}, "last_synced": read})
	probe := createSource(t, ctx, cwm, "probe", map[string]any{"connector": "probes", "location": map[string]any{"env": "x"}})

	if err := os.MkdirAll(filepath.Join(root, "m", "input"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "m", "input", "erp.json"), []byte(`{"code":"ERP","name":"Billing","tags":["core","finance"],"empty":""}`), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, d := range []api.IngestInput{
		{SourceDocID: "m.erp", SourceURI: "https://atlas.example/app/erp", NewContent: "x", Parent: atlas, Name: "Billing", ItemKey: "erp", ItemKind: "app", DataFile: "m/input/erp.json"},
		{SourceDocID: "m.erp-notes", SourceURI: "atlas://erp/notes", NewContent: "# Notes\n\nThe **billing** app ![shot](data:image/png;base64,AAAA) sends [invoices](https://x.example) nightly.\n", Parent: atlas, Name: "Billing notes", ItemKey: "notes", ItemKind: "page"},
		{SourceDocID: "m.erp-host", SourceURI: "probe://erp-host", NewContent: "x", Parent: probe, Name: "erp-host", ItemKey: "erp-host"},
	} {
		if _, err := svc.Ingest(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.Reanchor(ctx, api.ReanchorInput{SourceDocID: "m.erp-notes", NewContent: "changed"}); err != nil {
		t.Fatal(err)
	}

	open := func(id string) (map[string]any, string) {
		t.Helper()
		out, err := svc.DocumentPage(ctx, api.DocumentDetailInput{ID: id})
		if err != nil {
			t.Fatal(err)
		}
		if out.Kind != "page" {
			t.Errorf("%s opens as a page, got %q", id, out.Kind)
		}
		if err := render.ValidatePage(out.DSLTree); err != nil {
			t.Errorf("%s's page: %v", id, err)
		}
		tree, _ := json.Marshal(out.DSLTree)
		return out.Data.(map[string]any), string(tree)
	}

	// A record: its head, its source card, its fields as rows, its address.
	rec, tree := open("m.erp")
	if rec["title"] != "Billing" || rec["kind_label"] != "Enregistrement" || rec["state_label"] != "Lu" || rec["sub"] != "Application" {
		t.Errorf("a record's head: %v", rec)
	}
	src := rec["source"].([]any)
	if len(src) != 1 {
		t.Fatalf("a record comes from its source: %v", rec["source"])
	}
	card := src[0].(map[string]any)
	if card["id"] != atlas || card["name"] != "atlas" || card["mark"] != "Atlas" || card["color"] != "teal" ||
		card["line"] != "Applications Atlas · "+atlas+" · lu il y a 2 h" {
		t.Errorf("the source card: its connector, name, label, id and when it was read: %v", card)
	}
	fields := map[string]string{}
	for _, r := range rec["fields"].([]any) {
		row := r.(map[string]any)
		fields[row["key"].(string)] = row["value"].(string)
	}
	if len(fields) != 3 || fields["code"] != "ERP" || fields["name"] != "Billing" || fields["tags"] != "core, finance" {
		t.Errorf("a record's fields as rows, empty ones left out: %v", fields)
	}
	if rec["open_url"] != "https://atlas.example/app/erp" || rec["open_label"] != "Ouvrir dans Atlas" || rec["found_by"] != "" || rec["extract"] != "" {
		t.Errorf("a record opens in its system, found by no link: %v", rec)
	}
	for _, want := range []string{"Base de connaissance · élément", `"id":"source-document-source"`, `"navigate":{"object":"${item.id}"}`,
		`"id":"source-document-fields"`, `"href":"https://atlas.example/app/erp"`, `"id":"source-document-open-source"`,
		`"navigate":{"object":"` + atlas + `"}`} {
		if !strings.Contains(tree, want) {
			t.Errorf("the record's drawer misses %s", want)
		}
	}
	if strings.Contains(tree, "Item") || strings.Contains(tree, "source-document-extract") {
		t.Error("a record's drawer speaks French and draws its fields, no extract")
	}

	// A stale document: its extract in plain words, the stale note, no
	// address to open outside the web.
	doc, tree := open("m.erp-notes")
	if doc["kind_label"] != "Document" || doc["state_label"] != "À relire" || doc["sub"] != "Page" {
		t.Errorf("a stale document's head: %v", doc)
	}
	if ex, _ := doc["extract"].(string); !strings.Contains(ex, "The billing app sends invoices nightly.") && !strings.Contains(ex, "changed") {
		t.Errorf("a document's extract is its text in plain words: %q", ex)
	}
	if ex, _ := doc["extract"].(string); strings.Contains(ex, "data:image") || strings.Contains(ex, "**") {
		t.Errorf("an extract drops images and marks: %q", ex)
	}
	if n, _ := doc["stale_note"].(string); !strings.Contains(n, "Atlas") || doc["open_url"] != "" {
		t.Errorf("a stale document says its system changed it, no web address: %v", doc)
	}
	for _, want := range []string{`"id":"source-document-extract"`, `"id":"source-document-stale"`} {
		if !strings.Contains(tree, want) {
			t.Errorf("the document's drawer misses %s", want)
		}
	}

	// An item found through another source: how the link found it.
	found, tree := open("m.erp-host")
	if found["found_by"] != "Trouvé par le lien : l'application de la source Atlas donne son code, cherché dans Probe." {
		t.Errorf("an item found by a link says how: %q", found["found_by"])
	}
	if !strings.Contains(tree, `"id":"source-document-found-by"`) {
		t.Error("the found-by note is drawn")
	}

	// The knowledge base's source column says each fact once: a source named
	// as its connector shows its badge alone.
	if _, err := cwm.ObjectAddCapability(ctx, cwmapi.ObjectAddCapabilityInput{ID: "m", Capability: api.KnowledgeCapability}); err != nil {
		t.Fatal(err)
	}
	same := cwmapi.NewContainerObject("m", "same", api.InformationSourceType, "m", "Applications Atlas", "")
	raw, _ := json.Marshal(map[string]any{"connector": "apps", "location": map[string]any{"map": "y"}})
	same.SetFieldsJSON(raw)
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: same}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Ingest(ctx, api.IngestInput{SourceDocID: "m.same-crm", SourceURI: "atlas://crm", NewContent: "x", Parent: "m.same", Name: "CRM", ItemKey: "crm"}); err != nil {
		t.Fatal(err)
	}
	kb, err := svc.KnowledgeBase(ctx, api.KnowledgeBaseInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]any{}
	for _, it := range kb.Data.(map[string]any)["items"].([]any) {
		row := it.(map[string]any)
		names[row["id"].(string)] = row["source_name"]
		if row["source_name"] != "" && row["source_name"] == row["source_label"] {
			t.Errorf("%s repeats its source: %v", row["id"], row)
		}
	}
	if names["m.same-crm"] != "" || names["m.erp"] != "atlas" {
		t.Errorf("the source column: a name told apart from its connector shows, one equal to it does not: %v", names)
	}
}
