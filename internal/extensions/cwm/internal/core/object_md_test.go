// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// AN OBJECT WITH PROSE IS A `.cwm.md` (phase 7c): frontmatter carries the
// object, the markdown body carries the prose, nothing lives in both, the body
// is never parsed.

func mdFixture(t *testing.T) (*Service, string) {
	t.Helper()
	root := freshWorkspace(t)
	project := t.TempDir()
	cfg := &CWMServiceConfig{WorkspaceRoot: root, ProjectRoot: project, ObjectTypeValidation: api.TypeValidationError}
	cfg.CatalogTypes.Objects = []api.ObjectType{
		{Name: "decision", Capabilities: []string{"container", "content", "decision"}, Folder: "decisions", IDPrefix: "OD"},
		{Name: "note", Capabilities: []string{"container", "content"}},
		{Name: "record", Capabilities: []string{"content"}, Runtime: true},
		{Name: "module", Capabilities: []string{"container"}},
	}
	cfg.CatalogTypes.Capabilities = []api.Capability{{Name: "container"}, {Name: "content"}, {Name: "decision"}, {Name: "want"}}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	return svc, project
}

// ROUND TRIP: yaml → md → yaml is equal on capabilities and edges.
func TestObjectRoundTripsThroughMarkdown(t *testing.T) {
	dir := t.TempDir()
	o := api.Object{ID: "m.OD-001", Type: "decision", Edges: []api.ObjectEdge{{Kind: "serves", To: "m.g1"}}}
	o.Name = "OD-001"
	o.Parent = "m"
	o.Name = "Which store?"
	o.Tags = []string{"storage"}
	o.Body = "## Question\n\nTwo stores or one?\n\n- a\n- b\n"
	_ = o.SetCapability("decision", map[string]any{"options": []any{"a", "b"}, "raised": "2026-09-25"})

	yamlPath := filepath.Join(dir, "a"+ObjectFileSuffix)
	mdPath := filepath.Join(dir, "a"+api.ObjectFileSuffixMD)
	if err := WriteObjectFile(yamlPath, o); err != nil {
		t.Fatal(err)
	}
	fromYAML, err := ReadObjectFile(yamlPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteObjectFile(mdPath, fromYAML); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(mdPath)
	text := string(raw)
	if !strings.HasPrefix(text, "---\n") || !strings.Contains(text, "\n---\n## Question") {
		t.Fatalf("md shape:\n%s", text)
	}
	if strings.Contains(strings.SplitN(text, "\n---\n", 2)[0], "body:") {
		t.Errorf("the frontmatter carries the body — nothing lives in both:\n%s", text)
	}
	fromMD, err := ReadObjectFile(mdPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(jsonRound(t, fromYAML.Capabilities), jsonRound(t, fromMD.Capabilities)) {
		t.Errorf("capabilities differ after the md round trip:\n yaml: %v\n md:   %v", fromYAML.Capabilities, fromMD.Capabilities)
	}
	if !reflect.DeepEqual(fromYAML.Edges, fromMD.Edges) {
		t.Errorf("edges differ: %v vs %v", fromYAML.Edges, fromMD.Edges)
	}
	// And back to yaml, byte-equal on the parsed object.
	backPath := filepath.Join(dir, "b"+ObjectFileSuffix)
	if err := WriteObjectFile(backPath, fromMD); err != nil {
		t.Fatal(err)
	}
	back, _ := ReadObjectFile(backPath)
	if !reflect.DeepEqual(jsonRound(t, back), jsonRound(t, fromYAML)) {
		t.Errorf("yaml → md → yaml is not the same object")
	}
}

func jsonRound(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	_ = json.Unmarshal(b, &out)
	return out
}

// A body in BOTH places is refused on read; a structured body cannot be a md body.
func TestMarkdownBodyLivesInOnePlaceOnly(t *testing.T) {
	dir := t.TempDir()
	both := "---\nid: m.x\ntype: note\nname: X\nbody: repeated\n---\nthe prose\n"
	p := filepath.Join(dir, "x"+api.ObjectFileSuffixMD)
	_ = os.WriteFile(p, []byte(both), 0o644)
	if _, err := ReadObjectFile(p); !errors.Is(err, api.ErrInvalidInput) {
		t.Errorf("a body in both the frontmatter and the markdown: %v, want refused", err)
	}
	// The type's fields are data: they ride in the frontmatter, the markdown
	// stays the body.
	o := api.Object{ID: "m.y", Type: "note", Body: "the prose\n"}
	o.Name = "Y"
	o.SetFieldsJSON(json.RawMessage(`{"amount": 3}`))
	yp := filepath.Join(dir, "y"+api.ObjectFileSuffixMD)
	if err := WriteObjectFile(yp, o); err != nil {
		t.Fatal(err)
	}
	back, err := ReadObjectFile(yp)
	if err != nil || back.Body != "the prose\n" || back.Fields["amount"] != float64(3) && back.Fields["amount"] != 3 {
		t.Errorf("fields and prose in one .cwm.md: %+v %v", back, err)
	}
}

// THE SPELLING FOLLOWS THE OBJECT: a prose body is written as .cwm.md, a data
// body or none as .cwm.yaml; the index reads both; ObjectGet hydrates the prose.
func TestCreateWritesMarkdownForProseAndIndexesBoth(t *testing.T) {
	svc, project := mdFixture(t)
	ctx := stdctx.Background()
	prose := api.Object{ID: "m.n1", Type: "note"}
	prose.Name = "n1"
	prose.Name = "A note"
	prose.Body = "Some **prose** here.\n"
	data := api.Object{ID: "m.n2", Type: "note"}
	data.Name = "n2"
	data.Name = "A record"
	for _, o := range []api.Object{prose, data} {
		if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}
	if _, err := os.Stat(filepath.Join(project, "modules", "m", ObjectsDirName, "n1"+api.ObjectFileSuffixMD)); err != nil {
		t.Errorf("a prose body was not written as .cwm.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, "modules", "m", ObjectsDirName, "n2"+ObjectFileSuffix)); err != nil {
		t.Errorf("a data object was not written as .cwm.yaml: %v", err)
	}
	if _, err := svc.ObjectScan(ctx, api.ObjectScanInput{Force: true}); err != nil {
		t.Fatal(err)
	}
	lst, _ := svc.ObjectList(ctx, api.ObjectListInput{Module: "m"})
	if len(lst.Objects) != 2 {
		t.Fatalf("index holds %v, want both spellings", ids(lst.Objects))
	}
	got, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "m", ID: "m.n1"})
	if err != nil {
		t.Fatal(err)
	}
	if p := got.Object.Body; !strings.Contains(p, "**prose**") {
		t.Errorf("ObjectGet did not hydrate the markdown body: %q", p)
	}
}

// MINTED IDS: a title and no id → <id_prefix>-<NNN> in the type's folder with
// a slug in the file name; a type with no prefix → a UUIDv7.
func TestCreateMintsReadableIDsAndFiles(t *testing.T) {
	svc, project := mdFixture(t)
	ctx := stdctx.Background()
	seedModule(t, svc, "m")
	mk := func(title string) api.Object {
		o := api.Object{Type: "decision"}
		o.Parent = "m"
		o.Name = title
		o.Body = "## Question\n\nwhy?\n"
		out, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: o})
		if err != nil {
			t.Fatalf("create %q: %v", title, err)
		}
		return out.Object
	}
	a := mk("Which store do we keep?")
	b := mk("Second question")
	if a.ID != "m.OD-001" || b.ID != "m.OD-002" {
		t.Errorf("minted ids = %s, %s; want m.OD-001, m.OD-002", a.ID, b.ID)
	}
	want := filepath.Join(project, "modules", "m", "decisions", "OD-001-which-store-do-we-keep"+api.ObjectFileSuffixMD)
	if _, err := os.Stat(want); err != nil {
		t.Errorf("file not at %s: %v", want, err)
	}
	r := api.Object{Type: "record"}
	r.Name = "a run"
	out, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: r})
	if err != nil {
		t.Fatal(err)
	}
	if local := strings.TrimPrefix(out.Object.ID, "m."); len(local) != 36 {
		t.Errorf("a type without id_prefix must mint a UUIDv7, got %s", out.Object.ID)
	}
}

// THE GUARD MADE REAL: a body that restates a capability datum is refused; the
// title as the first heading is the one allowed echo.
func TestBodyMayNotRepeatAField(t *testing.T) {
	svc, _ := mdFixture(t)
	ctx := stdctx.Background()
	seedModule(t, svc, "m")
	o := api.Object{ID: "m.OD-007", Type: "decision"}
	o.Parent = "m"
	o.Name = "Which store do we keep for records?"
	o.Body = "# Which store do we keep for records?\n\n## Question\n\nOne or two?\n"
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: o}); err != nil {
		t.Fatalf("the title as first heading must be allowed: %v", err)
	}
	o.Description = "records survive a restart without a second copy"
	o.ID = "m.OD-008"
	o.Name = "Q"
	o.Body = "## Want\n\nrecords survive a restart without a second copy\n"
	// The want statement folds into the object's description (object core).
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: o}); !errors.Is(err, api.ErrInvalidInput) || !strings.Contains(err.Error(), "description") {
		t.Errorf("a body restating the description: %v — must refuse naming the field", err)
	}
}
