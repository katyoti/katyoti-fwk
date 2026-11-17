// SPDX-License-Identifier: Apache-2.0

package loader

import (
	"sort"
	"testing"
)

// TestOverlayLoader_Shadow: a well-formed overlay file wins over the base.
func TestOverlayLoader_Shadow(t *testing.T) {
	base := NewMapLoader(map[string]string{
		"skills/greet.md": "---\nname: greet\n---\nEMBED body",
	})
	overlay := NewMapLoader(map[string]string{
		"skills/greet.md": "---\nname: greet\n---\nOVERWRITE body",
	})
	o := NewOverlayLoader(base, overlay, nil)

	got, _ := o.ReadFile("skills/greet.md")
	if string(got) != "---\nname: greet\n---\nOVERWRITE body" {
		t.Fatalf("overlay must shadow base; got %q", got)
	}
	if !o.Exists("skills/greet.md") {
		t.Error("Exists must be true for a shadowed path")
	}
}

// TestOverlayLoader_NewResource: a path only the overlay has is served (create
// without recompiling the binary).
func TestOverlayLoader_NewResource(t *testing.T) {
	base := NewMapLoader(map[string]string{
		"skills/greet.md": "---\nname: greet\n---\nbody",
	})
	overlay := NewMapLoader(map[string]string{
		"skills/new.md": "---\nname: new\n---\nbrand new skill",
	})
	o := NewOverlayLoader(base, overlay, nil)

	got, _ := o.ReadFile("skills/new.md")
	if string(got) != "---\nname: new\n---\nbrand new skill" {
		t.Fatalf("overlay-only resource must be served; got %q", got)
	}

	list := o.List("skills/*.md")
	sort.Strings(list)
	want := []string{"skills/greet.md", "skills/new.md"}
	if len(list) != 2 || list[0] != want[0] || list[1] != want[1] {
		t.Fatalf("List must union base+overlay; got %v", list)
	}
}

// TestOverlayLoader_MalformedFallsBack: an overlay file that fails the accept
// guard (no frontmatter delimiter / empty) must NOT shadow — the released embed
// copy is served instead. This is the robustness probe (gate c).
func TestOverlayLoader_MalformedFallsBack(t *testing.T) {
	base := NewMapLoader(map[string]string{
		"skills/greet.md": "---\nname: greet\n---\nEMBED body",
	})
	overlay := NewMapLoader(map[string]string{
		"skills/greet.md": "garbage truncated file, no frontmatter",
		"skills/empty.md": "   \n  ",
	})
	o := NewOverlayLoader(base, overlay, nil)

	got, _ := o.ReadFile("skills/greet.md")
	if string(got) != "---\nname: greet\n---\nEMBED body" {
		t.Fatalf("malformed overlay must fall back to embed; got %q", got)
	}

	// An empty overlay-only file is not served (and Exists is false there).
	if o.Exists("skills/empty.md") {
		t.Error("an empty overlay-only file must not register as Exists")
	}
}

// TestOverlayLoader_NonMarkdownAcceptedRaw: the default guard only frontmatter-
// checks .md files; other types (yaml, json) pass on non-empty.
func TestOverlayLoader_NonMarkdownAcceptedRaw(t *testing.T) {
	base := NewMapLoader(map[string]string{"data/x.yaml": "from: embed"})
	overlay := NewMapLoader(map[string]string{"data/x.yaml": "from: overlay"})
	o := NewOverlayLoader(base, overlay, nil)

	got, _ := o.ReadFile("data/x.yaml")
	if string(got) != "from: overlay" {
		t.Fatalf("non-md overlay must shadow on non-empty; got %q", got)
	}
}
