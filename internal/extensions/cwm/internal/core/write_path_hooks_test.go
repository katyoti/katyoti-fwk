// SPDX-License-Identifier: Apache-2.0

package core

import (
	"errors"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// EVERY WRITE GOES THROUGH THE WRITE PATH. An edge edit is an update of its
// source object — before_update may refuse it, on_update fires — and a delete
// can be refused by before_delete before anything changes.

func TestAnEdgeEditRunsTheUpdateHooks(t *testing.T) {
	f := &hookActions{replies: map[string]string{"x:gate/create": `{"accept":true}`, "x:gate/update": `{"accept":false,"reason":"frozen"}`}}
	svc, ctx, _ := dispatchFixture(t, f)
	for _, id := range []string{"billing.a", "billing.b"} {
		if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: gatedObject(id)}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	_, err := svc.EdgeCreate(ctx, api.EdgeCreateInput{From: "billing.a", To: "billing.b", Kind: "about"})
	if !errors.Is(err, api.ErrRefusedByHook) || !strings.Contains(err.Error(), "frozen") {
		t.Fatalf("EdgeCreate on a frozen object: %v — want refused by before_update", err)
	}
	got, _ := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "billing", ID: "billing.a"})
	if len(got.Object.Edges) != 0 {
		t.Fatalf("a refused edge was written: %+v", got.Object.Edges)
	}
	calls := f.callsTo("x:gate/update")
	if len(calls) != 1 || calls[0].Prior == nil || len(calls[0].Object.Edges) != 1 {
		t.Fatalf("before_update calls = %+v, want one carrying the prior and the new edge", calls)
	}
}

func TestBeforeDeleteMayRefuseADelete(t *testing.T) {
	f := &hookActions{replies: map[string]string{"x:gate/create": `{"accept":true}`, "x:gate/delete": `{"accept":false,"reason":"still in use"}`}}
	svc, ctx, _ := dispatchFixture(t, f)
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: gatedObject("billing.a")}); err != nil {
		t.Fatal(err)
	}
	_, err := svc.ObjectDelete(ctx, api.ObjectDeleteInput{Module: "billing", ID: "billing.a"})
	if !errors.Is(err, api.ErrRefusedByHook) || !strings.Contains(err.Error(), "still in use") {
		t.Fatalf("delete: %v — want refused by before_delete with its reason", err)
	}
	if _, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "billing", ID: "billing.a"}); err != nil {
		t.Fatalf("a refused delete removed the object: %v", err)
	}
	if calls := f.callsTo("x:gate/delete"); len(calls) != 1 || calls[0].Object.ID != "billing.a" || calls[0].Point != api.HookBeforeDelete {
		t.Fatalf("before_delete calls = %+v", calls)
	}
}

// A move removes the old identity without being a delete: before_delete is
// not asked.
func TestAMoveDoesNotAskBeforeDelete(t *testing.T) {
	f := &hookActions{replies: map[string]string{"x:gate/create": `{"accept":true}`, "x:gate/update": `{"accept":true}`, "x:gate/delete": `{"accept":false,"reason":"no"}`}}
	svc, ctx, _ := dispatchFixture(t, f)
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: gatedObject("billing.a")}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ObjectMove(ctx, api.ObjectMoveInput{ID: "billing.a", NewLocalID: "z", Reason: "renamed"}); err != nil {
		t.Fatalf("rename: %v — before_delete must not be asked on a move", err)
	}
	if calls := f.callsTo("x:gate/delete"); len(calls) != 0 {
		t.Fatalf("before_delete asked on a move: %+v", calls)
	}
}
