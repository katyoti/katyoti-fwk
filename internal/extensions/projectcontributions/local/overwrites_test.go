// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// A project's overwrites tree becomes override objects: one per entry it
// changes, naming the entry it overrides, each layer a file of the object's
// module; each file override's widening to whole layers is reported; a file
// no entry's layer reads is listed, and the tree is then not retirable.
func TestAnOverwritesTreeBecomesOverrideObjects(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, text string) {
		t.Helper()
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("embed/note/hello.yaml", "title: Hello\ntext: overridden\n")
	write("embed/note/hello.i18n.yaml", "fr:\n  title: Bonjour\n")
	decl := contribapi.Contribution{Provider: "system", Type: contribapi.ResourceTypeContributionType, ID: "note"}
	decl.SetData(json.RawMessage(`{"name":"note","layers":{"data":{"mediaType":"application/json"}}}`))
	kinds := contribapi.KindsFrom([]contribapi.Contribution{decl})

	conv, err := ConvertOverwrites(context.Background(), dir, "ktw-example", kinds, "overrides/ktw-example")
	if err != nil {
		t.Fatal(err)
	}
	if len(conv.Overrides) != 1 {
		t.Fatalf("overrides = %+v; want hello's", conv.Overrides)
	}
	o := conv.Overrides[0]
	if o.Overrides != "ktw-example:/note/hello" || o.Kind != "note" || o.ID != "hello" {
		t.Fatalf("override = %s (%s/%s)", o.Overrides, o.Kind, o.ID)
	}
	if _, ok := o.Layers[contribapi.LayerData]; !ok {
		t.Fatalf("layers = %v; want the data the file carries", o.Layers)
	}
	if _, ok := o.Layers[contribapi.LayerTranslations]; !ok {
		t.Fatalf("layers = %v; want the translations its sibling carries", o.Layers)
	}
	for name, src := range o.Layers {
		file, ok := strings.CutPrefix(src, "files/")
		if !ok || len(o.Files[file]) == 0 || !strings.HasPrefix(file, "overrides/ktw-example/note/hello/") {
			t.Fatalf("layer %s = %s; want a file of the module under the given folder, written", name, src)
		}
	}
	if strings.Join(o.From, ",") != "embed/note/hello.i18n.yaml,embed/note/hello.yaml" {
		t.Fatalf("from = %v", o.From)
	}
	if len(conv.Widenings) != 1 || !strings.Contains(conv.Widenings[0], "whole layers") {
		t.Fatalf("widenings = %v", conv.Widenings)
	}
	if !conv.Retirable() {
		t.Fatalf("unmapped = %v; want none", conv.Unmapped)
	}

	write("embed/notes-of-nobody/stray.yaml", "x: 1\n")
	conv, err = ConvertOverwrites(context.Background(), dir, "ktw-example", kinds, "overrides/ktw-example")
	if err != nil {
		t.Fatal(err)
	}
	if conv.Retirable() || len(conv.Unmapped) != 1 || !strings.HasPrefix(conv.Unmapped[0], "embed/notes-of-nobody/stray.yaml") {
		t.Fatalf("unmapped = %v; want the stray file, and the tree kept", conv.Unmapped)
	}
}

// treeIn writes files into a fresh tree.
func treeIn(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, text := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func noteKinds() contribapi.Kinds {
	decl := contribapi.Contribution{Provider: "system", Type: contribapi.ResourceTypeContributionType, ID: "note"}
	decl.SetData(json.RawMessage(`{"name":"note","layers":{"data":{"mediaType":"application/json"}}}`))
	return contribapi.KindsFrom([]contribapi.Contribution{decl})
}

// A folder entry's file no layer reads (a skill's reference) is not counted
// mapped: it is listed and the tree is kept.
func TestAFolderEntrysUnreadFileKeepsTheTree(t *testing.T) {
	dir := treeIn(t, map[string]string{
		"embed/doc/greet/GUIDE.md":        "---\nname: greet\ndescription: Greets\n---\nHello.\n",
		"embed/doc/greet/references/x.md": "A reference no layer reads.\n",
	})
	decl := contribapi.Contribution{Provider: "system", Type: contribapi.ResourceTypeContributionType, ID: "doc"}
	decl.SetData(json.RawMessage(`{"name":"doc","layers":{"data":{"mediaType":"application/json","default":true},"source":{"mediaType":"text/markdown"}},
		"source":{"folder":"doc","match":["*.md"],"id":"dir","claims":"GUIDE.md","format":"frontmatter",
		"layers":{"data":{"from":"frontmatter"},"source":{"from":"file"}}}}`))
	kinds := contribapi.KindsFrom([]contribapi.Contribution{decl})

	conv, err := ConvertOverwrites(context.Background(), dir, "ktw-example", kinds, "overrides/ktw-example")
	if err != nil {
		t.Fatal(err)
	}
	if len(conv.Overrides) != 1 || strings.Join(conv.Overrides[0].From, ",") != "embed/doc/greet/GUIDE.md" {
		t.Fatalf("overrides = %+v; want greet, read from its GUIDE.md only", conv.Overrides)
	}
	if conv.Retirable() || len(conv.Unmapped) != 1 || !strings.HasPrefix(conv.Unmapped[0], "embed/doc/greet/references/x.md: ") {
		t.Fatalf("unmapped = %v; want the unread reference, and the tree kept", conv.Unmapped)
	}
}

// A layer whose file opens empty is not written: its file stays unmapped.
func TestAnEmptyLayerLeavesItsFileUnmapped(t *testing.T) {
	dir := treeIn(t, map[string]string{
		"embed/note/hello.yaml":      "title: Hello\n",
		"embed/note/hello.i18n.yaml": "",
	})
	conv, err := ConvertOverwrites(context.Background(), dir, "ktw-example", noteKinds(), "overrides/ktw-example")
	if err != nil {
		t.Fatal(err)
	}
	if len(conv.Overrides) != 1 {
		t.Fatalf("overrides = %+v", conv.Overrides)
	}
	if _, ok := conv.Overrides[0].Layers[contribapi.LayerTranslations]; ok {
		t.Fatalf("layers = %v; want no translations layer", conv.Overrides[0].Layers)
	}
	if strings.Join(conv.Overrides[0].From, ",") != "embed/note/hello.yaml" {
		t.Fatalf("from = %v", conv.Overrides[0].From)
	}
	if conv.Retirable() || len(conv.Unmapped) != 1 || !strings.HasPrefix(conv.Unmapped[0], "embed/note/hello.i18n.yaml: ") {
		t.Fatalf("unmapped = %v; want the empty translations file", conv.Unmapped)
	}
}

// Entries hello and hello.world each map only their own files.
func TestAnEntryNeverClaimsAnotherEntrysFiles(t *testing.T) {
	dir := treeIn(t, map[string]string{
		"embed/note/hello.yaml":            "title: Hello\n",
		"embed/note/hello.world.yaml":      "title: World\n",
		"embed/note/hello.world.i18n.yaml": "fr:\n  title: Monde\n",
	})
	conv, err := ConvertOverwrites(context.Background(), dir, "ktw-example", noteKinds(), "overrides/ktw-example")
	if err != nil {
		t.Fatal(err)
	}
	from := map[string]string{}
	for _, o := range conv.Overrides {
		from[o.ID] = strings.Join(o.From, ",")
	}
	want := map[string]string{
		"hello":       "embed/note/hello.yaml",
		"hello.world": "embed/note/hello.world.i18n.yaml,embed/note/hello.world.yaml",
	}
	if len(from) != len(want) || from["hello"] != want["hello"] || from["hello.world"] != want["hello.world"] {
		t.Fatalf("from = %v; want %v", from, want)
	}
	if !conv.Retirable() {
		t.Fatalf("unmapped = %v; want none", conv.Unmapped)
	}
}
