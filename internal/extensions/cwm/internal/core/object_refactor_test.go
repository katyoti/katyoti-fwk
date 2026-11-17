// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// THE STRUCTURE EDITS THAT CHANGE IDENTITY.
//
// What these prove is the part a move cannot get away with: an object id is
// module-qualified, so refiling across modules ends one identity and starts
// another — and everything that named the old one has to follow, or the move
// has not moved anything, it has broken it.

func refactorFixture(t *testing.T) *Service {
	t.Helper()
	root := freshWorkspace(t)
	cfg := &CWMServiceConfig{WorkspaceRoot: root, ProjectRoot: t.TempDir()}
	cfg.CatalogTypes.Objects = []api.ObjectType{
		{Name: "doc", Capabilities: []string{"container", "content"}},
		{Name: "decision", Capabilities: []string{"container", "content", "decision"}},
	}
	cfg.CatalogTypes.Capabilities = []api.Capability{
		{Name: "container"}, {Name: "content"}, {Name: "decision"},
	}
	cfg.CatalogTypes.Edges = []api.EdgeTypeRegistration{{
		Name: "serves", FromCapabilities: []string{"container"},
		Target: api.EdgeTargetObject, ObjectStorage: api.EdgeStorageEdges, Cardinality: "M:N",
	}}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

func mkDoc(t *testing.T, svc *Service, module, id, parent string, edges ...api.ObjectEdge) {
	t.Helper()
	o := api.Object{ID: id, Type: "doc", Edges: edges}
	o.Name = id
	o.Parent = parent
	o.Name = id
	if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: module, Object: o}); err != nil {
		t.Fatalf("create %s: %v", id, err)
	}
}

// refactorWorld: a doc in `billing` with a child and an inbound edge from
// another module.
func refactorWorld(t *testing.T) *Service {
	svc := refactorFixture(t)
	mkDoc(t, svc, "billing", "billing.spec", "")
	mkDoc(t, svc, "billing", "billing.chapter", "billing.spec")
	mkDoc(t, svc, "shipping", "shipping.plan", "", api.ObjectEdge{Kind: "serves", To: "billing.spec"})
	return svc
}

// TestMoveAcrossModulesMintsANewIdentity — the id is module-qualified, so
// this is not a move but a rename plus a relocation, and the output says so.
func TestMoveAcrossModulesMintsANewIdentity(t *testing.T) {
	svc := refactorWorld(t)
	ctx := stdctx.Background()
	out, err := svc.ObjectMove(ctx, api.ObjectMoveInput{
		ID: "billing.spec", NewModule: "shipping", Reason: "it belongs with shipping now",
	})
	if err != nil {
		t.Fatalf("ObjectMove: %v", err)
	}
	if out.ID != "shipping.spec" || out.PreviousID != "billing.spec" {
		t.Fatalf("identity: %+v", out)
	}
	if _, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "shipping", ID: "shipping.spec"}); err != nil {
		t.Errorf("the object is not at its new id: %v", err)
	}
	if _, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "billing", ID: "billing.spec"}); err == nil {
		t.Error("the old identity must be gone — two ids for one object is what this refuses")
	}
}

// TestMoveRepointsEverythingThatNamedTheOldID — a move that leaves dangling
// references has not moved anything.
func TestMoveRepointsEverythingThatNamedTheOldID(t *testing.T) {
	svc := refactorWorld(t)
	ctx := stdctx.Background()
	out, err := svc.ObjectMove(ctx, api.ObjectMoveInput{
		ID: "billing.spec", NewModule: "shipping", Reason: "regrouping",
	})
	if err != nil {
		t.Fatalf("ObjectMove: %v", err)
	}
	if len(out.Dangling) != 0 {
		t.Errorf("nothing should have been left dangling: %v", out.Dangling)
	}
	// The child follows its parent's new id.
	child, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "billing", ID: "billing.chapter"})
	if err != nil {
		t.Fatalf("ObjectGet child: %v", err)
	}
	if c := child.Object; c.Parent != "shipping.spec" {
		t.Errorf("the child still points at the old parent: %q", c.Parent)
	}
	// The inbound edge from the OTHER module follows too.
	src, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "shipping", ID: "shipping.plan"})
	if err != nil {
		t.Fatalf("ObjectGet source: %v", err)
	}
	if len(src.Object.Edges) != 1 || src.Object.Edges[0].To != "shipping.spec" {
		t.Errorf("the inbound edge was not re-pointed: %+v", src.Object.Edges)
	}
	// Both are REPORTED: a structure edit that touches other files says so.
	if !hasName(out.Rewritten, "billing.chapter") || !hasName(out.Rewritten, "shipping.plan") {
		t.Errorf("rewritten: %v, want both the child and the edge source", out.Rewritten)
	}
}

// TestMoveWithinAModuleKeepsTheIdentity — the same verb, and nothing is
// renamed: only the parent changes.
func TestMoveWithinAModuleKeepsTheIdentity(t *testing.T) {
	svc := refactorWorld(t)
	ctx := stdctx.Background()
	mkDoc(t, svc, "billing", "billing.other", "")
	out, err := svc.ObjectMove(ctx, api.ObjectMoveInput{
		ID: "billing.chapter", NewParent: "billing.other", Reason: "better home",
	})
	if err != nil {
		t.Fatalf("ObjectMove: %v", err)
	}
	if out.ID != "billing.chapter" || out.PreviousID != "" {
		t.Errorf("a same-module move must not rename: %+v", out)
	}
	got, _ := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "billing", ID: "billing.chapter"})
	if c := got.Object; c.Parent != "billing.other" {
		t.Errorf("parent: %q", c.Parent)
	}
}

// TestAMoveWritesNoDecisionObject — the journal is the trail: the move's one
// `moved` event carries the ids and the reason (object_events_test), and no
// decision object is written, even where the decision type is declared.
func TestAMoveWritesNoDecisionObject(t *testing.T) {
	svc := refactorWorld(t)
	ctx := stdctx.Background()
	if _, err := svc.ObjectMove(ctx, api.ObjectMoveInput{
		ID: "billing.spec", NewModule: "shipping", Reason: "ownership moved to the shipping team",
	}); err != nil {
		t.Fatalf("ObjectMove: %v", err)
	}
	list, err := svc.ObjectList(ctx, api.ObjectListInput{Type: "decision"})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Objects) != 0 {
		t.Fatalf("a move wrote decision objects: %v", list.Objects)
	}
}

// TestMoveRefusesWhatWouldBreakTheTree — a reason is required, and a move
// under one's own descendant detaches a subtree while every file stays
// readable, which is the worst kind of breakage.
func TestMoveRefusesWhatWouldBreakTheTree(t *testing.T) {
	svc := refactorWorld(t)
	ctx := stdctx.Background()

	if _, err := svc.ObjectMove(ctx, api.ObjectMoveInput{ID: "billing.spec", NewModule: "shipping"}); err == nil {
		t.Error("a trail with no reason records only that someone did something")
	}
	_, err := svc.ObjectMove(ctx, api.ObjectMoveInput{
		ID: "billing.spec", NewParent: "billing.chapter", Reason: "x",
	})
	if !errors.Is(err, api.ErrCircularReference) {
		t.Errorf("moving under one's own descendant must be refused: %v", err)
	}
	// And nothing was written on the way to the refusal.
	got, _ := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "billing", ID: "billing.spec"})
	if c := got.Object; c.Parent != "" {
		t.Errorf("the refused move still changed the parent: %q", c.Parent)
	}
}

// TestMoveRefusesAnIdAlreadyTaken — two objects with one id is the state the
// whole identity rule exists to prevent.
func TestMoveRefusesAnIdAlreadyTaken(t *testing.T) {
	svc := refactorWorld(t)
	mkDoc(t, svc, "shipping", "shipping.spec", "")
	_, err := svc.ObjectMove(stdctx.Background(), api.ObjectMoveInput{
		ID: "billing.spec", NewModule: "shipping", Reason: "x",
	})
	if !errors.Is(err, api.ErrAlreadyExists) {
		t.Fatalf("the target id is taken; the move must be refused: %v", err)
	}
	// THE REFUSAL MUST NAME THE TARGET MODULE. ObjectCreate would refuse this
	// too, one step later and with a message about an id — so without this
	// assertion the pre-check cannot be falsified, and a guard that cannot
	// fail is a guard that misleads. The check earns its place by saying
	// WHERE the collision is.
	if !strings.Contains(err.Error(), "shipping") {
		t.Errorf("the refusal must name the module the id is taken in: %v", err)
	}
}

// ── split ─────────────────────────────────────────────────────────────────

// TestSplitCreatesSuccessorsAndPlacesEveryChild — which part goes where is
// stated, never guessed.
func TestSplitCreatesSuccessorsAndPlacesEveryChild(t *testing.T) {
	svc := refactorFixture(t)
	ctx := stdctx.Background()
	mkDoc(t, svc, "billing", "billing.big", "")
	mkDoc(t, svc, "billing", "billing.c1", "billing.big")
	mkDoc(t, svc, "billing", "billing.c2", "billing.big")

	out, err := svc.ObjectSplit(ctx, api.ObjectSplitInput{
		ID:     "billing.big",
		Reason: "two concerns, two objects",
		Successors: []api.ObjectSplitTarget{
			{ID: "billing.left", Type: "doc", Title: "Left", Children: []string{"billing.c1"}},
			{ID: "billing.right", Type: "doc", Title: "Right", Children: []string{"billing.c2"}},
		},
	})
	if err != nil {
		t.Fatalf("ObjectSplit: %v", err)
	}
	if len(out.Successors) != 2 {
		t.Fatalf("successors: %v", out.Successors)
	}
	for id, want := range map[string]string{"billing.c1": "billing.left", "billing.c2": "billing.right"} {
		got, gerr := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "billing", ID: id})
		if gerr != nil {
			t.Fatalf("ObjectGet %s: %v", id, gerr)
		}
		if c := got.Object; c.Parent != want {
			t.Errorf("%s went to %q, want %q", id, c.Parent, want)
		}
	}
	if len(out.Orphans) != 0 {
		t.Errorf("every child was claimed; orphans: %v", out.Orphans)
	}
	if !out.SourceKept {
		t.Error("the source must be kept by default — deleting it is the irreversible choice")
	}
	if list, _ := svc.ObjectList(ctx, api.ObjectListInput{Type: "decision"}); len(list.Objects) != 0 {
		t.Errorf("a split wrote decision objects: %v", list.Objects)
	}
}

// TestSplitNamesTheChildrenNobodyAdopted — a split that quietly stranded
// three children would look successful.
func TestSplitNamesTheChildrenNobodyAdopted(t *testing.T) {
	svc := refactorFixture(t)
	ctx := stdctx.Background()
	mkDoc(t, svc, "billing", "billing.big", "")
	mkDoc(t, svc, "billing", "billing.c1", "billing.big")
	mkDoc(t, svc, "billing", "billing.forgotten", "billing.big")

	out, err := svc.ObjectSplit(ctx, api.ObjectSplitInput{
		ID: "billing.big", Reason: "x",
		Successors: []api.ObjectSplitTarget{
			{ID: "billing.a", Type: "doc", Title: "A", Children: []string{"billing.c1"}},
			{ID: "billing.b", Type: "doc", Title: "B"},
		},
	})
	if err != nil {
		t.Fatalf("ObjectSplit: %v", err)
	}
	if len(out.Orphans) != 1 || out.Orphans[0] != "billing.forgotten" {
		t.Errorf("orphans: %v, want the unclaimed child named", out.Orphans)
	}
}

// TestSplitRefusesToStrandChildrenOnDelete — removing the source while its
// children still hang off it would detach them silently.
func TestSplitRefusesToStrandChildrenOnDelete(t *testing.T) {
	svc := refactorFixture(t)
	ctx := stdctx.Background()
	mkDoc(t, svc, "billing", "billing.big", "")
	mkDoc(t, svc, "billing", "billing.orphan", "billing.big")

	_, err := svc.ObjectSplit(ctx, api.ObjectSplitInput{
		ID: "billing.big", Reason: "x", DeleteSource: true,
		Successors: []api.ObjectSplitTarget{
			{ID: "billing.a", Type: "doc", Title: "A"},
			{ID: "billing.b", Type: "doc", Title: "B"},
		},
	})
	if err == nil {
		t.Fatal("deleting a source whose children nobody adopted must be refused")
	}
	if !strings.Contains(err.Error(), "billing.orphan") {
		t.Errorf("the refusal must name what would be stranded: %v", err)
	}
	if _, gerr := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "billing", ID: "billing.big"}); gerr != nil {
		t.Error("the source was deleted despite the refusal")
	}
}

// TestSplitChecksTheClaimsBeforeWritingAnything — a split that half-succeeded
// because the fourth successor named a typo leaves a shape nobody asked for.
func TestSplitChecksTheClaimsBeforeWritingAnything(t *testing.T) {
	svc := refactorFixture(t)
	ctx := stdctx.Background()
	mkDoc(t, svc, "billing", "billing.big", "")
	mkDoc(t, svc, "billing", "billing.c1", "billing.big")

	for _, tc := range []struct {
		name string
		in   api.ObjectSplitInput
		want string
	}{
		{"a child that is not one", api.ObjectSplitInput{
			ID: "billing.big", Reason: "x",
			Successors: []api.ObjectSplitTarget{
				{ID: "billing.a", Type: "doc", Title: "A", Children: []string{"billing.nope"}},
				{ID: "billing.b", Type: "doc", Title: "B"},
			}}, "not a child"},
		{"one child claimed twice", api.ObjectSplitInput{
			ID: "billing.big", Reason: "x",
			Successors: []api.ObjectSplitTarget{
				{ID: "billing.a", Type: "doc", Title: "A", Children: []string{"billing.c1"}},
				{ID: "billing.b", Type: "doc", Title: "B", Children: []string{"billing.c1"}},
			}}, "claimed by two"},
		{"fewer than two successors", api.ObjectSplitInput{
			ID: "billing.big", Reason: "x",
			Successors: []api.ObjectSplitTarget{{ID: "billing.a", Type: "doc", Title: "A"}},
		}, "at least two"},
	} {
		_, err := svc.ObjectSplit(ctx, tc.in)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v", tc.name, err)
		}
		// NOTHING was written on the way to the refusal.
		if _, gerr := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "billing", ID: "billing.a"}); gerr == nil {
			t.Errorf("%s: a successor was written before the check ran", tc.name)
		}
	}
}

// A RENAME REWRITES EVERY REFERENCE: the child's parent, the inbound edge from
// another module and the object's own edge to itself follow the new id; the
// parent is kept; a taken id is refused; prose is not a reference and is not
// rewritten.
func TestRenameWithinAModuleRewritesEveryReference(t *testing.T) {
	svc := refactorWorld(t)
	ctx := stdctx.Background()
	spec, _ := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "billing", ID: "billing.spec"})
	o := spec.Object
	o.Edges = []api.ObjectEdge{{Kind: "serves", To: "billing.spec"}}
	o.Body = "See billing.spec for the old name."
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "billing", Object: o}); err != nil {
		t.Fatal(err)
	}
	mkDoc(t, svc, "billing", "billing.taken", "")
	if _, err := svc.ObjectMove(ctx, api.ObjectMoveInput{ID: "billing.spec", NewLocalID: "taken", Reason: "x"}); !errors.Is(err, api.ErrAlreadyExists) {
		t.Fatalf("a rename onto a taken id: %v — must refuse", err)
	}

	out, err := svc.ObjectMove(ctx, api.ObjectMoveInput{ID: "billing.spec", NewLocalID: "specification", Reason: "the name says what it is"})
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if out.ID != "billing.specification" || out.PreviousID != "billing.spec" || len(out.Dangling) != 0 {
		t.Fatalf("rename output: %+v", out)
	}
	got, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "billing", ID: "billing.specification"})
	if err != nil {
		t.Fatalf("the renamed object: %v", err)
	}
	if got.Object.Parent != "" || len(got.Object.Edges) != 1 || got.Object.Edges[0].To != "billing.specification" {
		t.Errorf("renamed: parent %q edges %+v — the parent kept, its own edge following it", got.Object.Parent, got.Object.Edges)
	}
	if !strings.Contains(got.Object.Body, "billing.spec for") {
		t.Errorf("prose was rewritten: %q", got.Object.Body)
	}
	child, _ := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "billing", ID: "billing.chapter"})
	plan, _ := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "shipping", ID: "shipping.plan"})
	if child.Object.Parent != "billing.specification" || plan.Object.Edges[0].To != "billing.specification" {
		t.Errorf("references not rewritten: child parent %q, inbound edge %+v", child.Object.Parent, plan.Object.Edges)
	}
	if _, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "billing", ID: "billing.spec"}); err == nil {
		t.Error("the old id is still there")
	}
	errs, _ := svc.ObjectErrorList(ctx, api.ObjectErrorListInput{})
	for _, e := range errs.Objects {
		for _, d := range e.Errors {
			if d.Code == api.DiagnosticDanglingLink || d.Code == api.DiagnosticDanglingParent {
				t.Errorf("a rename left %s with %s", e.ID, d.Label)
			}
		}
	}
	if _, err := svc.ObjectMove(ctx, api.ObjectMoveInput{ID: "billing.chapter", NewLocalID: "a/b", Reason: "x"}); !errors.Is(err, api.ErrInvalidInput) {
		t.Errorf("a local id with a separator: %v — must refuse", err)
	}
}
