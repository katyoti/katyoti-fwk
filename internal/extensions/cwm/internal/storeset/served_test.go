// SPDX-License-Identifier: Apache-2.0

package storeset

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/core"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/storage"
)

// servedStore opens a store on one project folder whose catalog declares a
// type with a workflow (goal), a second type of its level (aim), a capability
// to give (tagged) and an event type to raise (thing.happened), and mints its
// project object named name.
func servedStore(t *testing.T, folder, name string) *core.Service {
	t.Helper()
	ws := t.TempDir()
	if err := storage.WriteWorkspaceRaw(ws, []byte("version: \"1\"\n")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &core.CWMServiceConfig{WorkspaceRoot: ws, ProjectRoot: folder}
	cfg.CatalogTypes.Capabilities = []api.Capability{{Name: "container"}, {Name: "lifecycle"}, {Name: "tagged"}}
	cfg.CatalogTypes.Workflows = []api.Workflow{{
		Name: "approval", Initial: "idea",
		States:      []api.WorkflowState{{Name: "idea"}, {Name: "proposed"}, {Name: "dropped", Terminal: true}},
		Transitions: []api.WorkflowTransition{{Name: "propose", From: []string{"idea"}, To: "proposed"}, {Name: "drop", From: []string{"*"}, To: "dropped"}},
	}}
	cfg.CatalogTypes.Objects = []api.ObjectType{
		{Name: "goal", Capabilities: []string{"container", "lifecycle"}, Workflow: "approval"},
		{Name: "aim", Capabilities: []string{"container", "lifecycle"}, Workflow: "approval"},
	}
	cfg.CatalogTypes.Edges = []api.EdgeTypeRegistration{
		{Name: "attaches", FromCapabilities: []string{"container"}, Target: api.EdgeTargetPath, ObjectStorage: api.EdgeStorageEdges, Cardinality: "M:N"},
		{Name: "depends-on", From: "any", To: "any", Target: "object", ObjectStorage: api.EdgeStorageEdges, Cardinality: "M:N"},
	}
	cfg.CatalogTypes.EventTypes = []api.EventType{{Name: "thing.happened", Point: api.HookPoint("on_thing"), Provider: "test"}}
	svc, err := core.New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	if _, err := svc.ProjectInit(context.Background(), api.ProjectInitInput{Name: name}); err != nil {
		t.Fatalf("ProjectInit %s: %v", name, err)
	}
	return svc
}

func goal(id, description string) api.Object {
	o := api.Object{ID: id, Type: "goal"}
	o.Name = id
	o.Description = description
	return o
}

// tree is every file under a folder, folder-relative, with its content's hash.
func tree(t *testing.T, folder string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(folder, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(folder, p)
		h := sha256.Sum256(b)
		out[filepath.ToSlash(rel)] = hex.EncodeToString(h[:])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func sameTree(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// servedPair is two projects, alpha (the first store) and beta, behind one
// Served. Both hold inbox.n1; alpha alone has the module billing, beta alone
// the module work.
func servedPair(t *testing.T) (*Served, string, string) {
	t.Helper()
	ctx := context.Background()
	repo := t.TempDir()
	alphaDir, betaDir := filepath.Join(repo, "alpha"), filepath.Join(repo, "beta")
	set, err := New(servedStore(t, alphaDir, "alpha"), servedStore(t, betaDir, "beta"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	served, err := NewServed(set)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ project, module, id, desc string }{
		{"alpha", "inbox", "inbox.n1", "alpha's"},
		{"alpha", "billing", "billing.a1", ""},
		{"beta", "inbox", "inbox.n1", "beta's"},
		{"beta", "work", "work.b1", ""},
		{"beta", "work", "work.b2", ""},
		{"beta", "work", "work.b3", ""},
	} {
		if _, _, err := set.ObjectCreate(ctx, c.project, api.ObjectCreateInput{Module: c.module, Object: goal(c.id, c.desc)}); err != nil {
			t.Fatalf("create %s in %s: %v", c.id, c.project, err)
		}
	}
	return served, alphaDir, betaDir
}

func refusedNaming(err error, names ...string) bool {
	if !errors.Is(err, api.ErrInvalidInput) {
		return false
	}
	for _, n := range names {
		if !strings.Contains(err.Error(), n) {
			return false
		}
	}
	return true
}

// TestEveryVerbReachesTheStoreHoldingTheObject:
// behind storeset.Served, alpha is the first store and beta the second. Every
// verb keyed by an object id, run on an object of beta - written bare when
// beta alone holds it, qualified beta:<id> when both projects hold the id -
// writes in beta only (alpha's files unchanged) and reads it back there; a
// module verb goes to the store holding the module; the joined verbs answer
// for both stores; a move to another project's module and the one-project
// verbs are refused naming the projects.
func TestEveryVerbReachesTheStoreHoldingTheObject(t *testing.T) {
	ctx := context.Background()
	served, alphaDir, betaDir := servedPair(t)
	alphaBefore := tree(t, alphaDir)
	must := func(what string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
	}

	// Verbs of one object: bare (only beta holds work.b1) and qualified (both
	// hold inbox.n1).
	for _, id := range []string{"work.b1", "beta:inbox.n1"} {
		_, err := served.ObjectEffectiveCapabilities(ctx, api.ObjectEffectiveCapabilitiesInput{ID: id})
		must(id+": effective capabilities", err)
		_, err = served.ObjectEffectiveSurface(ctx, api.ObjectEffectiveSurfaceInput{ID: id})
		must(id+": effective surface", err)
		_, err = served.ObjectAllowedChildren(ctx, api.ObjectAllowedChildrenInput{ParentID: id})
		must(id+": allowed children", err)
		_, err = served.ObjectHooks(ctx, api.ObjectHooksInput{ID: id})
		must(id+": hooks", err)
		ev, err := served.ObjectEvaluate(ctx, api.ObjectEvaluateInput{ID: id, Expressions: []string{"current.description"}, NoFunctions: true})
		must(id+": evaluate", err)
		if id == "beta:inbox.n1" && (len(ev.Results) != 1 || ev.Results[0].Value != "beta's") {
			t.Fatalf("evaluate description of beta:inbox.n1: %+v - want beta's", ev.Results)
		}
		_, err = served.ObjectTransition(ctx, api.ObjectTransitionInput{ID: id, To: "proposed", By: api.Actor{ID: "p", Kind: api.ActorPerson}})
		must(id+": transition", err)
		_, err = served.ObjectAddCapability(ctx, api.ObjectAddCapabilityInput{ID: id, Capability: "tagged"})
		must(id+": add capability", err)
		_, err = served.ObjectRemoveCapability(ctx, api.ObjectRemoveCapabilityInput{ID: id, Capability: "tagged"})
		must(id+": remove capability", err)

		_, err = served.ObjectFileWrite(ctx, api.ObjectFileWriteInput{ID: id, Path: "notes/x.txt", Content: []byte("hello " + id)})
		must(id+": file write", err)
		rd, err := served.ObjectFileRead(ctx, api.ObjectFileReadInput{ID: id, Path: "notes/x.txt"})
		must(id+": file read", err)
		if string(rd.Content) != "hello "+id {
			t.Fatalf("%s: file read %q", id, rd.Content)
		}
		ls, err := served.ObjectFileList(ctx, api.ObjectFileListInput{ID: id, Recursive: true})
		must(id+": file list", err)
		if len(ls.Files) == 0 {
			t.Fatalf("%s: file list empty", id)
		}
		_, err = served.ObjectFileStat(ctx, api.ObjectFileStatInput{ID: id, Path: "notes/x.txt"})
		must(id+": file stat", err)
		_, err = served.ObjectFileAttach(ctx, api.ObjectFileAttachInput{ID: id, Patterns: []string{"notes/*.txt"}})
		must(id+": file attach", err)
		_, err = served.ObjectFileDelete(ctx, api.ObjectFileDeleteInput{ID: id, Path: "notes/x.txt"})
		must(id+": file delete", err)

		_, err = served.ObjectNamedFileDeclare(ctx, api.ObjectNamedFileDeclareInput{ID: id, Name: "spec", File: api.NamedFile{Path: "docs/${id}.md"}})
		must(id+": named file declare", err)
		_, err = served.ObjectNamedFileList(ctx, api.ObjectNamedFileListInput{ID: id})
		must(id+": named file list", err)
		_, err = served.ObjectNamedFileWrite(ctx, api.ObjectNamedFileWriteInput{ID: id, Name: "spec", Content: []byte("spec of " + id)})
		must(id+": named file write", err)
		nr, err := served.ObjectNamedFileRead(ctx, api.ObjectNamedFileReadInput{ID: id, Name: "spec"})
		must(id+": named file read", err)
		if string(nr.Content) != "spec of "+id {
			t.Fatalf("%s: named file read %q", id, nr.Content)
		}

		_, err = served.EventRaise(ctx, api.EventRaiseInput{Type: "thing.happened", ObjectID: id})
		must(id+": event raise", err)
		_, err = served.EdgeCreate(ctx, api.EdgeCreateInput{From: id, To: "beta:work.b2", Kind: "depends-on"})
		must(id+": edge create", err)
		el, err := served.EdgeList(ctx, api.EdgeListInput{ObjectID: id, Kind: "depends-on", Direction: api.DirectionOut})
		must(id+": edge list", err)
		if len(el.Edges) != 1 {
			t.Fatalf("%s: edges %+v - want the one created", id, el.Edges)
		}
		_, err = served.EdgeDelete(ctx, api.EdgeDeleteInput{From: id, To: "work.b2", Kind: "depends-on"})
		must(id+": edge delete", err)
		_, err = served.ObjectRunHooks(ctx, api.ObjectRunHooksInput{Point: api.HookLint, ID: id})
		must(id+": run hooks", err)
		_, err = served.ObjectRetype(ctx, api.ObjectRetypeInput{ID: id, Type: "aim"})
		must(id+": retype", err)
	}
	// What the verbs wrote is in beta, read back there.
	betaStore, _ := served.Set().Store("beta")
	n1, err := betaStore.ObjectGet(ctx, api.ObjectGetInput{ID: "inbox.n1"})
	must("beta's inbox.n1", err)
	if n1.Object.Type != "aim" {
		t.Fatalf("beta's inbox.n1 type %q - want aim (retyped through Served)", n1.Object.Type)
	}
	if _, err := os.Stat(filepath.Join(betaDir, "modules", "work", "docs", "work.b1.md")); err != nil {
		t.Fatalf("named file of work.b1 not in beta: %v", err)
	}

	// An edge's target in another project is refused.
	if _, err := served.EdgeCreate(ctx, api.EdgeCreateInput{From: "work.b1", To: "alpha:billing.a1", Kind: "depends-on"}); !refusedNaming(err, "alpha", "beta") {
		t.Fatalf("edge to another project: %v - want refused naming alpha and beta", err)
	}
	// A move stays in its project.
	mv, err := served.ObjectMove(ctx, api.ObjectMoveInput{ID: "work.b2", NewLocalID: "b2r", Reason: "rename"})
	must("move within beta", err)
	if mv.ID != "work.b2r" {
		t.Fatalf("move: %+v", mv)
	}
	if _, err := served.ObjectMove(ctx, api.ObjectMoveInput{ID: "work.b2r", NewModule: "billing", Reason: "x"}); !refusedNaming(err, "alpha", "beta") {
		t.Fatalf("move into alpha's module: %v - want refused naming alpha and beta", err)
	}
	if _, err := served.ObjectMove(ctx, api.ObjectMoveInput{ID: "work.b2r", NewModule: "alpha:billing", Reason: "x"}); !refusedNaming(err, "alpha", "beta") {
		t.Fatalf("move into alpha:billing: %v - want refused naming alpha and beta", err)
	}
	sp, err := served.ObjectSplit(ctx, api.ObjectSplitInput{ID: "beta:work.b3", Reason: "two parts", Successors: []api.ObjectSplitTarget{{Type: "goal", Title: "part one"}, {Type: "goal", Title: "part two"}}})
	must("split", err)
	if len(sp.Successors) != 2 {
		t.Fatalf("split: %+v", sp)
	}
	_, err = served.ObjectArchive(ctx, api.ObjectArchiveInput{ID: "work.b1"})
	must("archive", err)
	if ok, _ := betaStore.Holds("work.b1"); ok {
		t.Fatal("work.b1 still in beta's index after its archive")
	}

	// Module verbs: the store holding the module.
	_, err = served.ObjectErrorList(ctx, api.ObjectErrorListInput{Module: "work"})
	must("error list of work", err)
	_, err = served.ModuleSubscribers(ctx, api.ModuleSubscribersInput{Module: "work"})
	must("module subscribers of work", err)
	if _, err := served.ModuleSubscribers(ctx, api.ModuleSubscribersInput{Module: "inbox"}); !refusedNaming(err, "alpha", "beta") {
		t.Fatalf("module subscribers of inbox (both hold it): %v - want refused naming alpha and beta", err)
	}
	_, err = served.ModuleSubscribers(ctx, api.ModuleSubscribersInput{Module: "beta:inbox"})
	must("module subscribers of beta:inbox", err)
	rh, err := served.ObjectRunHooks(ctx, api.ObjectRunHooksInput{Point: api.HookLint, Module: "work"})
	must("run hooks over work", err)
	if rh.Point != api.HookLint {
		t.Fatalf("run hooks over work: %+v", rh)
	}

	// Joined verbs: both stores answer.
	scan, err := served.ObjectScan(ctx, api.ObjectScanInput{})
	must("scan", err)
	alphaStore, _ := served.Set().Store("alpha")
	alphaList, _ := alphaStore.ObjectList(ctx, api.ObjectListInput{Limit: -1})
	betaList, _ := betaStore.ObjectList(ctx, api.ObjectListInput{Limit: -1})
	if scan.Total != len(alphaList.Objects)+len(betaList.Objects) {
		t.Fatalf("scan total %d - want alpha's %d + beta's %d", scan.Total, len(alphaList.Objects), len(betaList.Objects))
	}
	tr, err := served.ObjectTree(ctx, api.ObjectTreeInput{})
	must("tree", err)
	roots := map[string]bool{}
	for _, r := range tr.Roots {
		roots[r.Object.ID] = true
	}
	if !roots["alpha"] || !roots["beta"] {
		t.Fatalf("tree roots %v - want both projects' roots", roots)
	}
	sub, err := served.ObjectTree(ctx, api.ObjectTreeInput{Root: "beta:inbox.n1"})
	must("tree of beta:inbox.n1", err)
	if len(sub.Roots) != 1 || sub.Roots[0].Object.Description != "beta's" {
		t.Fatalf("tree of beta:inbox.n1: %+v", sub.Roots)
	}
	_, err = served.EventSubscriptions(ctx, api.EventSubscriptionsInput{})
	must("event subscriptions", err)
	known, err := served.ProjectsKnown(ctx, api.ProjectsKnownInput{})
	must("projects known", err)
	var servedIDs []string
	for _, p := range known.Projects {
		if p.Served {
			servedIDs = append(servedIDs, p.ID)
		}
	}
	if strings.Join(servedIDs, " ") != "alpha beta" {
		t.Fatalf("projects known served %v - want alpha beta", servedIDs)
	}
	mg, err := served.Migrate(ctx, api.MigrateInput{DryRun: true})
	must("migrate dry run", err)
	if mg.From != mg.To {
		t.Fatalf("migrate dry run: %+v - want nothing waiting on fresh projects", mg)
	}
	if _, err := served.MigrateCommit(ctx, api.MigrateCommitInput{}); !refusedNaming(err, "alpha", "beta") {
		t.Fatalf("migrate commit with nothing waiting: %v - want refused naming alpha and beta", err)
	}
	_, err = served.OutboxList(ctx, api.OutboxListInput{})
	must("outbox list", err)
	_, err = served.OutboxDrop(ctx, api.OutboxDropInput{Reaction: "s@e1"})
	must("outbox drop", err)
	_, err = served.ChangeSetBegin(ctx, api.ChangeSetBeginInput{Reaction: "s@e1"})
	must("change set begin", err)
	cs, err := served.ChangeSetList(ctx, api.ChangeSetListInput{})
	must("change set list", err)
	if len(cs.Sets) != 1 {
		t.Fatalf("change sets %+v - want the reaction's once", cs.Sets)
	}
	_, err = served.ChangeSetDiscard(ctx, api.ChangeSetDiscardInput{Reaction: "s@e1"})
	must("change set discard", err)
	_, err = served.ChangeSetCommit(ctx, api.ChangeSetCommitInput{Reaction: "s@e2"})
	must("change set commit", err)
	_, err = served.OutboxRelease(ctx, api.OutboxReleaseInput{Reaction: "s@e2"})
	must("outbox release", err)
	if _, err := served.CatchUpRelease(ctx, api.CatchUpReleaseInput{EventID: "none"}); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("catch-up release of no held event: %v - want not found", err)
	}
	ch, cancel, err := served.EventsSubscribe(ctx, api.EventsSubscribeInput{TopicPattern: "cwm.>"})
	must("events subscribe", err)
	cancel()
	for range ch { // closes once every store's has
	}

	// One namespace: the first store answers.
	_, err = served.ObjectTypeGet(ctx, api.ObjectTypeGetInput{Name: "goal"})
	must("type get", err)

	// One project's own verbs are refused with several.
	for name, err := range map[string]error{
		"project init": second(served.ProjectInit(ctx, api.ProjectInitInput{Name: "gamma"})),
	} {
		if !refusedNaming(err, "alpha", "beta") {
			t.Fatalf("%s with two projects: %v - want refused naming alpha and beta", name, err)
		}
	}

	// Nothing reached alpha.
	if after := tree(t, alphaDir); !sameTree(alphaBefore, after) {
		t.Fatalf("alpha's files changed:\nbefore %v\nafter  %v", alphaBefore, after)
	}
	a1, err := alphaStore.ObjectGet(ctx, api.ObjectGetInput{ID: "inbox.n1"})
	must("alpha's inbox.n1", err)
	if a1.Object.Type != "goal" || a1.Object.Description != "alpha's" {
		t.Fatalf("alpha's inbox.n1 %+v - want untouched", a1.Object)
	}
}

func second[O any](_ O, err error) error { return err }

// TestServedKeepsEachObjectsProject (points, folded into
// ): ObjectGet with a qualified module routes to that key and a
// malformed module is refused; ObjectSearch keys its states by the qualified
// id, so two projects holding one bare id keep each its own; ObjectUpdate and
// ObjectDelete through Served act on the store holding the id only. ObjectGet
// names the project holding the object for a bare, a qualified
// and a module-qualified id; a store answering alone names none.
func TestServedKeepsEachObjectsProject(t *testing.T) {
	ctx := context.Background()
	served, _, _ := servedPair(t)

	got, err := served.ObjectGet(ctx, api.ObjectGetInput{Module: "beta:inbox", ID: "inbox.n1"})
	if err != nil || got.Object.Description != "beta's" || got.Project != "beta" {
		t.Fatalf("get inbox.n1 in module beta:inbox: %q project %q %v - want beta's in beta", got.Object.Description, got.Project, err)
	}
	for id, want := range map[string]string{"billing.a1": "alpha", "work.b1": "beta", "alpha:inbox.n1": "alpha", "beta:inbox.n1": "beta"} {
		got, err := served.ObjectGet(ctx, api.ObjectGetInput{ID: id})
		if err != nil || got.Project != want {
			t.Fatalf("get %s: project %q %v - want %q", id, got.Project, err, want)
		}
	}
	alone, ok := served.Set().Store("alpha")
	if !ok {
		t.Fatal("no store alpha")
	}
	if got, err := alone.ObjectGet(ctx, api.ObjectGetInput{ID: "billing.a1"}); err != nil || got.Project != "" {
		t.Fatalf("get billing.a1 from its store alone: project %q %v - want none", got.Project, err)
	}
	if _, err := served.ObjectGet(ctx, api.ObjectGetInput{Module: "beta:", ID: "inbox.n1"}); !errors.Is(err, api.ErrInvalidInput) {
		t.Fatalf("get with a malformed module: %v - want refused", err)
	}
	if _, err := served.ObjectGet(ctx, api.ObjectGetInput{Module: "alpha:inbox", ID: "beta:inbox.n1"}); !errors.Is(err, api.ErrInvalidInput) {
		t.Fatalf("get with id and module of two projects: %v - want refused", err)
	}

	if _, err := served.ObjectTransition(ctx, api.ObjectTransitionInput{ID: "beta:inbox.n1", To: "proposed", By: api.Actor{ID: "p", Kind: api.ActorPerson}}); err != nil {
		t.Fatal(err)
	}
	found, err := served.ObjectSearch(ctx, api.ObjectSearchInput{Query: "n1", WithStates: true})
	if err != nil {
		t.Fatal(err)
	}
	if a, b := found.States["alpha:inbox.n1"].State, found.States["beta:inbox.n1"].State; a != "idea" || b != "proposed" {
		t.Fatalf("search states %+v - want alpha:inbox.n1 idea and beta:inbox.n1 proposed", found.States)
	}

	got, err = served.ObjectGet(ctx, api.ObjectGetInput{ID: "beta:inbox.n1"})
	if err != nil {
		t.Fatal(err)
	}
	upd := got.Object
	upd.ID = "beta:inbox.n1"
	upd.Description = "beta's, updated"
	if _, err := served.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "inbox", Object: upd}); err != nil {
		t.Fatalf("update beta:inbox.n1: %v", err)
	}
	alpha, _ := served.Set().Store("alpha")
	beta, _ := served.Set().Store("beta")
	if a, _ := alpha.ObjectGet(ctx, api.ObjectGetInput{ID: "inbox.n1"}); a.Object.Description != "alpha's" {
		t.Fatalf("alpha's inbox.n1 after beta's update: %q", a.Object.Description)
	}
	if b, _ := beta.ObjectGet(ctx, api.ObjectGetInput{ID: "inbox.n1"}); b.Object.Description != "beta's, updated" {
		t.Fatalf("beta's inbox.n1 after its update: %q", b.Object.Description)
	}
	if _, err := served.ObjectDelete(ctx, api.ObjectDeleteInput{Module: "inbox", ID: "beta:inbox.n1"}); err != nil {
		t.Fatalf("delete beta:inbox.n1: %v", err)
	}
	if ok, _ := beta.Holds("inbox.n1"); ok {
		t.Fatal("beta still holds inbox.n1 after its delete")
	}
	if ok, _ := alpha.Holds("inbox.n1"); !ok {
		t.Fatal("alpha lost inbox.n1 to beta's delete")
	}
}

// TestServedUpdateAndDeleteTakeTheKeyOffTheModule: ObjectUpdate
// and ObjectDelete read a module written <key>:<module> as ObjectGet does - the
// module beta:billing beside the id beta: acts on beta's store
// the module alpha:billing beside the same id is refused naming both projects,
// and a module keyed alone names the store.
func TestServedUpdateAndDeleteTakeTheKeyOffTheModule(t *testing.T) {
	ctx := context.Background()
	served, _, _ := servedPair(t)
	for _, id := range []string{"billing.RQ-001", "billing.RQ-002"} {
		if _, _, err := served.Set().ObjectCreate(ctx, "beta", api.ObjectCreateInput{Module: "billing", Object: goal(id, "beta's")}); err != nil {
			t.Fatalf("create %s in beta: %v", id, err)
		}
	}
	beta, _ := served.Set().Store("beta")
	update := func(module, id, desc string) error {
		bare := id[strings.Index(id, ":")+1:]
		got, err := beta.ObjectGet(ctx, api.ObjectGetInput{ID: bare})
		if err != nil {
			t.Fatal(err)
		}
		upd := got.Object
		upd.ID = id
		upd.Description = desc
		_, err = served.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: module, Object: upd})
		return err
	}

	if err := update("alpha:billing", "beta:billing.RQ-001", "wrong"); !refusedNaming(err, "alpha", "beta") {
		t.Fatalf("update with module alpha:billing and id beta:billing.RQ-001: %v - want refused naming alpha and beta", err)
	}
	if err := update("beta:billing", "beta:billing.RQ-001", "updated"); err != nil {
		t.Fatalf("update with module beta:billing and id beta:billing.RQ-001: %v", err)
	}
	if b, _ := beta.ObjectGet(ctx, api.ObjectGetInput{ID: "billing.RQ-001"}); b.Object.Description != "updated" {
		t.Fatalf("beta's billing.RQ-001 after its update: %q", b.Object.Description)
	}
	if err := update("beta:billing", "billing.RQ-002", "keyed by its module"); err != nil {
		t.Fatalf("update with module beta:billing and bare id billing.RQ-002: %v", err)
	}
	if b, _ := beta.ObjectGet(ctx, api.ObjectGetInput{ID: "billing.RQ-002"}); b.Object.Description != "keyed by its module" {
		t.Fatalf("beta's billing.RQ-002 after its update: %q", b.Object.Description)
	}

	if _, err := served.ObjectDelete(ctx, api.ObjectDeleteInput{Module: "alpha:billing", ID: "beta:billing.RQ-001"}); !refusedNaming(err, "alpha", "beta") {
		t.Fatalf("delete with module alpha:billing and id beta:billing.RQ-001: %v - want refused naming alpha and beta", err)
	}
	if ok, _ := beta.Holds("billing.RQ-001"); !ok {
		t.Fatal("beta lost billing.RQ-001 to a refused delete")
	}
	if _, err := served.ObjectDelete(ctx, api.ObjectDeleteInput{Module: "beta:billing", ID: "beta:billing.RQ-001"}); err != nil {
		t.Fatalf("delete with module beta:billing and id beta:billing.RQ-001: %v", err)
	}
	if ok, _ := beta.Holds("billing.RQ-001"); ok {
		t.Fatal("beta still holds billing.RQ-001 after its delete")
	}
	alpha, _ := served.Set().Store("alpha")
	if ok, _ := alpha.Holds("billing.a1"); !ok {
		t.Fatal("alpha lost billing.a1")
	}
}
