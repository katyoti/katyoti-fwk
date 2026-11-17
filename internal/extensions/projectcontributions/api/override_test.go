// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// The override object's id is the provider, kind and entry id in kebab case;
// it is a document whose `contributes` group names the entry and its layers.
func TestOverrideObjectCarriesTheEntryAndItsLayers(t *testing.T) {
	if got := OverrideObjectID("ktw-example", "note", "Hello_World.v2"); got != "override-ktw-example-note-hello-world-v2" {
		t.Fatalf("OverrideObjectID = %q", got)
	}
	o := ConvertedOverride{Overrides: "ktw-example:/note/hello", Kind: "note", ID: "hello",
		Layers: map[string]string{"source": "files/overrides/note/hello/source.yaml"}}
	obj := OverrideObject("ktw-example", o)
	if obj.ID != "override-ktw-example-note-hello" || obj.Type != OverrideObjectType || obj.Name != o.Overrides {
		t.Fatalf("object = %+v", obj)
	}
	c := obj.Capabilities[CapContributes]
	layers, _ := c["layers"].(map[string]any)
	if c["kind"] != "note" || c["id"] != "hello" || c["overrides"] != o.Overrides || layers["source"] != o.Layers["source"] {
		t.Fatalf("contributes = %v", c)
	}
	if got := OverrideLayerFile("overrides", "note", "hello", "data", "application/json; charset=utf-8"); got != "overrides/note/hello/data.json" {
		t.Fatalf("OverrideLayerFile = %q", got)
	}
}

// A pack's layer-to-file map: the source and a sibling (the translations) are
// files read whole; the data a source carries is derived from part of it.
func TestPackLayerFilesSaysWhichFileEachLayerReads(t *testing.T) {
	dir := t.TempDir()
	for rel, text := range map[string]string{
		"embed/note/hello.yaml":      "title: Hello\n",
		"embed/note/hello.i18n.yaml": "fr:\n  title: Bonjour\n",
	} {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	decl := contribapi.Contribution{Provider: "system", Type: contribapi.ResourceTypeContributionType, ID: "note"}
	decl.SetData(json.RawMessage(`{"name":"note","layers":{"data":{"mediaType":"application/json"}}}`))
	packs, err := PackLayerFiles(context.Background(), NewTreeLoader(dir), "ktw-example", contribapi.KindsFrom([]contribapi.Contribution{decl}))
	if err != nil {
		t.Fatal(err)
	}
	var layers map[string]LayerFiles
	for _, k := range packs {
		if l, ok := k.Layers["ktw-example:/note/hello"]; ok {
			layers = l
		}
	}
	want := map[string]LayerFiles{
		contribapi.LayerSource:       {Files: []string{"embed/note/hello.yaml"}, Whole: true},
		contribapi.LayerData:         {Files: []string{"embed/note/hello.yaml"}, Whole: false},
		contribapi.LayerTranslations: {Files: []string{"embed/note/hello.i18n.yaml"}, Whole: true},
	}
	for name, w := range want {
		g, ok := layers[name]
		if !ok || len(g.Files) != 1 || g.Files[0] != w.Files[0] || g.Whole != w.Whole {
			t.Errorf("layer %s = %+v, want %+v (all: %+v)", name, g, w, layers)
		}
	}
}
