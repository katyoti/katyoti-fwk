// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
	sourcescontrib "github.com/katyoti/katyoti-fwk/internal/extensions/sources/contrib"
	sourceslocal "github.com/katyoti/katyoti-fwk/internal/extensions/sources/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// SOURCE ANCHORS, on a real CWM with the shipped sources pack: a
// source-document records the anchored section's hash; an object derived from
// it carries a `derived-from` edge; Reanchor reads the source again, records
// the outcome on the document through CWM and reports the derived objects when
// the section drifted or disappeared.

var others = fstest.MapFS{
	"object-types/module.yaml":   {Data: []byte("name: module\ntree: node\n")},
	"object-types/decision.yaml": {Data: []byte("name: decision\ntree: node\n")},
}

func fixture(t *testing.T, sourceText string) (context.Context, cwmapi.CWMService, api.SourcesService) {
	t.Helper()
	ctx, cwm, root := project(t)
	if err := os.WriteFile(filepath.Join(root, "spec.md"), []byte(sourceText), 0o644); err != nil {
		t.Fatal(err)
	}
	uri := fileURI(filepath.Join(root, "spec.md"))
	fields, _ := json.Marshal(map[string]any{
		"source_uri": uri, "section_path": []string{"Auth", "Rate Limits"},
		// sha256 of the normalized "Limit 100 rps." — the recorded anchor.
		"content_hash": "sha256:ab5dd67815ef89372f3aaca47fd205c77ed4c264ba3b2a132260de3b2e6f05e0",
	})
	create := func(o cwmapi.Object) {
		t.Helper()
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}
	doc := cwmapi.NewContainerObject("m", "spec", api.SourceDocumentType, "m", "spec", "")
	doc.SetFieldsJSON(fields)
	create(doc)
	dec := cwmapi.NewContainerObject("m", "rate-limit", "decision", "m", "rate limit", "")
	dec.Edges = []cwmapi.ObjectEdge{{Kind: api.DerivedFromEdge, To: "m.spec"}}
	create(dec)
	create(cwmapi.NewContainerObject("m", "other", "decision", "m", "other", ""))

	svc, err := application.UseService[api.SourcesService](ctx)
	if err != nil {
		t.Fatalf("resolve SourcesService: %v", err)
	}
	return ctx, cwm, svc
}

// project boots a real CWM on a temporary project holding module m; the
// project root is the folder the file fetcher is confined to.
func project(t *testing.T) (context.Context, cwmapi.CWMService, string) {
	t.Helper()
	root := t.TempDir()
	ctx, cwm := cwmapp.NewAt(t, root, sourcescontrib.Pack(""), cwmapp.Pack(others), sourceslocal.Extension())
	cwmapp.Module(t, ctx, cwm, "m")
	return ctx, cwm, root
}

// fileURI renders an absolute path as a file:// URI.
func fileURI(path string) string {
	if runtime.GOOS == "windows" {
		return "file:///" + filepath.ToSlash(path)
	}
	return "file://" + filepath.ToSlash(path)
}

func TestReanchorMatchReportsNothing(t *testing.T) {
	ctx, _, svc := fixture(t, "# Auth\n\n## Rate Limits\n\nLimit 100 rps.\n")
	out, err := svc.Reanchor(ctx, api.ReanchorInput{SourceDocID: "m.spec"})
	if err != nil || out.Status != "match" || len(out.StaleItemIDs)+len(out.MissingItemIDs) != 0 {
		t.Fatalf("Reanchor = %+v (%v), want a match reporting nothing", out, err)
	}
}

// TestReanchorDriftReportsTheDerivedObjects: a drifted section reports the
// object with a derived-from edge (and only it), and the source-document
// records the stale anchor, written through CWM.
func TestReanchorDriftReportsTheDerivedObjects(t *testing.T) {
	ctx, cwm, svc := fixture(t, "# Auth\n\n## Rate Limits\n\nLimit 50 rps.\n")
	out, err := svc.Reanchor(ctx, api.ReanchorInput{SourceDocID: "m.spec"})
	if err != nil || out.Status != "drift" || strings.Join(out.StaleItemIDs, ",") != "m.rate-limit" {
		t.Fatalf("Reanchor = %+v (%v), want drift reporting m.rate-limit", out, err)
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: "m.spec"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got.Object.FieldsJSON()), `"anchor_status":"stale"`) {
		t.Fatalf("stored fields = %s, want anchor_status stale", got.Object.FieldsJSON())
	}
}

func TestReanchorMissingSection(t *testing.T) {
	ctx, _, svc := fixture(t, "# Auth\n\n## Elsewhere\n\nx\n")
	out, err := svc.Reanchor(ctx, api.ReanchorInput{SourceDocID: "m.spec"})
	if err != nil || out.Status != "missing" || strings.Join(out.MissingItemIDs, ",") != "m.rate-limit" {
		t.Fatalf("Reanchor = %+v (%v), want missing reporting m.rate-limit", out, err)
	}
}

func TestReanchorRefusesAnotherType(t *testing.T) {
	ctx, _, svc := fixture(t, "# Auth\n")
	if _, err := svc.Reanchor(ctx, api.ReanchorInput{SourceDocID: "m.other"}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("reanchor a decision: %v, want ErrInvalidInput", err)
	}
	if _, err := svc.Reanchor(ctx, api.ReanchorInput{SourceDocID: "m.ghost"}); !errors.Is(err, cwmapi.ErrNotFound) {
		t.Fatalf("reanchor a missing object: %v, want ErrNotFound", err)
	}
}
