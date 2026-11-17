// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"os"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// A MOVE KEEPS THE BODY. A move changes where an object sits — its parent, its
// module, its local id — and keeps everything else: a `.cwm.md` object's
// markdown body above all, byte for byte, and its own data (tags, description,
// edges, named files).

const movedBody = "## Why\n\nThe criterion exists because **nothing** else proves it.\n\n- one\n- two\n"

// bodyOnDisk is the markdown below the frontmatter of the object's file.
func bodyOnDisk(t *testing.T, svc *Service, id string) string {
	t.Helper()
	p, ok := svc.index().pathOf(id)
	if !ok {
		t.Fatalf("%s is not indexed", id)
	}
	if !strings.HasSuffix(p, api.ObjectFileSuffixMD) {
		t.Fatalf("%s is written as %s, want a %s", id, p, api.ObjectFileSuffixMD)
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	_, body, ok := splitObjectFrontmatter(string(raw))
	if !ok {
		t.Fatalf("%s has no frontmatter fence", p)
	}
	return strings.TrimLeft(body, "\r\n")
}

func TestAMoveKeepsTheMarkdownBodyAndTheObjectsData(t *testing.T) {
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir(), ObjectTypeValidation: api.TypeValidationError}
	cfg.CatalogTypes.Objects = []api.ObjectType{
		{Name: "note", Capabilities: []string{"container", "content"}},
		{Name: "module", Capabilities: []string{"container"}},
	}
	cfg.CatalogTypes.Capabilities = []api.Capability{{Name: "container"}, {Name: "content"}}
	cfg.CatalogTypes.Edges = []api.EdgeTypeRegistration{{Name: "about", From: "any", To: "any", Cardinality: "M:N", Target: "object", ObjectStorage: "edges"}}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	ctx := stdctx.Background()
	ac := api.Object{ID: "m.ac", Type: "note", Name: "An acceptance criterion", Parent: "m.p1",
		Description: "what it asks", Tags: []string{"keep"},
		Edges: []api.ObjectEdge{{Kind: "about", To: "m.p1"}},
		Files: map[string]api.NamedFile{"notes": {Path: "notes/ac.md", Description: "named only"}}}
	ac.Body = movedBody
	for _, o := range []api.Object{
		{ID: "m", Type: "module", Name: "m"}, {ID: "k", Type: "module", Name: "k"},
		{ID: "m.p1", Type: "note", Name: "p1", Parent: "m"},
		{ID: "m.p2", Type: "note", Name: "p2", Parent: "m"},
		{ID: "k.p3", Type: "note", Name: "p3", Parent: "k"},
		ac,
	} {
		module, _ := api.ModuleOfObjectID(o.ID)
		if o.Type == "module" {
			module = o.ID
		}
		if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: module, Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}
	before := bodyOnDisk(t, svc, "m.ac")
	if before != movedBody {
		t.Fatalf("the body as written: %q, want %q", before, movedBody)
	}

	check := func(step, id, module, parent string) {
		t.Helper()
		if got := bodyOnDisk(t, svc, id); got != before {
			t.Errorf("%s: the file's body is %q, want it byte-identical: %q", step, got, before)
		}
		got, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: module, ID: id})
		if err != nil {
			t.Fatalf("%s: get %s: %v", step, id, err)
		}
		o := got.Object
		if o.Body != movedBody {
			t.Errorf("%s: object-get returns the body %q, want %q", step, o.Body, movedBody)
		}
		if o.ParentID() != parent {
			t.Errorf("%s: parent %q, want %q", step, o.ParentID(), parent)
		}
		if o.Name != ac.Name || o.Description != ac.Description || len(o.Tags) != 1 || o.Tags[0] != "keep" {
			t.Errorf("%s: name / description / tags lost: %q %q %v", step, o.Name, o.Description, o.Tags)
		}
		if len(o.Edges) != 1 || o.Edges[0].To != "m.p1" {
			t.Errorf("%s: edges %+v, want the one about m.p1", step, o.Edges)
		}
		if f, ok := o.Files["notes"]; !ok || f.Path != "notes/ac.md" {
			t.Errorf("%s: named files %+v, want notes", step, o.Files)
		}
	}

	// Under another object of its module: the same id, a new parent.
	out, err := svc.ObjectMove(ctx, api.ObjectMoveInput{ID: "m.ac", NewParent: "m.p2", Reason: "belongs under p2"})
	if err != nil {
		t.Fatalf("move under m.p2: %v", err)
	}
	if out.ID != "m.ac" {
		t.Fatalf("a move within the module minted %q", out.ID)
	}
	check("same module", "m.ac", "m", "m.p2")

	// Renamed in place.
	if _, err := svc.ObjectMove(ctx, api.ObjectMoveInput{ID: "m.ac", NewLocalID: "ac2", Reason: "clearer"}); err != nil {
		t.Fatalf("rename: %v", err)
	}
	check("renamed", "m.ac2", "m", "m.p2")

	// Refiled into another module.
	if _, err := svc.ObjectMove(ctx, api.ObjectMoveInput{ID: "m.ac2", NewModule: "k", NewParent: "k.p3", Reason: "belongs to k"}); err != nil {
		t.Fatalf("move to k: %v", err)
	}
	check("other module", "k.ac2", "k", "k.p3")
}
