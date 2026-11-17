// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// NAMED FILES (decided 2026-09-26): a capability declares the files it
// manages; the verbs resolve the declaration for one object and go through the
// ObjectFile* rules — the module root, `..` refused, object files unreachable.

func namedFileFixture(t *testing.T) (*Service, string) {
	t.Helper()
	root := freshWorkspace(t)
	project := t.TempDir()
	cfg := &CWMServiceConfig{WorkspaceRoot: root, ProjectRoot: project,
		ObjectTypeValidation: api.TypeValidationError}
	cfg.CatalogTypes.Objects = []api.ObjectType{
		{Name: "doc", Capabilities: []string{"container", "content", "memory"}},
		{Name: "plain", Capabilities: []string{"container"}},
	}
	cfg.CatalogTypes.Capabilities = []api.Capability{
		{Name: "container"}, {Name: "content"},
		{Name: "memory", Files: map[string]api.NamedFile{
			"notes":   {Path: "memory/${id}.md", Media: "text/markdown", Description: "What the agent keeps"},
			"shared":  {Path: "memory/shared.md"},
			"unbound": {Path: "memory/${current.nope}.md"},
			"escape":  {Path: "../outside.md"},
			"object":  {Path: "objects/${id}.cwm.yaml"},
		}},
	}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	for _, o := range []api.Object{{ID: "billing.spec", Type: "doc"}, {ID: "billing.other", Type: "plain"}} {
		o.Name = o.ID
		if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "billing", Object: o}); err != nil {
			t.Fatalf("ObjectCreate %s: %v", o.ID, err)
		}
	}
	return svc, project
}

// TestNamedFileRoundTrip — the declared path resolves for the object, a file
// not written yet reads empty (not an error), a write lands in the MODULE, and
// append appends.
func TestNamedFileRoundTrip(t *testing.T) {
	svc, project := namedFileFixture(t)
	ctx := stdctx.Background()

	got, err := svc.ObjectNamedFileRead(ctx, api.ObjectNamedFileReadInput{ID: "billing.spec", Capability: "memory", Name: "notes"})
	if err != nil || got.File.Exists || len(got.Content) != 0 {
		t.Fatalf("an unwritten named file reads empty: %+v %v", got, err)
	}
	if got.File.Path != "memory/billing.spec.md" {
		t.Errorf("resolved path %q, want memory/billing.spec.md", got.File.Path)
	}

	w, err := svc.ObjectNamedFileWrite(ctx, api.ObjectNamedFileWriteInput{ID: "billing.spec", Capability: "memory", Name: "notes", Content: []byte("one\n")})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, "modules", "billing", "memory", "billing.spec.md")); err != nil {
		t.Errorf("the file must live in the module: %v (project path %q)", err, w.ProjectPath)
	}
	if _, err := svc.ObjectNamedFileWrite(ctx, api.ObjectNamedFileWriteInput{ID: "billing.spec", Capability: "memory", Name: "notes", Content: []byte("two\n"), Append: true}); err != nil {
		t.Fatalf("append: %v", err)
	}
	got, _ = svc.ObjectNamedFileRead(ctx, api.ObjectNamedFileReadInput{ID: "billing.spec", Capability: "memory", Name: "notes"})
	if string(got.Content) != "one\ntwo\n" || !got.File.Exists {
		t.Errorf("read back %q (exists=%v), want both lines", got.Content, got.File.Exists)
	}
}

// TestNamedFileListReportsEachDeclaration — every declared file, resolved, with
// its state; a declaration that cannot resolve is REPORTED on its row, and does
// not hide the others.
func TestNamedFileListReportsEachDeclaration(t *testing.T) {
	svc, _ := namedFileFixture(t)
	ctx := stdctx.Background()
	_, _ = svc.ObjectNamedFileWrite(ctx, api.ObjectNamedFileWriteInput{ID: "billing.spec", Capability: "memory", Name: "shared", Content: []byte("x")})
	out, err := svc.ObjectNamedFileList(ctx, api.ObjectNamedFileListInput{ID: "billing.spec"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Module != "billing" || len(out.Files) != 5 {
		t.Fatalf("list: module %q, %d files: %+v", out.Module, len(out.Files), out.Files)
	}
	by := map[string]api.ObjectNamedFile{}
	for _, f := range out.Files {
		by[f.Name] = f
	}
	if !by["shared"].Exists || by["shared"].SizeBytes != 1 || by["notes"].Exists {
		t.Errorf("existence wrong: shared %+v notes %+v", by["shared"], by["notes"])
	}
	for _, bad := range []string{"unbound", "escape", "object"} {
		if by[bad].Error == "" {
			t.Errorf("%s must report its error on its row: %+v", bad, by[bad])
		}
	}
}

// TestNamedFileRefusals — a capability not in force, an undeclared name, a
// path that does not resolve, escapes the module, or names an object file.
func TestNamedFileRefusals(t *testing.T) {
	svc, _ := namedFileFixture(t)
	ctx := stdctx.Background()
	cases := []struct {
		name string
		in   api.ObjectNamedFileWriteInput
		want string
	}{
		{"capability not in force", api.ObjectNamedFileWriteInput{ID: "billing.other", Capability: "memory", Name: "notes"}, "not in force"},
		{"undeclared name", api.ObjectNamedFileWriteInput{ID: "billing.spec", Capability: "memory", Name: "nope"}, "declares no file"},
		{"unresolvable ${...}", api.ObjectNamedFileWriteInput{ID: "billing.spec", Capability: "memory", Name: "unbound"}, "does not resolve"},
		{"escapes the module", api.ObjectNamedFileWriteInput{ID: "billing.spec", Capability: "memory", Name: "escape"}, ".."},
		{"object file", api.ObjectNamedFileWriteInput{ID: "billing.spec", Capability: "memory", Name: "object"}, "object file"},
	}
	for _, c := range cases {
		c.in.Content = []byte("x")
		_, err := svc.ObjectNamedFileWrite(ctx, c.in)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want it to say %q", c.name, err, c.want)
		}
	}
	if _, err := svc.ObjectNamedFileRead(ctx, api.ObjectNamedFileReadInput{ID: "billing.spec", Capability: "memory", Name: "nope"}); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("an undeclared name reads as not found, got %v", err)
	}
}

// TestNamedFileReadForgivesAMissingCapability — a read with no capability, for
// a name the object does not declare itself, resolves to the ONE capability in
// force that declares it (a chat asking for `working` means the memory's); with
// two such capabilities it is refused, naming them; a capability not in force
// never matches.
func TestNamedFileReadForgivesAMissingCapability(t *testing.T) {
	svc, _ := namedFileFixture(t)
	ctx := stdctx.Background()
	if _, err := svc.ObjectNamedFileWrite(ctx, api.ObjectNamedFileWriteInput{ID: "billing.spec", Capability: "memory", Name: "notes", Content: []byte("kept")}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.ObjectNamedFileRead(ctx, api.ObjectNamedFileReadInput{ID: "billing.spec", Name: "notes"})
	if err != nil || string(got.Content) != "kept" || got.File.Capability != "memory" {
		t.Fatalf("a read without capability must resolve the one declaring capability: %+v %v", got, err)
	}
	if _, err := svc.ObjectNamedFileRead(ctx, api.ObjectNamedFileReadInput{ID: "billing.other", Name: "notes"}); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("a capability not in force must not match, got %v", err)
	}

	two := []namedFileEntry{{capability: "memory", name: "working"}, {capability: "journal", name: "working"}}
	_, err = findNamedFileToRead(two, []string{"memory", "journal"}, "", "working")
	if !errors.Is(err, api.ErrNotFound) || !strings.Contains(err.Error(), "memory, journal") {
		t.Errorf("two declaring capabilities must be refused, naming both: %v", err)
	}
	if e, err := findNamedFileToRead(two, []string{"memory"}, "", "working"); err != nil || e.capability != "memory" {
		t.Errorf("only the capability in force counts: %+v %v", e, err)
	}
}
