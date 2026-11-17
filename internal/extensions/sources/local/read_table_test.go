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
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// : an opened source's « Contenu lu » is a table, the same for
// every connector - the title (its entities decoded, its open mark before
// it), the item's kind as its connector names it, its state and why, when it
// was read - sortable, searched by title, a row opening the item read; a
// connector adds its own columns by declaration (read), filled from the
// item's data, never by GUI code.
func TestContenuLuIsATableWithTheConnectorsColumns(t *testing.T) {
	ctx, cwm, svc, _ := pickWorldWith(t, map[string]string{"links.yaml": `name: links
label: Interfaces
description: The interfaces of the map.
sync: ktw-apps:links/source-sync
kinds:
  interface: {label: interface}
read:
  - {attr: source, label: Source}
  - {attr: target, label: Cible}
`})
	src := createSource(t, ctx, cwm, "links", map[string]any{"connector": "links"})
	res, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: "m"})
	if err != nil || res.ProjectRoot == "" {
		t.Fatalf("the project root: %v %+v", err, res)
	}
	data := filepath.Join(res.ProjectRoot, "input", "flow.data.json")
	if err := os.MkdirAll(filepath.Dir(data), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(data, []byte(`{"source":"ERP","target":"CRM &amp; ventes","code":"IF-1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Ingest(ctx, api.IngestInput{SourceDocID: "m.links-flow", SourceURI: "leanix://flow", NewContent: "x", Parent: src,
		Name: "Architecture AS-IS (étude de l&#39;existant)", ItemKind: "interface", DataFile: "input/flow.data.json"}); err != nil {
		t.Fatal(err)
	}

	out, err := svc.SourceDetail(ctx, api.SourceDetailInput{ID: src})
	if err != nil {
		t.Fatal(err)
	}
	items := out.Data["detail"].(map[string]any)["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("one item read: %v", items)
	}
	row := items[0].(map[string]any)
	if row["title"] != "Architecture AS-IS (étude de l'existant)" {
		t.Errorf("the title is decoded: %q", row["title"])
	}
	if row["doc"] != "m.links-flow" || row["open"] != "external-link" || row["kind"] != "interface" || row["state_label"] != "synchronisé" || row["read_at"] == nil {
		t.Errorf("the row opens its document, says its kind, state and read date: %v", row)
	}
	if row["x_source"] != "ERP" || row["x_target"] != "CRM & ventes" {
		t.Errorf("the connector's columns come from the item's data: %v", row)
	}

	raw, _ := json.Marshal(out.DSLTree)
	tree := string(raw)
	for _, want := range []string{`"kind":"table"`, `"id":"source-detail-items"`, `"dataField":"$.detail.items"`,
		`"navigate":{"object":"${item.doc}"}`, `"filters":["title"]`, `"sortable":true`, `"prepend":["open"]`,
		`"id":"x_source"`, `"label":"Source"`, `"id":"x_target"`, `"label":"Cible"`} {
		if !strings.Contains(tree, want) {
			t.Errorf("the « Contenu lu » table lacks %s: %s", want, tree)
		}
	}
	if strings.Contains(tree, `"kind":"list"`) || strings.Contains(tree, `"actions":[{"label":"Ouvrir"`) {
		t.Errorf("« Contenu lu » is still a list with « Ouvrir » at the far right: %s", tree)
	}
	if err := render.ValidatePage(out.DSLTree); err != nil {
		t.Errorf("the detail is no valid page: %v", err)
	}

	// A connector declaring no column draws the shared ones only.
	plain := createSource(t, ctx, cwm, "drive", map[string]any{"connector": "drive"})
	if _, err := svc.Ingest(ctx, api.IngestInput{SourceDocID: "m.drive-doc", SourceURI: "drive://doc", NewContent: "x", Parent: plain, Name: "Doc"}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.SourceDetail(ctx, api.SourceDetailInput{ID: plain})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(got.DSLTree)
	if !strings.Contains(string(raw), `"kind":"table"`) || strings.Contains(string(raw), `"id":"x_`) {
		t.Errorf("a connector with no read columns: the shared table alone: %s", raw)
	}
}
