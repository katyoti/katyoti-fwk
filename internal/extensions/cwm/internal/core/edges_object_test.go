// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// THE OBJECT INDEX IS THE EDGE STORE (phase 7, cwm.p1n).
//
// Before: two edge systems, neither aware of the other. `Object.Edges` was
// authored in the file (the reference rule, concepts.md §3) while EdgeList /
// EdgeCreate / EdgeDelete read and wrote s.Graph, loaded from the node/item
// state the cut removes. 145 authored edges in the corpus, visible to no
// verb. These tests are the behavioural gate: author an edge on an object,
// see EdgeList return it; create one through the verb, see it in the FILE.

func edgeFixture(t *testing.T) (*Service, string) {
	t.Helper()
	root := freshWorkspace(t)
	project := t.TempDir()
	svc, err := New(stdctx.Background(), &CWMServiceConfig{WorkspaceRoot: root, ProjectRoot: project})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	ctx := stdctx.Background()
	for _, id := range []string{"m.a", "m.b", "m.c"} {
		o := api.Object{ID: id, Type: "goal"}
		o.Name = id
		if id == "m.a" {
			o.Edges = []api.ObjectEdge{{Kind: "serves", To: "m.b"}}
		}
		if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			t.Fatalf("ObjectCreate %s: %v", id, err)
		}
	}
	return svc, project
}

func edgeSet(edges []api.Edge) map[string]bool {
	out := map[string]bool{}
	for _, e := range edges {
		out[e.From+" -"+string(e.Kind)+"-> "+e.To] = true
	}
	return out
}

func TestObjectAuthoredEdgeReachesEdgeList(t *testing.T) {
	svc, _ := edgeFixture(t)
	ctx := stdctx.Background()
	const want = "m.a -serves-> m.b"

	all, err := svc.EdgeList(ctx, api.EdgeListInput{})
	if err != nil {
		t.Fatalf("EdgeList all: %v", err)
	}
	if !edgeSet(all.Edges)[want] {
		t.Fatalf("the edge authored on m.a is invisible to EdgeList: %v", all.Edges)
	}
	out, _ := svc.EdgeList(ctx, api.EdgeListInput{ObjectID: "m.a", Direction: api.DirectionOut})
	if !edgeSet(out.Edges)[want] || len(out.Edges) != 1 {
		t.Errorf("outgoing from m.a: %v, want exactly [%s]", out.Edges, want)
	}
	in, _ := svc.EdgeList(ctx, api.EdgeListInput{ObjectID: "m.b", Direction: api.DirectionIn})
	if !edgeSet(in.Edges)[want] || len(in.Edges) != 1 {
		t.Errorf("incoming to m.b: %v, want exactly [%s] — 'what points at this object' is THE index question", in.Edges, want)
	}
	none, _ := svc.EdgeList(ctx, api.EdgeListInput{ObjectID: "m.b", Direction: api.DirectionOut})
	if len(none.Edges) != 0 {
		t.Errorf("outgoing from m.b: %v, want none", none.Edges)
	}
	kind, _ := svc.EdgeList(ctx, api.EdgeListInput{ObjectID: "m.a", Kind: "impacts"})
	if len(kind.Edges) != 0 {
		t.Errorf("kind filter leaked: %v", kind.Edges)
	}
}

func TestEdgeCreateOnAnObjectEditsItsFile(t *testing.T) {
	svc, project := edgeFixture(t)
	ctx := stdctx.Background()

	if _, err := svc.EdgeCreate(ctx, api.EdgeCreateInput{From: "m.a", To: "m.c", Kind: "impacts"}); err != nil {
		t.Fatalf("EdgeCreate on an object: %v", err)
	}
	// Idempotent: the same triple again is a no-op success.
	if _, err := svc.EdgeCreate(ctx, api.EdgeCreateInput{From: "m.a", To: "m.c", Kind: "impacts"}); err != nil {
		t.Fatalf("EdgeCreate twice: %v", err)
	}
	obj, err := ReadObjectFile(filepath.Join(project, ModulesDirName, "m", ObjectsDirName, "a"+ObjectFileSuffix))
	if err != nil {
		t.Fatalf("read m.a: %v", err)
	}
	if len(obj.Edges) != 2 || obj.Edges[0] != (api.ObjectEdge{Kind: "impacts", To: "m.c"}) || obj.Edges[1] != (api.ObjectEdge{Kind: "serves", To: "m.b"}) {
		t.Fatalf("m.a's FILE after EdgeCreate: %v, want [impacts→m.c, serves→m.b] (sorted, no duplicate)", obj.Edges)
	}
	listed, _ := svc.EdgeList(ctx, api.EdgeListInput{ObjectID: "m.c", Direction: api.DirectionIn})
	if !edgeSet(listed.Edges)["m.a -impacts-> m.c"] {
		t.Errorf("the created edge is not listed: %v", listed.Edges)
	}

	if _, err := svc.EdgeDelete(ctx, api.EdgeDeleteInput{From: "m.a", To: "m.c", Kind: "impacts"}); err != nil {
		t.Fatalf("EdgeDelete: %v", err)
	}
	obj, _ = ReadObjectFile(filepath.Join(project, ModulesDirName, "m", ObjectsDirName, "a"+ObjectFileSuffix))
	if len(obj.Edges) != 1 || obj.Edges[0].To != "m.b" {
		t.Errorf("m.a's FILE after EdgeDelete: %v, want [serves→m.b]", obj.Edges)
	}
	if _, err := svc.EdgeDelete(ctx, api.EdgeDeleteInput{From: "m.a", To: "m.c", Kind: "impacts"}); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("deleting an edge that is not there: %v, want ErrNotFound — a silent no-op hides a wrong id", err)
	}
	if _, err := svc.EdgeCreate(ctx, api.EdgeCreateInput{From: "m.a", To: "m.b", Kind: api.EdgeTypeParentID}); !errors.Is(err, api.ErrTypeRuleViolation) {
		t.Errorf("parent_id as an edge on an object: %v, want refused — the tree is a field, never an edge (§3)", err)
	}
}

// The tree the GUI asks for (DrawnOnly) shows a drawn type and hides one that
// is not, lifting its children rather than dropping them.
func TestDrawnTreeHidesAnUndrawnTypeAndLiftsItsChildren(t *testing.T) {
	root := freshWorkspace(t)
	project := t.TempDir()
	cfg := &CWMServiceConfig{WorkspaceRoot: root, ProjectRoot: project}
	cfg.CatalogTypes.Objects = []api.ObjectType{
		{Name: "dossier", Tree: api.TreeNode},
		{Name: "record"},
	}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	ctx := stdctx.Background()
	mk := func(id, typ, parent string) {
		o := api.Object{ID: id, Type: typ}
		o.Name = id
		o.Parent = parent
		if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	mk("m.d1", "dossier", "")
	mk("m.r1", "record", "m.d1")
	mk("m.d2", "dossier", "m.r1") // a dossier filed under a record: lifted, never dropped

	tree, err := svc.ObjectTree(ctx, api.ObjectTreeInput{Root: "m.d1", DrawnOnly: true})
	if err != nil {
		t.Fatalf("ObjectTree: %v", err)
	}
	got := map[string]int{}
	flattenObjectTreeIDs(tree.Roots, 0, got)
	if _, ok := got["m.d1"]; !ok {
		t.Errorf("the drawn dossier is not in the tree: %v", got)
	}
	if _, ok := got["m.r1"]; ok {
		t.Errorf("the undrawn record is in the tree: %v", got)
	}
	if d, ok := got["m.d2"]; !ok || d != 1 {
		t.Errorf("the dossier under the hidden record should be lifted to depth 1, got %v", got)
	}
}

func flattenObjectTreeIDs(roots []api.ObjectTreeNode, depth int, out map[string]int) {
	for _, r := range roots {
		out[r.Object.ID] = depth
		flattenObjectTreeIDs(r.Children, depth+1, out)
	}
}
