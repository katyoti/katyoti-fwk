// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// WRITES ARE SERIALISED. A write over a revision that is no longer stored is
// refused with ErrConflict; CWM's own read-modify-write verbs redo their
// change instead of erasing another's; two moves from one state cannot both
// pass; two creates cannot mint one id.

func concurrentFixture(t *testing.T) (*Service, stdctx.Context) {
	t.Helper()
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir(), ObjectTypeValidation: api.TypeValidationError}
	caps := []api.Capability{{Name: "lifecycle"}}
	for i := 0; i < 12; i++ {
		caps = append(caps, api.Capability{Name: fmt.Sprintf("c%d", i)})
	}
	cfg.CatalogTypes.Capabilities = caps
	cfg.CatalogTypes.Workflows = []api.Workflow{{Name: "flow", Initial: "proposed",
		States: []api.WorkflowState{{Name: "proposed"}, {Name: "accepted"}, {Name: "rejected"}},
		Transitions: []api.WorkflowTransition{
			{Name: "accept", From: []string{"proposed"}, To: "accepted"},
			{Name: "reject", From: []string{"proposed"}, To: "rejected"}}}}
	cfg.CatalogTypes.Objects = []api.ObjectType{
		{Name: "module"},
		{Name: "goal", Capabilities: []string{"lifecycle"}, Workflow: "flow"},
		{Name: "request", IDPrefix: "RQ"},
	}
	cfg.CatalogTypes.Edges = []api.EdgeTypeRegistration{{Name: "about", From: "any", To: "any", Cardinality: "M:N", Target: "object", ObjectStorage: "edges"}}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	ctx := stdctx.Background()
	for _, o := range []api.Object{{ID: "m", Type: "module", Name: "m"}, {ID: "m.g", Type: "goal", Name: "g", Parent: "m"}} {
		if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 12; i++ {
		o := api.Object{ID: fmt.Sprintf("m.t%d", i), Type: "module", Name: "t", Parent: "m"}
		if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			t.Fatal(err)
		}
	}
	return svc, ctx
}

// A read-modify-write that passes its revision is refused when another write
// landed; one that passes none writes as it always did.
func TestAStaleRevisionIsRefused(t *testing.T) {
	svc, ctx := concurrentFixture(t)
	read, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "m", ID: "m.g"})
	if err != nil || read.Revision == "" {
		t.Fatalf("ObjectGet: revision %q, %v", read.Revision, err)
	}
	if _, err := svc.ObjectAddCapability(ctx, api.ObjectAddCapabilityInput{ID: "m.g", Capability: "c1"}); err != nil {
		t.Fatal(err)
	}
	stale := read.Object
	stale.Description = "edited on an old read"
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "m", Object: stale, Revision: read.Revision}); !errors.Is(err, api.ErrConflict) {
		t.Fatalf("a write over a newer version: %v — want ErrConflict", err)
	}
	now, _ := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "m", ID: "m.g"})
	if _, ok := now.Object.Capabilities["c1"]; !ok {
		t.Error("the refused write erased the capability anyway")
	}
	fresh := now.Object
	fresh.Description = "edited on a fresh read"
	out, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "m", Object: fresh, Revision: now.Revision})
	if err != nil || out.Revision == "" || out.Revision == now.Revision {
		t.Fatalf("a write on the current version: %+v, %v — want written with a new revision", out.Revision, err)
	}
	// No revision: unconditional, as before.
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "m", Object: stale}); err != nil {
		t.Errorf("an unconditional write: %v", err)
	}
}

// Many writers on one object at once — capabilities added, edges created —
// and not one write is lost.
func TestConcurrentWritesOnOneObjectLoseNothing(t *testing.T) {
	svc, ctx := concurrentFixture(t)
	var wg sync.WaitGroup
	errs := make(chan error, 24)
	for i := 0; i < 12; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			if _, err := svc.ObjectAddCapability(ctx, api.ObjectAddCapabilityInput{ID: "m.g", Capability: fmt.Sprintf("c%d", i)}); err != nil {
				errs <- err
			}
		}(i)
		go func(i int) {
			defer wg.Done()
			if _, err := svc.EdgeCreate(ctx, api.EdgeCreateInput{From: "m.g", To: fmt.Sprintf("m.t%d", i), Kind: "about"}); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("a concurrent write failed: %v", err)
	}
	got, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "m", ID: "m.g"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		if _, ok := got.Object.Capabilities[fmt.Sprintf("c%d", i)]; !ok {
			t.Errorf("capability c%d was lost", i)
		}
	}
	if len(got.Object.Edges) != 12 {
		t.Errorf("edges kept: %d of 12", len(got.Object.Edges))
	}
}

// Two moves from one state at once: one passes, the other is judged from the
// state the first left — and refused.
func TestTwoMovesFromOneStateCannotBothPass(t *testing.T) {
	svc, ctx := concurrentFixture(t)
	var wg sync.WaitGroup
	results := make([]error, 2)
	for i, to := range []string{"accepted", "rejected"} {
		wg.Add(1)
		go func(i int, to string) {
			defer wg.Done()
			_, results[i] = svc.ObjectTransition(ctx, api.ObjectTransitionInput{ID: "m.g", To: to, By: api.Actor{ID: "ana", Kind: api.ActorPerson}})
		}(i, to)
	}
	wg.Wait()
	passed := 0
	for _, err := range results {
		if err == nil {
			passed++
		}
	}
	got, _ := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "m", ID: "m.g"})
	life, _ := got.Object.Lifecycle()
	if passed != 1 || (life.State != "accepted" && life.State != "rejected") {
		t.Errorf("moves passed: %d (%v), state %q — want exactly one", passed, results, life.State)
	}
}

// Creates minting ids at once: every one gets its own id and its own file.
func TestConcurrentCreatesMintDistinctIDs(t *testing.T) {
	svc, ctx := concurrentFixture(t)
	const n = 16
	var wg sync.WaitGroup
	ids := make(chan string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			out, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: api.Object{Type: "request", Name: fmt.Sprintf("req %d", i), Parent: "m"}})
			if err != nil {
				t.Errorf("create %d: %v", i, err)
				return
			}
			ids <- out.Object.ID
		}(i)
	}
	wg.Wait()
	close(ids)
	seen := map[string]bool{}
	for id := range ids {
		if seen[id] {
			t.Errorf("id %s minted twice", id)
		}
		seen[id] = true
	}
	list, _ := svc.ObjectList(ctx, api.ObjectListInput{Module: "m", Type: "request"})
	if len(seen) != n || len(list.Objects) != n {
		t.Errorf("distinct ids %d, stored requests %d — want %d each", len(seen), len(list.Objects), n)
	}
}
