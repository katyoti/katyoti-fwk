// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"io"
	"strings"
	"testing"
	"testing/fstest"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/api/providertest"

	"github.com/katyoti/katyoti-fwk/internal/core/loader"
)

func makeFS() fstest.MapFS {
	return fstest.MapFS{
		"embed/skills/doc-create.md": {
			Data: []byte("# Doc Create\nA skill"),
		},
		"embed/skills/diagram.md": {
			Data: []byte("# Diagram\nAnother"),
		},
		"embed/skills/category/nested.md": {
			Data: []byte("# Nested\nOK"),
		},
		"embed/skills/notes.txt": {
			Data: []byte("ignored — wrong ext"),
		},
		"embed/references/templates/report.typ": {
			Data: []byte("typst content"),
		},
		"embed/references/schemas/diagram.yaml": {
			Data: []byte("yaml content"),
		},
	}
}

func TestEmbedFSProvider_Skill_Flat_Enumerate(t *testing.T) {
	ldr := loader.NewEmbedFSLoader(makeFS(), ".")
	p := &EmbedFSResourceProvider{
		ProviderName:        "ktw-test",
		ResourceType:        "skill",
		Loader:              ldr,
		Root:                "embed/skills",
		KeepFolderStructure: false,
		FileExtensions:      []string{"md"},
	}

	entries, err := p.Enumerate(context.Background())
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	// 3 .md files (doc-create, diagram, category/nested); .txt excluded.
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}

	gotIDs := map[string]bool{}
	for _, e := range entries {
		gotIDs[e.ID] = true
		if e.Provider != "ktw-test" {
			t.Errorf("provider = %q", e.Provider)
		}
		if e.Type != "skill" {
			t.Errorf("type = %q", e.Type)
		}
	}
	want := []string{"doc-create", "diagram", "category-nested"}
	for _, id := range want {
		if !gotIDs[id] {
			t.Errorf("missing ID %q (got: %v)", id, gotIDs)
		}
	}
}

func TestEmbedFSProvider_Reference_Tree_Enumerate(t *testing.T) {
	ldr := loader.NewEmbedFSLoader(makeFS(), ".")
	p := &EmbedFSResourceProvider{
		ProviderName:        "ktw-docs",
		ResourceType:        "reference",
		Loader:              ldr,
		Root:                "embed/references",
		KeepFolderStructure: true,
	}

	entries, err := p.Enumerate(context.Background())
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}

	gotIDs := map[string]bool{}
	for _, e := range entries {
		gotIDs[e.ID] = true
	}
	// In tree mode, the path structure is preserved (extension stripped from basename).
	for _, want := range []string{"templates/report", "schemas/diagram"} {
		if !gotIDs[want] {
			t.Errorf("missing ID %q (got: %v)", want, gotIDs)
		}
	}
}

// Open addresses an entry by the ID Enumerate derived (flat mode strips the
// extension) and serves the file's bytes as the source layer.
func TestEmbedFSProvider_Open_ReturnsSourceBytes(t *testing.T) {
	ldr := loader.NewEmbedFSLoader(makeFS(), ".")
	p := &EmbedFSResourceProvider{
		ProviderName:        "ktw-test",
		ResourceType:        "skill",
		Loader:              ldr,
		Root:                "embed/skills",
		KeepFolderStructure: false,
		FileExtensions:      []string{"md"},
	}

	rc, ref, err := p.Open(context.Background(), "doc-create", contribapi.LayerSource)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	body, _ := io.ReadAll(rc)
	_ = rc.Close()
	if !strings.HasPrefix(string(body), "# Doc Create") {
		t.Errorf("bytes = %q", body)
	}
	if ref.MediaType != "text/markdown" {
		t.Errorf("MediaType = %q", ref.MediaType)
	}
	if ref.Hash != contribapi.HashContent(body) || ref.Inline != nil {
		t.Errorf("ref = %+v, want the source layer's address without inline bytes", ref)
	}

	providertest.Run(t, p)
}

func TestEmbedFSProvider_TypeAndProvider(t *testing.T) {
	p := &EmbedFSResourceProvider{ProviderName: "ktw-x", ResourceType: "task"}
	if p.Type() != "task" {
		t.Errorf("Type = %q", p.Type())
	}
	if p.Provider() != "ktw-x" {
		t.Errorf("Provider = %q", p.Provider())
	}
}

func TestEmbedFSProvider_ImplementsInterface(t *testing.T) {
	var _ contribapi.ContributionProvider = &EmbedFSResourceProvider{}
}
