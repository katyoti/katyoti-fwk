// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"io"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/loader"
)

// A sibling file is a layer of the entry beside it, not an entry: a display
// engine's widget page travels with its declaration.
func TestSiblingLayer(t *testing.T) {
	fs := fstest.MapFS{
		"embed/display-engines/landscape.yaml": {Data: []byte("label: Landscape\naccepts: [application/vnd.x+json]\n")},
		"embed/display-engines/landscape.html": {Data: []byte("<!doctype html><p>map</p>")},
		"embed/display-engines/plain.yaml":     {Data: []byte("label: Plain\n")},
	}
	p := &EmbedFSResourceProvider{
		ProviderName:        "ktw-test",
		ResourceType:        "display-engine",
		Loader:              loader.NewEmbedFSLoader(fs, "."),
		Root:                "embed/display-engines",
		KeepFolderStructure: true,
		BodyDecoder:         structuredDataLayer,
		SiblingLayers:       map[string]string{"widget": ".html"},
	}
	entries, err := p.Enumerate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2 (the .html is a layer, not an entry)", len(entries))
	}
	rc, ref, err := p.Open(context.Background(), "landscape", "widget")
	if err != nil {
		t.Fatalf("open widget layer: %v", err)
	}
	defer rc.Close()
	b, _ := io.ReadAll(rc)
	if string(b) != "<!doctype html><p>map</p>" || ref.MediaType != "text/html" {
		t.Fatalf("widget layer = %q (%s)", b, ref.MediaType)
	}
	if _, _, err := p.Open(context.Background(), "plain", "widget"); err == nil {
		t.Fatal("an entry without the sibling file has no widget layer")
	}
}

// A data-type carries its field labels' translations: `<name>.dsl.i18n.yaml`
// beside `<name>.dsl.yaml` is its `translations` layer, as JSON, and not a
// data-type of its own.
func TestDataTypeTranslationsLayer(t *testing.T) {
	fs := fstest.MapFS{
		"data-types/items/finding.dsl.yaml":      {Data: []byte("root: Finding\ntypes:\n  Finding:\n    fields:\n      severity: {type: string, label: Severity}\n")},
		"data-types/items/finding.dsl.i18n.yaml": {Data: []byte("fr:\n  types:\n    Finding:\n      fields:\n        severity: {label: Gravité}\n")},
	}
	p := NewDataTypeProvider("ktw-test", loader.NewEmbedFSLoader(fs, "."), "data-types", WithFlatSubfolders())
	entries, err := p.Enumerate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ID != "finding" {
		t.Fatalf("entries: %+v (the .i18n.yaml is a layer, not a data-type)", entries)
	}
	rc, _, err := p.Open(context.Background(), "finding", "translations")
	if err != nil {
		t.Fatalf("open translations layer: %v", err)
	}
	defer rc.Close()
	b, _ := io.ReadAll(rc)
	if string(b) != `{"fr":{"types":{"Finding":{"fields":{"severity":{"label":"Gravité"}}}}}}` {
		t.Fatalf("translations layer = %s", b)
	}
}
