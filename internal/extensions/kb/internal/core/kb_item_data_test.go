// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	kbapi "github.com/katyoti/katyoti-fwk/internal/extensions/kb/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/kb/internal/bundling"
	"github.com/katyoti/katyoti-fwk/internal/extensions/kb/internal/embedding"
	"github.com/katyoti/katyoti-fwk/internal/test/long"
)

// : the knowledge base reads an item's text, and its data rendered
// as text where the item has no text. An item with data only
// (<stem>.data.json) is indexed as its data rendered as text, searchable; an
// item with data and text is indexed once, through its text - its data file
// read on its own never, and a bundle its data had alone leaves the base.
func TestTheKnowledgeBaseReadsAnItemsTextOrItsDataAsText(t *testing.T) {
	long.Test(t)
	t.Setenv("KB_TEST_KEY", "secret")
	srv, _ := embedServer(t)
	s := serviceWith(t, "main", map[string]embedding.Profile{
		"main": {Type: embedding.TypeOpenAI, Endpoint: srv.URL + "/main", KeyEnv: "KB_TEST_KEY", Model: "m", Dimensions: 3},
	})
	root := t.TempDir()
	kb := "name: testkb\nsources: [leanix]\nextraction:\n  profile: fast\nindexing:\n  backend: chromem-go\n  embedding:\n    profile: main\n"
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("KnowledgeBase.yaml", kb)
	// ERP: data only. CRM: data and its text.
	write("leanix/erp.data.json", `{"name":"ERP Acme","code":"ACME","owner":{"team":"Finance"},"interfaces":["SAP","Payroll"]}`)
	write("leanix/crm.data.json", `{"name":"CRM","code":"CRM01"}`)
	write("leanix/crm.md", "# CRM\n\nThe customer relationship application of the sales teams.\n")
	ctx := context.Background()

	ext, err := s.Extract(ctx, kbapi.ExtractInput{Path: root})
	if err != nil || len(ext.Errors) > 0 {
		t.Fatalf("extract: %+v %v", ext, err)
	}
	if ext.New != 2 || ext.Skipped != 1 {
		t.Fatalf("extract read %d new and skipped %d; want ERP's data and CRM's text read, CRM's data skipped", ext.New, ext.Skipped)
	}
	erp, err := bundling.LoadMetadata(root, bundling.Slug("leanix", "erp.data.json"))
	if err != nil {
		t.Fatal(err)
	}
	if erp.Extraction.Extractor != "item-data-text" {
		t.Errorf("ERP's data was read by %q; want its data rendered as text", erp.Extraction.Extractor)
	}
	text, err := os.ReadFile(filepath.Join(bundling.BundleDir(root, erp.Slug), "content.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"- **code**: ACME", "- **owner**:\n  - **team**: Finance", "- **interfaces**:\n  - SAP"} {
		if !strings.Contains(string(text), want) {
			t.Errorf("ERP's text lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(string(text), `"code"`) {
		t.Errorf("ERP's data was read as raw JSON:\n%s", text)
	}
	if _, err := bundling.LoadMetadata(root, bundling.Slug("leanix", "crm.data.json")); err == nil {
		t.Errorf("CRM's data has a bundle of its own: the item would be read twice")
	}

	idx, err := s.Index(ctx, kbapi.IndexInput{Path: root})
	if err != nil || idx.ChunksIndexed == 0 || len(idx.Errors) > 0 {
		t.Fatalf("index: %+v %v", idx, err)
	}
	q, err := s.Query(ctx, kbapi.QueryInput{Path: root, Query: "ACME"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range q.Results {
		if strings.Contains(r.Content, "ACME") {
			found = true
		}
	}
	if !found {
		t.Errorf("a query does not find the data-only item: %+v", q.Results)
	}

	// A text appearing beside a data read alone: the data's bundle leaves.
	write("leanix/erp.md", "# ERP Acme\n\nThe finance application.\n")
	if _, err := s.Extract(ctx, kbapi.ExtractInput{Path: root}); err != nil {
		t.Fatal(err)
	}
	if m, err := bundling.LoadMetadata(root, erp.Slug); err != nil || m.Status != "stale" {
		t.Errorf("ERP's data bundle stays once its text came: %+v %v", m, err)
	}
}
