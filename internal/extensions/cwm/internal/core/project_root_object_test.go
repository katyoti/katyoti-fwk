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

// PROJECTS AND MODULES ARE THE HIGH-LEVEL OBJECTS (phase 7-first,
// identity-and-index.md §7): the project object lives at the project root and
// says where the modules are; the tree's root is the project; what runs
// produce lives in the CWM store and hangs under the project too.

func projectFixture(t *testing.T, types []api.ObjectType) (*Service, string) {
	t.Helper()
	root := freshWorkspace(t)
	project := t.TempDir()
	cfg := &CWMServiceConfig{WorkspaceRoot: root, ProjectRoot: project}
	cfg.CatalogTypes.Objects = types
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	return svc, project
}

func writeModuleObject(t *testing.T, project, modulesDir, module string) {
	t.Helper()
	dir := filepath.Join(project, modulesDir, module, ObjectsDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	o := api.Object{ID: module, Type: "module"}
	o.Name = module
	if err := WriteObjectFile(filepath.Join(dir, module+ObjectFileSuffix), o); err != nil {
		t.Fatal(err)
	}
}

func TestProjectInitMintsOnceAndWritesTheRoot(t *testing.T) {
	svc, project := projectFixture(t, nil)
	ctx := stdctx.Background()
	out, err := svc.ProjectInit(ctx, api.ProjectInitInput{Name: "acme", Title: "Acme", ModulesDir: "src/modules"})
	if err != nil {
		t.Fatalf("ProjectInit: %v", err)
	}
	if out.Object.ID != "acme" || out.Object.Type != api.DefaultProjectType {
		t.Errorf("project object = (%q, %q)", out.Object.ID, out.Object.Type)
	}
	if u := out.Object.ProjectUUID(); len(u) != 36 {
		t.Errorf("uuid %q is not a UUID", u)
	}
	if _, err := os.Stat(filepath.Join(project, api.ProjectFileName)); err != nil {
		t.Errorf("the root object is not at <project>/%s: %v", api.ProjectFileName, err)
	}
	if _, err := svc.ProjectInit(ctx, api.ProjectInitInput{Name: "acme"}); !errors.Is(err, api.ErrAlreadyExists) {
		t.Errorf("second init: %v, want ErrAlreadyExists — a project id is minted ONCE", err)
	}
}

// The scanner reads the root object first and takes the modules folder from
// it; without a root object the default folder applies.
func TestModulesDirComesFromTheRootObject(t *testing.T) {
	svc, project := projectFixture(t, nil)
	ctx := stdctx.Background()
	if _, err := svc.ProjectInit(ctx, api.ProjectInitInput{Name: "acme", ModulesDir: "src/modules"}); err != nil {
		t.Fatal(err)
	}
	writeModuleObject(t, project, "src/modules", "billing")
	writeModuleObject(t, project, "modules", "decoy") // the default folder — NOT this project's
	if _, err := svc.ObjectScan(ctx, api.ObjectScanInput{Force: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "billing", ID: "billing"}); err != nil {
		t.Errorf("a module under the declared modules_dir is not indexed: %v", err)
	}
	if _, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "decoy", ID: "decoy"}); err == nil {
		t.Errorf("a module under the DEFAULT folder was indexed although the project declares src/modules")
	}
	// And a create lands in the declared folder.
	o := api.Object{ID: "billing.g1", Type: "goal"}
	o.Name = "g1"
	o.Parent = "billing"
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: o}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(project, "src", "modules", "billing", ObjectsDirName, "g1"+ObjectFileSuffix)); err != nil {
		t.Errorf("ObjectCreate did not file under the declared modules_dir: %v", err)
	}
}

// THE PROJECT IS THE ROOT: everything parentless hangs under it.
func TestTreeRootIsTheProject(t *testing.T) {
	svc, project := projectFixture(t, nil)
	ctx := stdctx.Background()
	if _, err := svc.ProjectInit(ctx, api.ProjectInitInput{Name: "acme"}); err != nil {
		t.Fatal(err)
	}
	writeModuleObject(t, project, "modules", "billing")
	writeModuleObject(t, project, "modules", "shipping")
	if _, err := svc.ObjectScan(ctx, api.ObjectScanInput{Force: true}); err != nil {
		t.Fatal(err)
	}
	stray := api.Object{ID: "billing.stray", Type: "note"} // parentless, not a module
	stray.Name = "stray"
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: stray}); err != nil {
		t.Fatal(err)
	}
	tree, err := svc.ObjectTree(ctx, api.ObjectTreeInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Roots) != 1 || tree.Roots[0].Object.ID != "acme" {
		t.Fatalf("roots = %d (%v), want exactly the project", len(tree.Roots), rootIDs(tree.Roots))
	}
	got := map[string]bool{}
	for _, c := range tree.Roots[0].Children {
		got[c.Object.ID] = true
	}
	for _, want := range []string{"billing", "shipping", "billing.stray"} {
		if !got[want] {
			t.Errorf("%s is not a child of the project: %v", want, got)
		}
	}
	if tree.Total != 4 {
		t.Errorf("total = %d, want 4 (project + 3)", tree.Total)
	}
	// A module subtree is unchanged.
	sub, err := svc.ObjectTree(ctx, api.ObjectTreeInput{Root: "billing"})
	if err != nil || len(sub.Roots) != 1 || sub.Roots[0].Object.ID != "billing" {
		t.Errorf("ObjectTree{Root: billing} = %v, %v", rootIDs(sub.Roots), err)
	}
}

// THE PROJECT IS FOUND BY ITS PLACE, NOT ITS TYPE'S NAME: the
// object filed at the project root is the tree's root whatever its type - a
// software project, a project group - and its modules hang under it.
func TestTreeRootIsTheProjectWhateverItsType(t *testing.T) {
	for _, typ := range []string{"software-project", "dossier-architecture"} {
		t.Run(typ, func(t *testing.T) {
			svc, project := projectFixture(t, nil)
			ctx := stdctx.Background()
			if err := os.WriteFile(filepath.Join(project, api.ProjectFileName), []byte("id: acme\ntype: "+typ+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			writeModuleObject(t, project, "modules", "kb")
			writeModuleObject(t, project, "modules", "leanix")
			if _, err := svc.ObjectScan(ctx, api.ObjectScanInput{Force: true}); err != nil {
				t.Fatal(err)
			}
			tree, err := svc.ObjectTree(ctx, api.ObjectTreeInput{})
			if err != nil {
				t.Fatal(err)
			}
			if len(tree.Roots) != 1 || tree.Roots[0].Object.ID != "acme" || len(tree.Roots[0].Children) != 2 {
				t.Fatalf("roots = %v; want the project, its two modules under it", rootIDs(tree.Roots))
			}
		})
	}
}

// Without a root object, the old behaviour: parentless objects are the roots.
func TestTreeWithoutAProjectObjectIsFlat(t *testing.T) {
	svc, project := projectFixture(t, nil)
	writeModuleObject(t, project, "modules", "billing")
	if _, err := svc.ObjectScan(stdctx.Background(), api.ObjectScanInput{Force: true}); err != nil {
		t.Fatal(err)
	}
	tree, err := svc.ObjectTree(stdctx.Background(), api.ObjectTreeInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Roots) != 1 || tree.Roots[0].Object.ID != "billing" {
		t.Errorf("roots = %v, want [billing]", rootIDs(tree.Roots))
	}
}

// WHAT A RUN PRODUCES LIVES IN THE STORE, and is in the same tree.
func TestRuntimeTypeIsFiledInTheStoreAndIndexed(t *testing.T) {
	svc, project := projectFixture(t, []api.ObjectType{
		{Name: "approval-request", Capabilities: []string{"content"}, Runtime: true},
		{Name: "project", Capabilities: []string{"container", "content", "project"}},
	})
	ctx := stdctx.Background()
	if _, err := svc.ProjectInit(ctx, api.ProjectInitInput{Name: "acme"}); err != nil {
		t.Fatal(err)
	}
	req := api.Object{ID: "inbox.req-1", Type: "approval-request"}
	req.Name = "may I?"
	req.Name = "req-1"
	out, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "inbox", Object: req})
	if err != nil {
		t.Fatalf("ObjectCreate runtime: %v", err)
	}
	store := svc.storeObjectsRoot()
	if _, err := os.Stat(filepath.Join(store, "inbox", ObjectsDirName, "req-1"+ObjectFileSuffix)); err != nil {
		t.Errorf("runtime object not in the store %s: %v (path reported %s)", store, err, out.Path)
	}
	if _, err := os.Stat(filepath.Join(project, "modules", "inbox")); err == nil {
		t.Errorf("a runtime object was filed under the committed modules folder")
	}
	// Read, update, delete resolve through the index — not a composed path.
	got, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "inbox", ID: "inbox.req-1"})
	if err != nil {
		t.Fatalf("ObjectGet runtime: %v", err)
	}
	got.Object.Tags = []string{"approved"}
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "inbox", Object: got.Object}); err != nil {
		t.Fatalf("ObjectUpdate runtime: %v", err)
	}
	// In the tree, under the project.
	if _, err := svc.ObjectScan(ctx, api.ObjectScanInput{Force: true}); err != nil {
		t.Fatal(err)
	}
	tree, _ := svc.ObjectTree(ctx, api.ObjectTreeInput{})
	found := false
	for _, c := range tree.Roots[0].Children {
		if c.Object.ID == "inbox.req-1" {
			found = true
		}
	}
	if !found {
		t.Errorf("the runtime object is not under the project in the tree: %v", rootIDs(tree.Roots[0].Children))
	}
	if _, err := svc.ObjectDelete(ctx, api.ObjectDeleteInput{Module: "inbox", ID: "inbox.req-1"}); err != nil {
		t.Fatalf("ObjectDelete runtime: %v", err)
	}
}

func rootIDs(nodes []api.ObjectTreeNode) []string {
	var ids []string
	for _, n := range nodes {
		ids = append(ids, n.Object.ID)
	}
	return ids
}

// OBJECT-RESOLVE: a full name from a log, or a bare id, becomes a file — and a
// full name from ANOTHER project is answered with that project's id, never
// silently resolved here.
func TestObjectResolveFullNameAndBareID(t *testing.T) {
	svc, project := projectFixture(t, nil)
	ctx := stdctx.Background()
	init, err := svc.ProjectInit(ctx, api.ProjectInitInput{Name: "acme"})
	if err != nil {
		t.Fatal(err)
	}
	pid := init.Object.ProjectUUID()
	writeModuleObject(t, project, "modules", "billing")
	if _, err := svc.ObjectScan(ctx, api.ObjectScanInput{Force: true}); err != nil {
		t.Fatal(err)
	}
	bare, err := svc.ObjectResolve(ctx, api.ObjectResolveInput{Ref: "billing"})
	if err != nil {
		t.Fatalf("bare: %v", err)
	}
	if bare.FullName != api.FullName(pid, "billing") || bare.ProjectID != pid || bare.Project != "acme" || bare.Type != "module" {
		t.Errorf("bare resolve = %+v", bare)
	}
	if _, err := os.Stat(bare.Path); err != nil {
		t.Errorf("resolved path does not exist: %s", bare.Path)
	}
	// The project root is CWM's own fact, answered with every resolution.
	if wantRoot, _ := filepath.Abs(project); bare.ProjectRoot != wantRoot {
		t.Errorf("project root = %q, want %q", bare.ProjectRoot, wantRoot)
	}
	if _, err := os.Stat(filepath.Join(bare.ProjectRoot, api.ProjectFileName)); err != nil {
		t.Errorf("the project root does not hold %s: %v", api.ProjectFileName, err)
	}
	full, err := svc.ObjectResolve(ctx, api.ObjectResolveInput{Ref: bare.FullName})
	if err != nil || full.Path != bare.Path {
		t.Errorf("full name resolve: %+v %v", full, err)
	}
	other := api.FullName("0000000f-0000-7000-8000-000000000000", "billing")
	if _, err := svc.ObjectResolve(ctx, api.ObjectResolveInput{Ref: other}); !errors.Is(err, api.ErrNotFound) || !strings.Contains(err.Error(), "0000000f-0000-7000-8000-000000000000") {
		t.Errorf("another project's full name: %v — must refuse AND name the project id", err)
	}
	if _, err := svc.ObjectResolve(ctx, api.ObjectResolveInput{Ref: "billing.nope"}); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("unknown id: %v", err)
	}
}

// A project with no project object gets one — named after its folder, with
// its own uuid — and only once: a second pass, or a project that has its
// object, is left as it is.
func TestEnsureProjectGivesAProjectItsObjectOnce(t *testing.T) {
	svc, project := projectFixture(t, nil)
	ctx := stdctx.Background()
	created, err := svc.EnsureProject(ctx)
	if err != nil || !created {
		t.Fatalf("EnsureProject on a project without its object = %v, %v; want created", created, err)
	}
	got, err := svc.ObjectList(ctx, api.ObjectListInput{Capability: api.CapProject})
	if err != nil || len(got.Objects) != 1 || got.Objects[0].ID != filepath.Base(project) {
		t.Fatalf("project objects = %+v, %v; want one named %q", got.Objects, err, filepath.Base(project))
	}
	before, _ := os.ReadFile(filepath.Join(project, api.ProjectFileName))
	created, err = svc.EnsureProject(ctx)
	after, _ := os.ReadFile(filepath.Join(project, api.ProjectFileName))
	if err != nil || created || string(before) != string(after) {
		t.Fatalf("a second EnsureProject = %v, %v, file changed %v; want nothing done", created, err, string(before) != string(after))
	}
}

// A MODULE FILED UNDER THE PROJECT BY ITS PARENT hangs under it
// like a parentless one: a study whose `parent:` names the dossier
// d'architecture - its type accepts only that parent - is a child of the
// project's root, never dropped from the tree.
func TestAModuleWhoseParentIsTheProjectHangsUnderIt(t *testing.T) {
	svc, project := projectFixture(t, nil)
	ctx := stdctx.Background()
	if err := os.WriteFile(filepath.Join(project, api.ProjectFileName), []byte("id: dossier\ntype: dossier-architecture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeModuleObject(t, project, "modules", "guide")
	dir := filepath.Join(project, "modules", "etude-sab", ObjectsDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "etude-sab"+ObjectFileSuffix), []byte("id: etude-sab\ntype: module\nname: ERP SAB\nparent: dossier\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ObjectScan(ctx, api.ObjectScanInput{Force: true}); err != nil {
		t.Fatal(err)
	}
	tree, err := svc.ObjectTree(ctx, api.ObjectTreeInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Roots) != 1 || tree.Roots[0].Object.ID != "dossier" {
		t.Fatalf("roots = %v; want the project alone", rootIDs(tree.Roots))
	}
	var kids []string
	for _, c := range tree.Roots[0].Children {
		kids = append(kids, c.Object.ID)
	}
	if strings.Join(kids, ",") != "etude-sab,guide" {
		t.Errorf("the project's children = %v; want etude-sab (its parent the project) and guide (parentless), by id", kids)
	}
	if tree.Total != 3 {
		t.Errorf("total = %d, want 3 (project + 2)", tree.Total)
	}
}
