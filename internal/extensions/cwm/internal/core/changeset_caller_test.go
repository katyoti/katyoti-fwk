// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// A CALLER'S CHANGE SET: any in-process caller opens a set under
// an id of its choosing, marks its writes with api.WithChangeSet, and commits
// them all or none; its hooks go out at the commit, never held in an outbox.

const callerSet = "ops.RQ-1/end"

// callerFixture: billing.b exists; a person acts.
func callerFixture(t *testing.T) (*Service, stdctx.Context, *durableActions, *objectEvents, *CWMServiceConfig) {
	t.Helper()
	cfg := asyncConfig(t)
	acts := newDurableActions()
	svc, ctx, bus := openAsync(t, cfg, acts)
	t.Cleanup(func() { _ = svc.Close() })
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: typed("billing.b", "chained")}); err != nil {
		t.Fatal(err)
	}
	ctx = cause.WithCause(ctx, cause.Cause{Kind: cause.KindPerson, ID: "jdoe"})
	return svc, ctx, acts, watchObjects(bus, "billing.a", "billing.b"), cfg
}

// stageBoth stages a create of billing.a and an update of billing.b in the
// caller's set.
func stageBoth(t *testing.T, svc *Service, ctx stdctx.Context) {
	t.Helper()
	if _, err := svc.ChangeSetBegin(ctx, api.ChangeSetBeginInput{Set: callerSet}); err != nil {
		t.Fatalf("begin: %v", err)
	}
	sctx := api.WithChangeSet(ctx, callerSet)
	if _, err := svc.ObjectCreate(sctx, api.ObjectCreateInput{Module: "billing", Object: typed("billing.a", "chained")}); err != nil {
		t.Fatalf("stage create: %v", err)
	}
	got, err := svc.ObjectGet(sctx, api.ObjectGetInput{ID: "billing.b"})
	if err != nil {
		t.Fatal(err)
	}
	b := got.Object
	b.Description = "the caller's"
	if _, err := svc.ObjectUpdate(sctx, api.ObjectUpdateInput{Module: "billing", Object: b, Revision: got.Revision}); err != nil {
		t.Fatalf("stage update: %v", err)
	}
}

// Two writes staged in a caller's set: nothing lands before the commit; a
// failure at the commit (B changed meanwhile) leaves neither written; a fresh
// set committed writes both.
func TestACallerChangeSetWritesAllOrNone(t *testing.T) {
	svc, ctx, _, _, cfg := callerFixture(t)
	if _, err := svc.ChangeSetBegin(ctx, api.ChangeSetBeginInput{Reaction: atomicReaction, Set: callerSet}); !errors.Is(err, api.ErrInvalidInput) {
		t.Errorf("begin with a reaction and a set = %v, want refused", err)
	}
	if _, err := svc.ChangeSetBegin(ctx, api.ChangeSetBeginInput{}); !errors.Is(err, api.ErrInvalidInput) {
		t.Errorf("begin with neither = %v, want refused", err)
	}

	stageBoth(t, svc, ctx)
	if _, ok := onDisk(t, cfg, "billing.a"); ok {
		t.Fatal("a staged create is on disk before the commit")
	}
	if got, _ := onDisk(t, cfg, "billing.b"); got.Description != "" {
		t.Fatal("a staged update is on disk before the commit")
	}
	if _, err := svc.ObjectGet(ctx, api.ObjectGetInput{ID: "billing.a"}); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("a write outside the set sees the staged create: %v", err)
	}
	sets, _ := svc.ChangeSetList(ctx, api.ChangeSetListInput{})
	if len(sets.Sets) != 1 || sets.Sets[0].Set != callerSet || sets.Sets[0].Reaction != "" || len(sets.Sets[0].Writes) != 2 {
		t.Fatalf("open sets = %+v, want the caller's holding two writes", sets.Sets)
	}
	// Someone edits B meanwhile, outside the set: the commit fails.
	cur, _ := svc.ObjectGet(ctx, api.ObjectGetInput{ID: "billing.b"})
	edited := cur.Object
	edited.Description = "someone else's"
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "billing", Object: edited}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ChangeSetCommit(ctx, api.ChangeSetCommitInput{Set: callerSet}); !errors.Is(err, api.ErrConflict) {
		t.Fatalf("commit over a moved revision = %v, want ErrConflict", err)
	}
	if _, ok := onDisk(t, cfg, "billing.a"); ok {
		t.Error("A was written by a commit that failed on B")
	}
	if got, _ := onDisk(t, cfg, "billing.b"); got.Description != "someone else's" {
		t.Errorf("B = %q, want the other write kept", got.Description)
	}

	// A fresh set on the current state: both written.
	stageBoth(t, svc, ctx)
	out, err := svc.ChangeSetCommit(ctx, api.ChangeSetCommitInput{Set: callerSet})
	if err != nil || len(out.Writes) != 2 {
		t.Fatalf("commit = %+v, %v", out, err)
	}
	if _, ok := onDisk(t, cfg, "billing.a"); !ok {
		t.Error("the committed create is not on disk")
	}
	if got, _ := onDisk(t, cfg, "billing.b"); got.Description != "the caller's" {
		t.Errorf("the committed update is not on disk: %q", got.Description)
	}
	if sets, _ := svc.ChangeSetList(ctx, api.ChangeSetListInput{}); len(sets.Sets) != 0 {
		t.Errorf("sets after the commit = %+v", sets.Sets)
	}
	// The marker with no open set: written at once.
	b, _ := svc.ObjectGet(ctx, api.ObjectGetInput{ID: "billing.b"})
	b.Object.Description = "after"
	if _, err := svc.ObjectUpdate(api.WithChangeSet(ctx, callerSet), api.ObjectUpdateInput{Module: "billing", Object: b.Object}); err != nil {
		t.Fatal(err)
	}
	if got, _ := onDisk(t, cfg, "billing.b"); got.Description != "after" {
		t.Errorf("a marked write with no open set = %q, want written at once", got.Description)
	}
}

// A person's set: no hook and no event at the stage; at the commit the object
// events go out and B's on_update hook runs once, under the person's cause,
// with nothing held in an outbox.
func TestACallerChangeSetFiresItsHooksAtCommit(t *testing.T) {
	svc, ctx, acts, ev, _ := callerFixture(t)
	stageBoth(t, svc, ctx)
	time.Sleep(30 * time.Millisecond) // what a hook at the stage would need
	if n := acts.callsTo(afterChain); n != 0 {
		t.Fatalf("the on_update hook ran %d time(s) at the stage", n)
	}
	if n := ev.count(); n != 0 {
		t.Fatalf("%d object event(s) at the stage", n)
	}
	if _, err := svc.ChangeSetCommit(ctx, api.ChangeSetCommitInput{Set: callerSet}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the on_update hook runs at the commit", func() bool { return acts.callsTo(afterChain) == 1 })
	time.Sleep(30 * time.Millisecond)
	if n := acts.callsTo(afterChain); n != 1 {
		t.Errorf("the on_update hook ran %d times, want once", n)
	}
	if c := acts.causeOf(afterChain); c.Kind == cause.KindReaction {
		t.Errorf("the hook ran under %+v, want the person's chain", c)
	}
	if len(ev.created.all()) != 1 || len(ev.updated.all()) != 1 {
		t.Errorf("events after the commit: %d created, %d updated — want one each", len(ev.created.all()), len(ev.updated.all()))
	}
	if boxes := svc.outbox().list(); len(boxes) != 0 {
		t.Errorf("outbox = %+v, want nothing held for a caller's set", boxes)
	}
}
