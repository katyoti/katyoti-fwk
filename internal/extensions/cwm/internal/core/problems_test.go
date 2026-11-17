// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// A target that goes makes its referrers' errors at once, and one that comes
// back clears them — the index marks the objects pointing at an id that
// appears or goes, so the next read recomputes them without a scan.
func TestDeletingATargetMarksItsReferrersInError(t *testing.T) {
	svc, _ := edgeFixture(t) // m.a -serves-> m.b
	ctx := stdctx.Background()
	if errs := linkErrors(svc, "m.a"); len(errs) != 0 {
		t.Fatalf("m.a before: %+v", errs)
	}
	// The target goes the way a pull removes a file: no guard asked (the
	// guarded ObjectDelete refuses it, see TestDeleteNeverLeavesALinkDangling).
	if _, err := svc.deleteObject(ctx, api.ObjectDeleteInput{Module: "m", ID: "m.b"}); err != nil {
		t.Fatalf("deleteObject: %v", err)
	}
	errs := linkErrors(svc, "m.a")
	if len(errs) != 1 || errs[0].Code != api.DiagnosticDanglingLink {
		t.Fatalf("m.a after its target went: %+v, want one dangling-link", errs)
	}
	if errs := linkErrors(svc, "m.c"); len(errs) != 0 {
		t.Errorf("m.c points at nothing that went: %+v", errs)
	}
	if got := svc.inError([]string{"m.a"}); len(got) != 1 {
		t.Errorf("m.a is not marked in error: %v", got)
	}
	list, err := svc.ObjectErrorList(ctx, api.ObjectErrorListInput{})
	if err != nil {
		t.Fatal(err)
	}
	listed := false
	for _, o := range list.Objects {
		listed = listed || o.ID == "m.a"
	}
	if !listed {
		t.Errorf("ObjectErrorList misses m.a: %+v", list.Objects)
	}

	b := api.Object{ID: "m.b", Type: "goal"}
	b.Name = "m.b"
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: b}); err != nil {
		t.Fatalf("ObjectCreate m.b: %v", err)
	}
	if errs := linkErrors(svc, "m.a"); len(errs) != 0 {
		t.Fatalf("m.a once its target is back: %+v", errs)
	}
}

func linkErrors(svc *Service, id string) []api.ObjectDiagnostic {
	var out []api.ObjectDiagnostic
	for _, d := range svc.problemsOf(id) {
		if d.Code == api.DiagnosticDanglingLink || d.Code == api.DiagnosticDanglingParent {
			out = append(out, d)
		}
	}
	return out
}

// seedModule writes the module object a module declares itself with.
func seedModule(t *testing.T, svc *Service, m string) {
	t.Helper()
	o := api.Object{ID: m, Type: "module"}
	o.Name = m
	if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: m, Object: o}); err != nil {
		t.Fatalf("seed module %s: %v", m, err)
	}
}

// AN OBJECT IN ERROR IS REPAIRED ONE WRITE AT A TIME: a write is refused for
// what it adds, never for what the object already carried.
func TestAWriteIsRefusedOnlyForWhatItAdds(t *testing.T) {
	svc, _ := edgeKindsFixture(t, api.TypeValidationError)
	ctx := stdctx.Background()
	// m.g1 serves m.g2; m.g2 goes behind CWM's back: m.g1 now carries a
	// dangling link.
	if err := withEdge(svc, t, "m.g1", "serves", "m.g2"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.deleteObject(ctx, api.ObjectDeleteInput{Module: "m", ID: "m.g2"}); err != nil {
		t.Fatal(err)
	}
	cur, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "m", ID: "m.g1"})
	if err != nil || len(cur.Errors) == 0 {
		t.Fatalf("m.g1 must read with its dangling link: %+v, %v", cur.Errors, err)
	}
	o := cur.Object
	o.Description = "renamed while still in error"
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "m", Object: o}); err != nil {
		t.Fatalf("a write that adds no error was refused: %v", err)
	}
	o.Edges = append(o.Edges, api.ObjectEdge{Kind: "serves", To: "m.ghost"})
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "m", Object: o}); err == nil || !strings.Contains(err.Error(), "m.ghost") {
		t.Fatalf("a write that adds a dangling link must be refused naming it: %v", err)
	}
}

// DELETE NEVER LEAVES A LINK DANGLING: an object with children, or one others
// link to, is refused naming them; Detach removes the links and lists them.
func TestDeleteNeverLeavesALinkDangling(t *testing.T) {
	svc, _ := edgeFixture(t) // m.a -serves-> m.b
	ctx := stdctx.Background()
	_, err := svc.ObjectDelete(ctx, api.ObjectDeleteInput{Module: "m", ID: "m.b"})
	if !errors.Is(err, api.ErrReferenced) || !strings.Contains(err.Error(), "m.a -serves->") {
		t.Fatalf("delete of a linked object: %v — must refuse naming the link", err)
	}
	out, err := svc.ObjectDelete(ctx, api.ObjectDeleteInput{Module: "m", ID: "m.b", Detach: true})
	if err != nil {
		t.Fatalf("delete with detach: %v", err)
	}
	if len(out.Detached) != 1 || out.Detached[0] != "m.a" {
		t.Errorf("detached = %v, want [m.a]", out.Detached)
	}
	a, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "m", ID: "m.a"})
	if err != nil || len(a.Object.Edges) != 0 || len(linkErrors(svc, "m.a")) != 0 {
		t.Errorf("m.a after detach: edges %v, errors %+v, %v", a.Object.Edges, a.Errors, err)
	}

	child := api.Object{ID: "m.kid", Type: "goal"}
	child.Name = "kid"
	child.Parent = "m.c"
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: child}); err != nil {
		t.Fatal(err)
	}
	_, err = svc.ObjectDelete(ctx, api.ObjectDeleteInput{Module: "m", ID: "m.c", Detach: true})
	if !errors.Is(err, api.ErrReferenced) || !strings.Contains(err.Error(), "m.kid") {
		t.Fatalf("delete of a parent: %v — must refuse naming its children, detach or not", err)
	}
}
