// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// AN EDGE REACHES ANOTHER MODULE. A bare target is the source module's own
// object when it exists, else the object with that bare id — a module object —
// else a dangling link, refused (error mode) or reported (warn mode). The same
// on EdgeCreate, on ObjectUpdate's authored edges and on the scan.

func moduleEdgeFixture(t *testing.T, mode string) (*Service, stdctx.Context) {
	t.Helper()
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir(), ObjectTypeValidation: mode}
	cfg.CatalogTypes.Objects = []api.ObjectType{{Name: "module"}, {Name: "note"}}
	cfg.CatalogTypes.Edges = []api.EdgeTypeRegistration{
		{Name: "reads-module", From: "any", To: "any", Cardinality: "M:N", Target: "object", ObjectStorage: "edges"},
		{Name: "about", From: "any", To: "any", Cardinality: "M:N", Target: "object", ObjectStorage: "edges"},
	}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	ctx := stdctx.Background()
	for _, o := range []api.Object{
		{ID: "billing", Type: "module", Name: "billing"},
		{ID: "stock", Type: "module", Name: "stock"},
		{ID: "billing.n1", Type: "note", Name: "n1", Parent: "billing"},
	} {
		module, _ := api.ModuleOfObjectID(o.ID)
		if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: module, Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}
	return svc, ctx
}

func edgeTargets(t *testing.T, svc *Service, ctx stdctx.Context, id, kind string) []string {
	t.Helper()
	out, err := svc.EdgeList(ctx, api.EdgeListInput{ObjectID: id, Kind: api.EdgeType(kind), Direction: api.DirectionOut})
	if err != nil {
		t.Fatal(err)
	}
	var to []string
	for _, e := range out.Edges {
		to = append(to, e.To)
	}
	return to
}

func TestAnEdgeReachesAnotherModuleByItsBareID(t *testing.T) {
	svc, ctx := moduleEdgeFixture(t, api.TypeValidationError)

	// EdgeCreate: billing reads stock.
	got, err := svc.EdgeCreate(ctx, api.EdgeCreateInput{From: "billing", To: "stock", Kind: "reads-module"})
	if err != nil || got.Edge.To != "stock" {
		t.Fatalf("EdgeCreate billing -> stock: %+v, %v — want the bare module id kept", got.Edge, err)
	}
	if to := edgeTargets(t, svc, ctx, "billing", "reads-module"); len(to) != 1 || to[0] != "stock" {
		t.Errorf("read back: %v, want [stock]", to)
	}
	// The other end sees it: the target resolves.
	in, _ := svc.EdgeList(ctx, api.EdgeListInput{ObjectID: "stock", Direction: api.DirectionIn})
	if len(in.Edges) != 1 || in.Edges[0].From != "billing" {
		t.Errorf("stock's incoming edges: %+v", in.Edges)
	}

	// ObjectUpdate with an authored edge, and the scan, read it the same way.
	n, _ := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "billing", ID: "billing.n1"})
	o := n.Object
	o.Edges = []api.ObjectEdge{{Kind: "about", To: "stock"}}
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "billing", Object: o}); err != nil {
		t.Fatalf("ObjectUpdate with an edge to a module: %v", err)
	}
	if _, err := svc.ObjectScan(ctx, api.ObjectScanInput{Force: true}); err != nil {
		t.Fatal(err)
	}
	if to := edgeTargets(t, svc, ctx, "billing.n1", "about"); len(to) != 1 || to[0] != "stock" {
		t.Errorf("after a rescan: %v, want [stock]", to)
	}
	if errs := svc.problemsOf("billing.n1"); len(errs) != 0 {
		t.Errorf("the edge to a module reads as an error: %+v", errs)
	}
}

// A bare id of the source's own module keeps its meaning: `to: n1` from
// billing is billing.n1, even when a module could be named so.
func TestAnInModuleBareIDIsStillQualified(t *testing.T) {
	svc, ctx := moduleEdgeFixture(t, api.TypeValidationError)
	got, err := svc.EdgeCreate(ctx, api.EdgeCreateInput{From: "billing", To: "n1", Kind: "about"})
	if err != nil || got.Edge.To != "billing.n1" {
		t.Fatalf("EdgeCreate billing -> n1: %+v, %v — want billing.n1", got.Edge, err)
	}
	// Both exist — billing's own `stock` and the module stock: the module's
	// own object wins, as a short id always meant.
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: api.Object{ID: "billing.stock", Type: "note", Name: "stock", Parent: "billing"}}); err != nil {
		t.Fatal(err)
	}
	got, err = svc.EdgeCreate(ctx, api.EdgeCreateInput{From: "billing", To: "stock", Kind: "about"})
	if err != nil || got.Edge.To != "billing.stock" {
		t.Fatalf("EdgeCreate billing -> stock with billing.stock present: %+v, %v — want billing.stock", got.Edge, err)
	}
}

// A target that is no object is refused in error mode, on both write paths,
// and reported on the object in warn mode — never stored silently.
func TestAnEdgeToNothingIsRefusedOrReported(t *testing.T) {
	svc, ctx := moduleEdgeFixture(t, api.TypeValidationError)
	if _, err := svc.EdgeCreate(ctx, api.EdgeCreateInput{From: "billing", To: "ghost", Kind: "reads-module"}); !errors.Is(err, api.ErrInvalidInput) {
		t.Errorf("EdgeCreate to nothing: %v — want refused", err)
	}
	n, _ := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "billing", ID: "billing.n1"})
	o := n.Object
	o.Edges = []api.ObjectEdge{{Kind: "about", To: "ghost"}}
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "billing", Object: o}); !errors.Is(err, api.ErrInvalidInput) {
		t.Errorf("ObjectUpdate with an edge to nothing: %v — want refused", err)
	}

	warn, wctx := moduleEdgeFixture(t, api.TypeValidationWarn)
	if _, err := warn.EdgeCreate(wctx, api.EdgeCreateInput{From: "billing", To: "ghost", Kind: "reads-module"}); err != nil {
		t.Fatalf("warn mode stores it: %v", err)
	}
	got, err := warn.ObjectGet(wctx, api.ObjectGetInput{Module: "billing", ID: "billing"})
	if err != nil {
		t.Fatal(err)
	}
	reported := false
	for _, d := range got.Errors {
		reported = reported || d.Code == api.DiagnosticDanglingLink
	}
	if !reported {
		t.Errorf("warn mode: the edge to nothing is not reported on the object: %+v", got.Errors)
	}
}
