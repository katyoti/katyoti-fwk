// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// : sources' own connectors - the local folder and Documents -
// declare their source: a browse-only picker (no free search) with its unit
// and layout, items keyed with a kind; so both are in Pickers (what the
// source-set field lists) and in the proposal, and their browse answers keyed
// items with a kind - Documents' what was deposited, nothing before.
func TestTheLocalFolderAndDocumentsArePickers(t *testing.T) {
	ctx, _, svc, root, _ := knowledgeWorld(t)

	pk, err := svc.Pickers(ctx, api.PickersInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	views := map[string]api.ConnectorPickerView{}
	for _, c := range pk.Connectors {
		views[c.Name] = c
	}
	for name, want := range map[string]struct{ label, unit, layout string }{
		api.FolderConnector:    {"Dossier local", "fichier", "tree"},
		api.DocumentsConnector: {"Documents", "document", "list"},
	} {
		v, ok := views[name]
		if !ok {
			t.Fatalf("%s is not a picker: %+v", name, pk.Connectors)
		}
		if v.Label != want.label || v.Unit != want.unit || v.Layout != want.layout || v.Search != "" || v.Browse != api.SourceBrowseURI || v.Icon == "" {
			t.Errorf("the %s picker: %+v; want %+v, browse-only", name, v, want)
		}
	}
	wiz, err := svc.ProposalWizard(ctx, api.ProposalWizardInput{Object: "m"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(wiz.Page)
	for _, name := range []string{api.FolderConnector, api.DocumentsConnector} {
		if !strings.Contains(string(raw), `"id":"`+name+`"`) {
			t.Errorf("the proposal holds no step for %s: %s", name, raw)
		}
	}

	// Documents before anything is deposited: nothing to browse, no error.
	none, err := svc.SourceBrowse(ctx, api.SourceBrowseInput{Connector: api.DocumentsConnector, Object: "m"})
	if err != nil || len(none.Items) != 0 {
		t.Fatalf("Documents before a deposit: %v %+v", err, none)
	}
	b64 := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	source := ""
	for _, name := range []string{"Contrat.pdf", "Inventaire.xlsx"} {
		d, err := svc.Deposit(ctx, api.DepositInput{ID: "m", Name: name, Content: b64(name), By: &api.Keeper{ID: "cmartin", Kind: "person"}})
		if err != nil {
			t.Fatal(err)
		}
		source = d.Source
	}
	docs, err := svc.SourceBrowse(ctx, api.SourceBrowseInput{Connector: api.DocumentsConnector, Object: "m"})
	if err != nil || keysOf(docs.Items) != "Contrat.pdf Inventaire.xlsx" {
		t.Fatalf("Documents browsed: %v %+v", err, docs)
	}
	for _, it := range docs.Items {
		if it.Kind != "file" || it.Title != it.Key || it.Modified == "" {
			t.Errorf("a deposited document's item: %+v; want keyed, kind file, modified", it)
		}
	}
	// Its own source browsed: each document picked by its deposit is ticked.
	mine, err := svc.SourceBrowse(ctx, api.SourceBrowseInput{ID: source})
	if err != nil || len(mine.Items) != 2 || !mine.Items[0].Picked || !mine.Items[1].Picked {
		t.Errorf("the Documents source browsed: %v %+v; want both deposits ticked", err, mine)
	}

	// The local folder: a file below a folder carries its parent and path.
	for name, body := range map[string]string{"specs/api/v1.md": "# V1\n", "readme.md": "# R\n"} {
		p := filepath.Join(root, "ext", filepath.FromSlash(name))
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	loc := map[string]any{"path": filepath.Join(root, "ext")}
	top, err := svc.SourceBrowse(ctx, api.SourceBrowseInput{Connector: api.FolderConnector, Location: loc})
	if err != nil || keysOf(top.Items) != "specs readme.md" || top.Items[0].Kind != "folder" || top.Items[1].Kind != "file" || top.Items[0].Parent != "" {
		t.Fatalf("the local folder's top level: %v %+v", err, top)
	}
	deep, err := svc.SourceBrowse(ctx, api.SourceBrowseInput{Connector: api.FolderConnector, Location: loc, Parent: "specs/api"})
	if err != nil || len(deep.Items) != 1 {
		t.Fatalf("specs/api: %v %+v", err, deep)
	}
	if it := deep.Items[0]; it.Key != "specs/api/v1.md" || it.Kind != "file" || it.Parent != "specs/api" || strings.Join(it.Path, "/") != "specs/api" || it.Modified == "" {
		t.Errorf("a file below a folder: %+v; want its key, kind, parent, path and modified", it)
	}
}
