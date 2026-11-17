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

// AN OBJECT'S FILES ARE ITS MODULE'S (decided 2026-09-25).
//
// What these prove is the self-contained rule: the bytes land under
// `modules/<module>/`, committed with the module, not in the recreatable store
// the node verbs wrote to — and that the object store's own files cannot be
// reached through a byte-level API.

func fileFixture(t *testing.T) (*Service, string) {
	t.Helper()
	root := freshWorkspace(t)
	project := t.TempDir()
	// ERROR mode, so the edge checks REFUSE rather than warn: the attach test
	// asserts a refusal, and under the default warn mode it would assert
	// nothing at all.
	cfg := &CWMServiceConfig{WorkspaceRoot: root, ProjectRoot: project,
		ObjectTypeValidation: api.TypeValidationError}
	cfg.CatalogTypes.Objects = []api.ObjectType{
		{Name: "doc", Capabilities: []string{"container", "content"}},
	}
	cfg.CatalogTypes.Capabilities = []api.Capability{{Name: "container"}, {Name: "content"}}
	// DECLARED, so edge validation actually runs: without a declared kind the
	// attach test below would pass vacuously, proving nothing.
	cfg.CatalogTypes.Edges = []api.EdgeTypeRegistration{{
		Name: "attaches", FromCapabilities: []string{"container"},
		Target: api.EdgeTargetPath, ObjectStorage: api.EdgeStorageEdges, Cardinality: "M:N",
	}, {
		Name: "links", FromCapabilities: []string{"container"},
		Target: api.EdgeTargetURI, ObjectStorage: api.EdgeStorageEdges, Cardinality: "M:N",
	}}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })

	o := api.Object{ID: "billing.spec", Type: "doc"}
	o.Name = "Spec"
	o.Name = "Spec"
	if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "billing", Object: o}); err != nil {
		t.Fatalf("ObjectCreate: %v", err)
	}
	return svc, project
}

// TestObjectFilesLiveInTheModule — the bytes land under the module folder in
// the PROJECT, so a clone has them, and the returned project path is what an
// `attaches` edge stores.
func TestObjectFilesLiveInTheModule(t *testing.T) {
	svc, project := fileFixture(t)
	ctx := stdctx.Background()

	out, err := svc.ObjectFileWrite(ctx, api.ObjectFileWriteInput{
		ID: "billing.spec", Path: "diagrams/flow.svg", Content: []byte("<svg/>"),
	})
	if err != nil {
		t.Fatalf("ObjectFileWrite: %v", err)
	}
	want := filepath.Join(project, "modules", "billing", "diagrams", "flow.svg")
	if _, serr := os.Stat(want); serr != nil {
		t.Fatalf("the file is not in the module: %v", serr)
	}
	// NOT in the store: that is the whole point. A module whose attachments
	// live in one person's `.katyoti` is the opposite of self-contained.
	if strings.Contains(want, ".katyoti") {
		t.Fatal("the module folder resolved into the store")
	}
	if out.ProjectPath != "modules/billing/diagrams/flow.svg" {
		t.Errorf("project path %q — an attaches edge stores exactly this", out.ProjectPath)
	}
	if out.Path != "diagrams/flow.svg" || out.SizeBytes != 6 {
		t.Errorf("write result: %+v", out)
	}

	got, err := svc.ObjectFileRead(ctx, api.ObjectFileReadInput{ID: "billing.spec", Path: "diagrams/flow.svg"})
	if err != nil {
		t.Fatalf("ObjectFileRead: %v", err)
	}
	if string(got.Content) != "<svg/>" {
		t.Errorf("read back %q", string(got.Content))
	}
}

// TestObjectFilePathIsAttachableAsAnEdge — the point of returning the project
// path: the edge kind validates that the file EXISTS, so a write followed by
// an attach must be accepted.
func TestObjectFilePathIsAttachableAsAnEdge(t *testing.T) {
	svc, _ := fileFixture(t)
	ctx := stdctx.Background()
	w, err := svc.ObjectFileWrite(ctx, api.ObjectFileWriteInput{
		ID: "billing.spec", Path: "notes.md", Content: []byte("# notes"),
	})
	if err != nil {
		t.Fatalf("ObjectFileWrite: %v", err)
	}
	got, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "billing", ID: "billing.spec"})
	if err != nil {
		t.Fatalf("ObjectGet: %v", err)
	}
	obj := got.Object
	obj.Edges = []api.ObjectEdge{{Kind: "attaches", To: w.ProjectPath}}
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "billing", Object: obj}); err != nil {
		t.Fatalf("attaching the file it just wrote was refused: %v", err)
	}
	// THE VALIDATION IS LIVE: a path that names nothing is refused, which is
	// what makes the acceptance above mean something.
	obj.Edges = []api.ObjectEdge{{Kind: "attaches", To: "modules/billing/does-not-exist.png"}}
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "billing", Object: obj}); err == nil {
		t.Error("attaching a path that is not a file must be refused")
	}
	// And the other half of the rule: what is not in git is a LINK, not a file.
	obj.Edges = []api.ObjectEdge{{Kind: "links", To: "https://sharepoint.example/spec.docx"}}
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "billing", Object: obj}); err != nil {
		t.Errorf("a link to something outside the repo must be accepted: %v", err)
	}
	obj.Edges = []api.ObjectEdge{{Kind: "links", To: "not-a-uri"}}
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "billing", Object: obj}); err == nil {
		t.Error("a links edge that is not a URI must be refused")
	}
}

// TestObjectFileRefusesAnObjectFile — `.cwm.yaml` and `.cwm.md` belong to the
// object verbs, which check types, capabilities and edges. A byte write past
// them is a second write path with none of the checks.
func TestObjectFileRefusesAnObjectFile(t *testing.T) {
	svc, _ := fileFixture(t)
	ctx := stdctx.Background()
	for _, p := range []string{"objects/spec.cwm.yaml", "notes.cwm.md"} {
		if _, err := svc.ObjectFileWrite(ctx, api.ObjectFileWriteInput{
			ID: "billing.spec", Path: p, Content: []byte("id: sneaky"),
		}); err == nil {
			t.Errorf("writing %q through the file API must be refused", p)
		}
	}
	// And they are not listed either, so nothing invites the attempt.
	if _, err := svc.ObjectFileWrite(ctx, api.ObjectFileWriteInput{
		ID: "billing.spec", Path: "readme.md", Content: []byte("ok"),
	}); err != nil {
		t.Fatalf("a normal file must still write: %v", err)
	}
	list, err := svc.ObjectFileList(ctx, api.ObjectFileListInput{ID: "billing.spec", Recursive: true})
	if err != nil {
		t.Fatalf("ObjectFileList: %v", err)
	}
	for _, f := range list.Files {
		if strings.HasSuffix(f.Path, ".cwm.yaml") || strings.HasSuffix(f.Path, ".cwm.md") {
			t.Errorf("an object file was listed as an attachment: %s", f.Path)
		}
	}
	if len(list.Files) != 1 || list.Files[0].Path != "readme.md" {
		t.Errorf("listing: %+v", list.Files)
	}
	if list.Module != "billing" {
		t.Errorf("the listing must say whose files these are: %q", list.Module)
	}
}

// TestObjectFileStaysInsideTheModule — a path must not climb out into another
// module or the project, which is the containment the self-contained rule
// depends on.
func TestObjectFileStaysInsideTheModule(t *testing.T) {
	svc, _ := fileFixture(t)
	ctx := stdctx.Background()
	for _, p := range []string{"../other/steal.txt", "a/../../escape.txt"} {
		if _, err := svc.ObjectFileWrite(ctx, api.ObjectFileWriteInput{
			ID: "billing.spec", Path: p, Content: []byte("x"),
		}); err == nil {
			t.Errorf("%q escaped the module folder", p)
		}
	}
}

// TestObjectFileScopeComesFromTheID — the module is derived from the object's
// id, the one place membership is stated, so a caller cannot name a module its
// object does not belong to.
func TestObjectFileScopeComesFromTheID(t *testing.T) {
	svc, project := fileFixture(t)
	ctx := stdctx.Background()
	other := api.Object{ID: "shipping.notes", Type: "doc"}
	other.Name = "Notes"
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "shipping", Object: other}); err != nil {
		t.Fatalf("ObjectCreate: %v", err)
	}
	if _, err := svc.ObjectFileWrite(ctx, api.ObjectFileWriteInput{
		ID: "shipping.notes", Path: "x.txt", Content: []byte("x"),
	}); err != nil {
		t.Fatalf("ObjectFileWrite: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, "modules", "shipping", "x.txt")); err != nil {
		t.Errorf("the second object's file did not land in ITS module: %v", err)
	}
	// The first object's module does not see it.
	list, _ := svc.ObjectFileList(ctx, api.ObjectFileListInput{ID: "billing.spec", Recursive: true})
	for _, f := range list.Files {
		if f.Path == "x.txt" {
			t.Error("a module listed another module's file")
		}
	}
}

// TestObjectFileDeleteIsIdempotent — removing what is not there is the state
// asked for, not an error.
func TestObjectFileDeleteIsIdempotent(t *testing.T) {
	svc, _ := fileFixture(t)
	ctx := stdctx.Background()
	if _, err := svc.ObjectFileWrite(ctx, api.ObjectFileWriteInput{
		ID: "billing.spec", Path: "tmp.txt", Content: []byte("x"),
	}); err != nil {
		t.Fatalf("write: %v", err)
	}
	first, err := svc.ObjectFileDelete(ctx, api.ObjectFileDeleteInput{ID: "billing.spec", Path: "tmp.txt"})
	if err != nil || len(first.DeletedPaths) != 1 {
		t.Fatalf("first delete: %+v %v", first, err)
	}
	second, err := svc.ObjectFileDelete(ctx, api.ObjectFileDeleteInput{ID: "billing.spec", Path: "tmp.txt"})
	if err != nil {
		t.Errorf("deleting an absent file must be a no-op success: %v", err)
	}
	if len(second.DeletedPaths) != 0 {
		t.Errorf("the second delete claimed to remove %v", second.DeletedPaths)
	}
}

// TestProjectObjectFilesLiveInItsDotKtw — the project object belongs to no
// module, so its files live in its own committed space at the project root,
// `<project>/.ktw/` (identity-and-index.md §7), and a scan does not read them
// as objects. It used to refuse ("belongs to no module"), which broke the
// Comments section of the project in the GUI.
func TestProjectObjectFilesLiveInItsDotKtw(t *testing.T) {
	svc, project := fileFixture(t)
	ctx := stdctx.Background()
	// THE OBJECT MUST EXIST, or a refusal would come from "not found" and
	// never reach the module check.
	proj, err := svc.ProjectInit(ctx, api.ProjectInitInput{Name: "acme", Title: "Acme"})
	if err != nil {
		t.Fatalf("ProjectInit: %v", err)
	}
	before, err := svc.ObjectScan(ctx, api.ObjectScanInput{})
	if err != nil {
		t.Fatalf("ObjectScan: %v", err)
	}
	out, err := svc.ObjectFileWrite(ctx, api.ObjectFileWriteInput{
		ID: proj.Object.ID, Path: "notes/x.txt", Content: []byte("x"),
	})
	if err != nil {
		t.Fatalf("the project object must get a file root: %v", err)
	}
	if out.ProjectPath != ".ktw/notes/x.txt" {
		t.Errorf("project path %q, want .ktw/notes/x.txt", out.ProjectPath)
	}
	if _, serr := os.Stat(filepath.Join(project, ".ktw", "notes", "x.txt")); serr != nil {
		t.Errorf("not written into <project>/.ktw: %v", serr)
	}
	got, err := svc.ObjectFileRead(ctx, api.ObjectFileReadInput{ID: proj.Object.ID, Path: "notes/x.txt"})
	if err != nil || string(got.Content) != "x" {
		t.Errorf("read back %q, %v", got.Content, err)
	}
	ls, err := svc.ObjectFileList(ctx, api.ObjectFileListInput{ID: proj.Object.ID, Recursive: true})
	if err != nil || len(ls.Files) != 1 || ls.Files[0].Path != "notes/x.txt" {
		t.Errorf("list = %+v, %v", ls.Files, err)
	}
	// An object file written there by hand is NOT an object: the scanner
	// never walks the project's `.ktw`.
	stray := filepath.Join(project, ".ktw", "stray.cwm.yaml")
	if err := os.WriteFile(stray, []byte("id: stray\ntype: doc\nname: stray\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sc, err := svc.ObjectScan(ctx, api.ObjectScanInput{})
	if err != nil {
		t.Fatalf("ObjectScan: %v", err)
	}
	if len(sc.Added) != 0 || sc.Total != before.Total {
		t.Errorf("the project's .ktw was scanned for objects: added %v, total %d", sc.Added, sc.Total)
	}
	if _, err := svc.ObjectFileDelete(ctx, api.ObjectFileDeleteInput{ID: proj.Object.ID, Path: "notes/x.txt"}); err != nil {
		t.Errorf("delete: %v", err)
	}
}

// TestModuleObjectOwnsItsOwnFolder — a MODULE object's id is BARE (`billing`,
// not `billing.billing`), so deriving the module by splitting the id on the dot
// denies a module its own folder. Caught by the first real run.
func TestModuleObjectOwnsItsOwnFolder(t *testing.T) {
	svc, project := fileFixture(t)
	ctx := stdctx.Background()
	mod := api.Object{ID: "billing", Type: "doc"}
	mod.Name = "billing"
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: mod}); err != nil {
		t.Fatalf("ObjectCreate: %v", err)
	}
	out, err := svc.ObjectFileWrite(ctx, api.ObjectFileWriteInput{
		ID: "billing", Path: "charter.md", Content: []byte("# billing"),
	})
	if err != nil {
		t.Fatalf("a module object must own its own folder: %v", err)
	}
	if out.ProjectPath != "modules/billing/charter.md" {
		t.Errorf("project path %q", out.ProjectPath)
	}
	if _, serr := os.Stat(filepath.Join(project, "modules", "billing", "charter.md")); serr != nil {
		t.Errorf("not written into the module: %v", serr)
	}
}
