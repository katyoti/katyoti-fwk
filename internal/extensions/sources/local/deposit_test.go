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

// : a document deposited on a carrier joins its « Documents »
// source - one of its own, beside any folder source - and its knowledge base,
// saying who deposited it; a name deposited again never replaces the first:
// it goes under a free name with its own depositor; a name with a folder, a
// reserved name or a refused character, and a file over 5 MB, are refused.
func TestADepositedDocumentJoinsTheDocumentsSourceSayingWho(t *testing.T) {
	ctx, _, svc, root, _ := knowledgeWorld(t)
	b64 := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	cmartin := &api.Keeper{ID: "cmartin", Kind: "person"}
	jdubois := &api.Keeper{ID: "jdubois", Kind: "person"}

	one, err := svc.Deposit(ctx, api.DepositInput{ID: "m", Name: "Contrat Acme 2024.pdf", Content: b64("%PDF contrat"), By: cmartin})
	if err != nil || !one.OK {
		t.Fatalf("deposit: %v %+v", err, one)
	}
	if _, err := svc.Deposit(ctx, api.DepositInput{ID: "m", Name: "Inventaire.xlsx", Content: b64("interfaces"), By: jdubois}); err != nil {
		t.Fatal(err)
	}
	of, err := svc.SourcesOf(ctx, api.SourcesOfInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if len(of.Sources) != 1 || of.Sources[0].Connector != api.DocumentsConnector || of.Sources[0].ID != one.Source {
		t.Fatalf("the carrier's sources: %+v", of.Sources)
	}
	got := map[string]api.DepositedDocument{}
	for _, d := range of.Deposited {
		got[d.Name] = d
	}
	if len(got) != 2 || got["Contrat Acme 2024.pdf"].By != "cmartin" || got["Inventaire.xlsx"].By != "jdubois" {
		t.Fatalf("who deposited each: %+v", of.Deposited)
	}
	for _, d := range got {
		if d.State != api.ItemRead || !strings.Contains(d.Label, "dans la base") || !strings.HasPrefix(d.Label, "déposé par ") {
			t.Errorf("a document not read into the base: %+v", d)
		}
	}
	if of.Documents != 2 {
		t.Errorf("the knowledge base counts %d documents, want 2", of.Documents)
	}
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(got["Contrat Acme 2024.pdf"].Path)))
	if err != nil || string(raw) != "%PDF contrat" {
		t.Errorf("the deposited file: %q %v", raw, err)
	}

	// Deposited again by someone else: never over the first - under a free
	// name, its own item and its own depositor, the message saying where.
	again, err := svc.Deposit(ctx, api.DepositInput{ID: "m", Name: "Contrat Acme 2024.pdf", Content: b64("%PDF v2"), By: jdubois})
	if err != nil || !again.OK || !strings.Contains(again.Message, "déposé sous Contrat Acme 2024 (2).pdf") {
		t.Fatalf("a second deposit of a name: %v %+v", err, again)
	}
	of, _ = svc.SourcesOf(ctx, api.SourcesOfInput{ID: "m"})
	got = map[string]api.DepositedDocument{}
	for _, d := range of.Deposited {
		got[d.Name] = d
	}
	if len(got) != 3 || got["Contrat Acme 2024.pdf"].By != "cmartin" || got["Contrat Acme 2024 (2).pdf"].By != "jdubois" {
		t.Fatalf("each deposit its own document and depositor: %+v", of.Deposited)
	}
	for name, want := range map[string]string{"Contrat Acme 2024.pdf": "%PDF contrat", "Contrat Acme 2024 (2).pdf": "%PDF v2"} {
		if raw, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(got[name].Path))); string(raw) != want {
			t.Errorf("%s holds %q, want %q", name, raw, want)
		}
	}

	// Over 5 MB: refused, saying so, before the hub would.
	big := strings.Repeat("a", 5*1024*1024+1)
	if _, err := svc.Deposit(ctx, api.DepositInput{ID: "m", Name: "big.pdf", Content: b64(big), By: cmartin}); err == nil || !strings.Contains(err.Error(), "trop volumineux") {
		t.Errorf("a file over 5 MB: %v", err)
	}

	for _, bad := range []string{"../x.pdf", "a/b.pdf", `a\b.pdf`, "..", "", "CON", "nul.pdf", "COM1.txt", "a*b.pdf", "x?.pdf", `a"b.pdf`, "a<b.pdf", "a>b.pdf", "a|b.pdf", "c:x.pdf", "fin."} {
		if _, err := svc.Deposit(ctx, api.DepositInput{ID: "m", Name: bad, Content: b64("x"), By: cmartin}); err == nil {
			t.Errorf("the name %q was accepted", bad)
		}
	}
	if _, err := svc.Deposit(ctx, api.DepositInput{ID: "m", Name: "x.pdf", Content: "not base64!", By: cmartin}); err == nil {
		t.Error("a content that is not base64 was accepted")
	}

	// The tab holds the « Documents » section, no inline copy.
	panel, err := os.ReadFile("../contrib/embed/cwm/object-panels/sources.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(panel), `load: {method: "ktw-agents:sources/documents-page", args: {id: "${id}"}}`) || strings.Contains(string(panel), `uri: "ktw-agents:sources/deposit"`) {
		t.Error("the « Sources » tab does not hold the « Documents » section, or keeps an inline copy")
	}
}

// TestTheDocumentsSectionIsARegionAnyPageHolds: sources/documents-page draws
// a carrier's « Documents » for any page - « Déposer un
// document » handing the chosen file to the deposit with who presses it, then
// each document, who deposited it, read in place in the reader drawer
// , never sent to the files panel.
func TestTheDocumentsSectionIsARegionAnyPageHolds(t *testing.T) {
	ctx, _, svc, _, _ := knowledgeWorld(t)
	b64 := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

	empty, err := svc.DocumentsPage(ctx, api.DocumentsPageInput{ID: "m"})
	if err != nil || len(empty.Data["deposited"].([]any)) != 0 {
		t.Fatalf("a carrier with no document: %v %+v", err, empty.Data)
	}
	for who, name := range map[string]string{"cmartin": "Contrat Acme 2024.pdf", "jdubois": "Inventaire.xlsx"} {
		if _, err := svc.Deposit(ctx, api.DepositInput{ID: "m", Name: name, Content: b64(name), By: &api.Keeper{ID: who, Kind: "person"}}); err != nil {
			t.Fatal(err)
		}
	}
	page, err := svc.DocumentsPage(ctx, api.DocumentsPageInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(page.DSLTree)
	for _, want := range []string{`"title":"Documents"`, `"label":"Déposer un document"`, `"uri":"ktw-agents:sources/deposit"`,
		`"args":{"by":"${person}","id":"m"}`, `"file":{"accept":".pdf,`, `"dataField":"$.deposited"`,
		`"drawer":{"file":"${item.path}","object":"m","title":"${item.name}"}`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the region lacks %s: %s", want, raw)
		}
	}
	if strings.Contains(string(raw), `"panel":"files"`) {
		t.Errorf("« Ouvrir le document » still sends the document to the files panel: %s", raw)
	}
	got := map[string]map[string]any{}
	for _, d := range page.Data["deposited"].([]any) {
		m := d.(map[string]any)
		got[m["name"].(string)] = m
	}
	for name, who := range map[string]string{"Contrat Acme 2024.pdf": "cmartin", "Inventaire.xlsx": "jdubois"} {
		d := got[name]
		if d == nil || d["label"] != "déposé par "+who+" · dans la base" || !strings.HasSuffix(d["path"].(string), "documents/"+name) {
			t.Errorf("%s: %+v", name, d)
		}
	}
	if len(got) != 2 {
		t.Errorf("the region lists %d documents, want 2: %+v", len(got), page.Data)
	}
	if _, err := svc.DocumentsPage(ctx, api.DocumentsPageInput{ID: "nope"}); err == nil {
		t.Error("a carrier that does not exist answered a page")
	}
}
