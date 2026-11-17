// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/api/providertest"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
)

// packKinds: a frontmatter kind read from its own folder (data from the
// header, instructions from the body folded into data, the file as source,
// translations beside it, an input schema as a companion data-type), the
// data-type kind its companion is, and a kind with no source block.
func packKinds(t *testing.T) contribapi.Kinds {
	t.Helper()
	note := contribapi.KindDeclaration{Name: "note",
		Layers: map[string]contribapi.LayerDecl{"data": {}, "instructions": {Default: true}, "source": {}, "translations": {}},
		Source: &contribapi.SourceDecl{Folder: "notes", Match: []string{"*.md"}, Format: contribapi.SourceFrontmatter,
			Layers: map[string]contribapi.LayerFrom{
				"data":         {From: contribapi.FromFrontmatter},
				"instructions": {From: contribapi.FromBody, Fold: "instructions"},
				"source":       {From: contribapi.FromFile},
				"translations": {From: contribapi.FromSibling, Suffix: TranslationsSuffix},
			},
			Companions: []contribapi.Companion{{Kind: "data-type", Suffix: ".input.dsl.yaml", ID: "note/{id}/input"}}}}
	dataType := contribapi.KindDeclaration{Name: "data-type", Layers: map[string]contribapi.LayerDecl{"source": {Default: true}}}
	guide := contribapi.KindDeclaration{Name: "guide", Layers: map[string]contribapi.LayerDecl{"data": {Default: true}}}
	var entries []contribapi.Contribution
	for _, d := range []contribapi.KindDeclaration{note, dataType, guide} {
		b, err := json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		c := contribapi.Contribution{Provider: "system", Type: contribapi.ResourceTypeContributionType, ID: d.Name}
		c.SetData(b)
		entries = append(entries, c)
	}
	return contribapi.KindsFrom(entries)
}

// packFiles is one pack exercising every way a file finds its kind.
var packFiles = map[string]string{
	"pack/notes/greet.md":                "---\nname: greet\ndescription: Say hello\n---\n# Greet\n\nSay hello.\n",
	"pack/notes/greet.i18n.yaml":         "fr:\n  description: Dire bonjour\n",
	"pack/notes/greet.input.dsl.yaml":    "name: greet-input\ntypes: {}\n",
	"pack/notes/README.txt":              "not a note",
	"pack/notes/_draft.md":               "---\nname: draft\n---\nwip\n",
	"pack/recipes/pancakes.yaml":         "name: pancakes\n",
	"pack/widgets/clock.yaml":            "name: clock\nface: round\nskill: \":/skill/tell-time\"\n",
	"pack/contribution-type/widget.yaml": "name: widget\nlayers: {data: {default: true}, source: {}}\nsource: {folder: widgets, format: structured, layers: {data: {from: tree}, source: {from: file}}}\n",
	"pack/welcome.yaml":                  "type: guide\ntitle: Welcome\n",
	"pack/loose.yaml":                    "title: no type\n",
	"pack/_bookkeeping/state.yaml":       "x: 1\n",
	"pack/notes/greet.cwm.yaml":          "id: greet\n",
}

// TestPackProvider_ReadsEveryKindOfAPack: each folder finds its kind (by a
// source folder or by the pack's own declaration; a folder no kind claims is
// left out), a root file by its `type:` field; `_`-prefixed and `*.cwm.*` files are left
// out; a frontmatter kind's layers are read as its source block says — and
// every provider keeps the ContributionProvider contract, over a map loader
// and over a folder on disk.
func TestPackProvider_ReadsEveryKindOfAPack(t *testing.T) {
	kinds := packKinds(t)
	dir := t.TempDir()
	for name, content := range packFiles {
		p := filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(name, "pack/")))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A folder loader walks paths under its own root: presented under the
	// same prefix as the map loader's, both are read the same way.
	loaders := map[string]loader.Loader{
		"map":  loader.NewMapLoader(packFiles),
		"disk": prefixed{loader.NewFSLoader(dir), "pack"},
	}
	for name, ldr := range loaders {
		t.Run(name, func(t *testing.T) {
			providers := PackProviders("ktw-test", ldr, "pack", kinds)
			got := map[string]contribapi.Contribution{}
			for _, p := range providers {
				providertest.Run(t, p)
				entries, err := p.Enumerate(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				for _, c := range entries {
					got[c.Type+"/"+c.ID] = c
				}
			}
			var keys []string
			for k := range got {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			// recipes/ is no kind's folder: left out.
			want := []string{"contribution-type/widget", "data-type/note/greet/input", "guide/welcome", "note/greet", "widget/clock"}
			if strings.Join(keys, " ") != strings.Join(want, " ") {
				t.Fatalf("entries = %v, want %v", keys, want)
			}

			note := got["note/greet"]
			var data map[string]any
			if err := json.Unmarshal(note.Data(), &data); err != nil {
				t.Fatalf("note data: %v", err)
			}
			if data["name"] != "greet" || data["instructions"] != "# Greet\n\nSay hello." {
				t.Errorf("note data = %v; want the header and the body folded as instructions", data)
			}
			for _, l := range []string{"instructions", "source", "translations"} {
				if _, ok := note.Layers[l]; !ok {
					t.Errorf("note lacks its %s layer: %v", l, note.Layers)
				}
			}
			var clock map[string]any
			if err := json.Unmarshal(got["widget/clock"].Data(), &clock); err != nil || clock["face"] != "round" {
				t.Errorf("widget data = %v, %v; want its tree, read by the pack's own declaration", clock, err)
			}
			if clock["skill"] != "ktw-test:/skill/tell-time" {
				t.Errorf("widget skill = %v; want the host-relative URI given this provider's name", clock["skill"])
			}
		})
	}
}

// prefixed presents a loader rooted at a pack's folder as if the pack sat
// under a prefix, so both loaders are walked the same way.
type prefixed struct {
	loader.Loader
	prefix string
}

func (p prefixed) Walk() []string {
	var out []string
	for _, f := range p.Loader.Walk() {
		out = append(out, p.prefix+"/"+f)
	}
	return out
}

func (p prefixed) ReadFile(name string) ([]byte, error) {
	return p.Loader.ReadFile(strings.TrimPrefix(name, p.prefix+"/"))
}

func (p prefixed) Exists(name string) bool {
	return p.Loader.Exists(strings.TrimPrefix(name, p.prefix+"/"))
}

// A file that cannot be read as its kind is dropped and recorded — never an
// entry whose header was guessed, never a failed enumeration that loses the
// kind's other entries.
func TestPackProvider_AnUnreadableFileIsDroppedAndRecorded(t *testing.T) {
	files := map[string]string{
		"pack/notes/broken.md": "no header here\n",
		"pack/notes/fine.md":   "---\nname: fine\n---\nok\n",
	}
	for _, p := range PackProviders("ktw-test", loader.NewMapLoader(files), "pack", packKinds(t)) {
		if p.ResourceType != "note" {
			continue
		}
		entries, err := p.Enumerate(context.Background())
		if err != nil || len(entries) != 1 || entries[0].ID != "fine" {
			t.Fatalf("Enumerate = %v, %v; want the readable entry alone", entries, err)
		}
		drops := p.Drops()
		if len(drops) != 1 || !strings.Contains(drops[0].Path, "broken.md") {
			t.Fatalf("Drops = %+v; want broken.md recorded", drops)
		}
		return
	}
	t.Fatal("no provider for the note kind")
}

// A skill-like kind: the id is the data's name whatever the layout (flat,
// grouped, a folder's SKILL.md), a skipped folder's files are part of an
// entry, a file naming no name is dropped — read from a loader rooted at the
// kind's folder itself (`.`), as a universe's skills folder is.
func TestPackProvider_IDByNameSkipFoldersAndARootedLoader(t *testing.T) {
	skill := contribapi.KindDeclaration{Name: "skill",
		Layers: map[string]contribapi.LayerDecl{"data": {}, "instructions": {Default: true}, "source": {}},
		Source: &contribapi.SourceDecl{Match: []string{"*.md"}, ID: contribapi.SourceIDName,
			Skip: []string{"references", "scripts", "assets"}, Format: contribapi.SourceFrontmatter,
			Layers: map[string]contribapi.LayerFrom{
				"data":         {From: contribapi.FromFrontmatter},
				"instructions": {From: contribapi.FromBody},
				"source":       {From: contribapi.FromFile},
			}}}
	files := map[string]string{
		"flat.md":                    "---\nname: flat\n---\nflat\n",
		"group/grouped.md":           "---\nname: grouped\n---\ngrouped\n",
		"folder/SKILL.md":            "---\nname: folder\n---\nfolder\n",
		"folder/references/notes.md": "---\nname: not-a-skill\n---\nref\n",
		"folder/scripts/run.md":      "---\nname: not-a-skill-either\n---\nrun\n",
		"nameless.md":                "---\ndescription: no name\n---\nx\n",
	}
	ps := kindProviders("ktw-test", loader.NewMapLoader(files), ".", skill, nil)
	p := ps[0]
	providertest.Run(t, p)
	entries, err := p.Enumerate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, c := range entries {
		ids = append(ids, c.ID)
	}
	sort.Strings(ids)
	if strings.Join(ids, " ") != "flat folder grouped" {
		t.Fatalf("ids = %v; want the names of the three skills, the skipped folders' files left out", ids)
	}
	if drops := p.Drops(); len(drops) != 1 || !strings.Contains(drops[0].Path, "nameless.md") {
		t.Fatalf("Drops = %+v; want nameless.md, which names no id", drops)
	}
}

// A task-like kind: TASK.md is the entry of its folder (claims) — the folder's
// other files are part of it, never entries — and a flat <name>.md elsewhere
// is an entry; the body folds into data without being a layer of its own.
func TestPackProvider_AClaimingFileOwnsItsFolder(t *testing.T) {
	task := contribapi.KindDeclaration{Name: "task",
		Layers: map[string]contribapi.LayerDecl{"data": {Default: true}, "source": {}},
		Source: &contribapi.SourceDecl{Match: []string{"*.md"}, ID: contribapi.SourceIDName, Claims: "TASK.md",
			Format: contribapi.SourceFrontmatter,
			Layers: map[string]contribapi.LayerFrom{
				"data":   {From: contribapi.FromFrontmatter},
				"body":   {From: contribapi.FromBody, Fold: "body"},
				"source": {From: contribapi.FromFile},
			}}}
	files := map[string]string{
		"review/TASK.md":       "---\nname: review\n---\nReview it.\n",
		"review/checklist.md":  "---\nname: not-a-task\n---\npart of review\n",
		"review/refs/notes.md": "---\nname: nor-this\n---\ndeeper part\n",
		"group/summarise.md":   "---\nname: summarise\n---\nSummarise it.\n",
	}
	p := KindProvider("ktw-test", loader.NewMapLoader(files), ".", task)
	providertest.Run(t, p)
	entries, err := p.Enumerate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]contribapi.Contribution{}
	for _, c := range entries {
		ids[c.ID] = c
	}
	if len(ids) != 2 || ids["review"].ID == "" || ids["summarise"].ID == "" {
		t.Fatalf("entries = %v; want review (its folder's other files part of it) and summarise", ids)
	}
	var data map[string]any
	if err := json.Unmarshal(ids["review"].Data(), &data); err != nil || data["body"] != "Review it." {
		t.Fatalf("review data = %v, %v; want the body folded", data, err)
	}
	if _, layer := ids["review"].Layers["body"]; layer {
		t.Fatal("a part folded into data is not a layer of its own")
	}
}

// TestPackProvider_ReadsWhatAKindClaims: a pack read at its loader's root
// publishes the folders a kind claims and nothing else; a remap reads a kind
// from another folder (nested or not) and not from its own, switches one off
// with "", and reads a kind no binary declares the generic way; a folder only
// the pack's layer manifest records (bulk content gone from disk) is read.
func TestPackProvider_ReadsWhatAKindClaims(t *testing.T) {
	files := map[string]string{
		"notes/greet.md":             "---\nname: greet\n---\nHello.\n",
		"content/guides/tour.yaml":   "title: Tour\n",
		"guide/welcome.yaml":         "title: Welcome\n",
		"config/settings.yaml":       "x: 1\n",
		"docs/readme.yaml":           "title: not a kind\n",
		"sketches/a.yaml":            "title: A\n",
		contribapi.LayerManifestFile: "apiVersion: 1\nlayers:\n  data-type/big.yaml:\n    hash: sha256:" + strings.Repeat("a", 64) + "\n    size: 12\n",
	}
	read := func(opts ...PackOption) []string {
		var out []string
		for _, p := range PackProviders("ktw-test", loader.NewMapLoader(files), ".", packKinds(t), opts...) {
			providertest.Run(t, p)
			got, err := p.Enumerate(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range got {
				out = append(out, c.Type+"/"+c.ID)
			}
		}
		sort.Strings(out)
		return out
	}
	check := func(what string, got []string, want ...string) {
		t.Helper()
		if strings.Join(got, " ") != strings.Join(want, " ") {
			t.Errorf("%s: entries = %v, want %v", what, got, want)
		}
	}
	// config/, docs/ and sketches/ are no kind's folder; data-type/ is only
	// recorded, and read.
	check("claimed", read(), "data-type/big", "guide/welcome", "note/greet")
	check("guide moved", read(WithRemap(map[string]string{"guide": "content/guides"})),
		"data-type/big", "guide/tour", "note/greet")
	check("notes off", read(WithRemap(map[string]string{"note": ""})), "data-type/big", "guide/welcome")
	check("an undeclared kind", read(WithRemap(map[string]string{"sketch": "sketches"})),
		"data-type/big", "guide/welcome", "note/greet", "sketch/a")
	// A remap may take another kind's folder: it is read as the remapped kind
	// only.
	check("guide/ taken", read(WithRemap(map[string]string{"sketch": "guide"})),
		"data-type/big", "note/greet", "sketch/welcome")
}

// A folder of a kind's former name (its source's `former:`) is read as that
// kind while the pack holds no folder of the current name; beside the current
// folder it is not.
func TestPackProvider_ReadsAFormerFolder(t *testing.T) {
	note := contribapi.KindDeclaration{Name: "note", Layers: map[string]contribapi.LayerDecl{"data": {Default: true}, "source": {}},
		Source: &contribapi.SourceDecl{Folder: "notes", Former: []string{"note_files"}, Format: contribapi.SourceStructured,
			Layers: map[string]contribapi.LayerFrom{"data": {From: contribapi.FromTree}, "source": {From: contribapi.FromFile}}}}
	b, err := json.Marshal(note)
	if err != nil {
		t.Fatal(err)
	}
	decl := contribapi.Contribution{Provider: "system", Type: contribapi.ResourceTypeContributionType, ID: "note"}
	decl.SetData(b)
	kinds := contribapi.KindsFrom([]contribapi.Contribution{decl})
	read := func(files map[string]string) []string {
		var out []string
		for _, p := range PackProviders("ktw-test", loader.NewMapLoader(files), ".", kinds) {
			got, err := p.Enumerate(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range got {
				out = append(out, c.Type+"/"+c.ID)
			}
		}
		sort.Strings(out)
		return out
	}
	if got := read(map[string]string{"note_files/a.yaml": "x: 1\n"}); strings.Join(got, " ") != "note/a" {
		t.Errorf("former folder alone: %v, want note/a", got)
	}
	if got := read(map[string]string{"notes/b.yaml": "x: 1\n", "note_files/a.yaml": "x: 1\n"}); strings.Join(got, " ") != "note/b" {
		t.Errorf("beside the current folder: %v, want note/b alone", got)
	}
}
