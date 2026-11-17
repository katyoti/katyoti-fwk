// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// THREE SOURCES, closer wins (decided 2026-09-26): a capability's files, the
// object type's, the object's own — the last declared at runtime, one object
// at a time, through ObjectNamedFileDeclare.

func sourcesFixture(t *testing.T) (*Service, string) {
	t.Helper()
	root := freshWorkspace(t)
	project := t.TempDir()
	cfg := &CWMServiceConfig{WorkspaceRoot: root, ProjectRoot: project,
		ObjectTypeValidation: api.TypeValidationError}
	cfg.CatalogTypes.Capabilities = []api.Capability{
		{Name: "container"},
		{Name: "memory", Files: map[string]api.NamedFile{"notes": {Path: "memory/${id}.md"}}},
		{Name: "absent", Files: map[string]api.NamedFile{"x": {Path: "absent.md"}}},
	}
	cfg.CatalogTypes.Objects = []api.ObjectType{{
		Name: "doc", Capabilities: []string{"container", "memory"},
		Files: map[string]api.NamedFile{
			"spec": {Path: "specs/${id}.md", Media: "text/markdown"}, // the object's own, from its type
			// the type REPLACES the memory capability's notes for its objects
			"notes": {Path: "type-notes/${id}.md", Capability: "memory"},
		},
	}}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	o := api.Object{ID: "billing.spec", Type: "doc"}
	o.Name = "Spec"
	if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "billing", Object: o}); err != nil {
		t.Fatalf("ObjectCreate: %v", err)
	}
	return svc, project
}

func namedByKey(t *testing.T, svc *Service) map[string]api.ObjectNamedFile {
	t.Helper()
	out, err := svc.ObjectNamedFileList(stdctx.Background(), api.ObjectNamedFileListInput{ID: "billing.spec"})
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]api.ObjectNamedFile{}
	for _, f := range out.Files {
		m[f.Capability+"/"+f.Name] = f
	}
	return m
}

// TestNamedFileFromTheTypeAndCloserWins — a type gives its objects a file of
// their own, and a type declaration replaces a capability's for the same key.
func TestNamedFileFromTheTypeAndCloserWins(t *testing.T) {
	svc, _ := sourcesFixture(t)
	m := namedByKey(t, svc)
	if f := m["/spec"]; f.Source != api.NamedFileFromType || f.Path != "specs/billing.spec.md" {
		t.Errorf("type file: %+v", f)
	}
	if f := m["memory/notes"]; f.Source != api.NamedFileFromType || f.Path != "type-notes/billing.spec.md" {
		t.Errorf("the type must replace the capability's declaration of the same key: %+v", f)
	}
	if len(m) != 2 {
		t.Errorf("one entry per key, got %v", m)
	}
}

// TestNamedFileDeclaredOnTheObject — a capability's code gives ONE object a
// file at runtime; it is stored on the object, read and written by name, and
// it is the closest source.
func TestNamedFileDeclaredOnTheObject(t *testing.T) {
	svc, project := sourcesFixture(t)
	ctx := stdctx.Background()

	d, err := svc.ObjectNamedFileDeclare(ctx, api.ObjectNamedFileDeclareInput{ID: "billing.spec", Name: "draft",
		File: api.NamedFile{Path: "drafts/${id}.txt"}})
	if err != nil || d.File.Source != api.NamedFileFromObject || d.File.Path != "drafts/billing.spec.txt" {
		t.Fatalf("declare: %+v %v", d, err)
	}
	if _, err := svc.ObjectNamedFileWrite(ctx, api.ObjectNamedFileWriteInput{ID: "billing.spec", Name: "draft", Content: []byte("hello")}); err != nil {
		t.Fatalf("write an object's own file (no capability): %v", err)
	}
	got, _ := svc.ObjectNamedFileRead(ctx, api.ObjectNamedFileReadInput{ID: "billing.spec", Name: "draft"})
	if string(got.Content) != "hello" {
		t.Errorf("read back %q", got.Content)
	}
	// Stored ON THE OBJECT: the object file carries the declaration.
	raw, _ := os.ReadFile(filepath.Join(project, "modules", "billing", "objects", "spec.cwm.yaml"))
	if !strings.Contains(string(raw), "draft:") || !strings.Contains(string(raw), "drafts/${id}.txt") {
		t.Errorf("the declaration must live in the object file:\n%s", raw)
	}

	// The object's own declaration of a capability-owned key wins over the type's.
	if _, err := svc.ObjectNamedFileDeclare(ctx, api.ObjectNamedFileDeclareInput{ID: "billing.spec", Name: "notes",
		File: api.NamedFile{Path: "mine/${id}.md", Capability: "memory"}}); err != nil {
		t.Fatalf("declare over the type: %v", err)
	}
	if f := namedByKey(t, svc)["memory/notes"]; f.Source != api.NamedFileFromObject || f.Path != "mine/billing.spec.md" {
		t.Errorf("the object's declaration must win: %+v", f)
	}
	// Removing it puts the farther one back in force.
	back, err := svc.ObjectNamedFileDeclare(ctx, api.ObjectNamedFileDeclareInput{ID: "billing.spec", Name: "notes", Remove: true})
	if err != nil || back.File.Source != api.NamedFileFromType || back.File.Path != "type-notes/billing.spec.md" {
		t.Errorf("remove must restore the type's declaration: %+v %v", back, err)
	}
}

// TestNamedFileDeclareRefusesWhatCouldNeverBeRead — checked before stored:
// nothing is written for a declaration that is wrong.
func TestNamedFileDeclareRefusesWhatCouldNeverBeRead(t *testing.T) {
	svc, project := sourcesFixture(t)
	ctx := stdctx.Background()
	objFile := filepath.Join(project, "modules", "billing", "objects", "spec.cwm.yaml")
	before, _ := os.ReadFile(objFile)
	for _, c := range []struct {
		name string
		file api.NamedFile
		want string
	}{
		{"owner not in force", api.NamedFile{Path: "a.md", Capability: "absent"}, "not in force"},
		{"unresolvable", api.NamedFile{Path: "x/${current.nope}.md"}, "does not resolve"},
		{"escapes", api.NamedFile{Path: "../out.md"}, ".."},
		{"object file", api.NamedFile{Path: "objects/other.cwm.yaml"}, "object file"},
		{"no path", api.NamedFile{}, "no path"},
	} {
		_, err := svc.ObjectNamedFileDeclare(ctx, api.ObjectNamedFileDeclareInput{ID: "billing.spec", Name: "bad", File: c.file})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want %q", c.name, err, c.want)
		}
	}
	after, _ := os.ReadFile(objFile)
	if string(after) != string(before) {
		t.Errorf("a refused declaration must store nothing:\n%s", after)
	}
}
