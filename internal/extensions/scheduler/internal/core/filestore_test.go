// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// : the scheduler reads and writes its to-dos, their claims and
// the waits through one store port; the file store is its first adapter.

func testFileStore(t *testing.T) Store {
	t.Helper()
	space := t.TempDir()
	return &fileStore{spaceOf: func(context.Context, string) (string, error) { return space, nil }}
}

// TestTheStorePortKeepsToDosAtTheirRevision drives a to-do through the port:
// created at revision 1, put once at a key, written at the revision read - a
// stale one refused - claimed with a ticket a later claim supersedes, archived
// to done and removed.
func TestTheStorePortKeepsToDosAtTheirRevision(t *testing.T) {
	ctx := context.Background()
	st := testFileStore(t)
	alice := cwmapi.Actor{ID: "alice", Kind: "agent"}
	bob := cwmapi.Actor{ID: "bob", Kind: "agent"}

	made, err := st.Create(ctx, api.Todo{Module: "billing", Title: "invoice", Owner: "planification", For: api.ForAgent, Start: api.StartAuto, Priority: "P1", State: api.StateReady, Key: "k1"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(made.ID, "billing.TD-") || made.Made == "" || made.Revision != 1 {
		t.Fatalf("created %+v: want a billing id, a made time and revision 1", made)
	}
	if again, err := st.Create(ctx, api.Todo{Module: "billing", Title: "invoice again", Key: "k1"}); err != nil || again.ID != made.ID {
		t.Fatalf("a second create at key k1 = %s (%v), want the open to-do %s", again.ID, err, made.ID)
	}
	if got, err := st.List(ctx, "billing", false); err != nil || len(got) != 1 {
		t.Fatalf("scheduled = %d to-dos (%v), want 1", len(got), err)
	}

	// Written at the revision read; the same read written again is stale.
	read, err := st.Get(ctx, made.ID)
	if err != nil {
		t.Fatal(err)
	}
	read.Title = "invoice, renamed"
	put, err := st.Put(ctx, read)
	if err != nil || put.Revision != 2 {
		t.Fatalf("put at revision 1 = revision %d (%v), want 2", put.Revision, err)
	}
	read.Title = "a late writer"
	if _, err := st.Put(ctx, read); !errors.Is(err, api.ErrStaleRevision) || !errors.Is(err, cwmapi.ErrConflict) {
		t.Fatalf("put at the passed revision 1: %v, want a stale-revision conflict", err)
	}

	// A claim takes a ticket; a later claim supersedes it.
	claimAs := func(by cwmapi.Actor) func(*api.Todo) error {
		return func(t *api.Todo) error {
			t.State, t.Claim = api.StateClaimed, &api.Claim{By: by, At: now(), Until: lease(1)}
			return nil
		}
	}
	first, err := st.Claim(ctx, made.ID, claimAs(alice))
	if err != nil || first.Claim == nil || first.Claim.Ticket != first.Revision || first.Revision != 3 {
		t.Fatalf("alice's claim = %+v (%v), want ticket 3 at revision 3", first.Claim, err)
	}
	second, err := st.Claim(ctx, made.ID, claimAs(bob)) // alice's lease passed: bob takes it
	if err != nil || second.Claim.Ticket <= first.Claim.Ticket {
		t.Fatalf("bob's claim = %+v (%v), want a higher ticket than %d", second.Claim, err, first.Claim.Ticket)
	}
	late := first
	late.Note = "alice's late write"
	if _, err := st.Put(ctx, late); !errors.Is(err, api.ErrStaleRevision) {
		t.Fatalf("alice writes her old read: %v, want stale", err)
	}
	late, _ = st.Get(ctx, made.ID)
	late.Claim = first.Claim // fresh revision, alice's superseded ticket
	if _, err := st.Put(ctx, late); !errors.Is(err, api.ErrSupersededTicket) || !errors.Is(err, cwmapi.ErrConflict) {
		t.Fatalf("alice writes with her superseded ticket: %v, want a superseded-ticket conflict", err)
	}
	mine, _ := st.Get(ctx, made.ID)
	mine.Note = "bob's write"
	if got, err := st.Put(ctx, mine); err != nil || got.Claim.Ticket != second.Claim.Ticket {
		t.Fatalf("bob writes with his ticket: %+v (%v), want it kept", got.Claim, err)
	}
	// An update keeps the claim's ticket; only a claim gives one.
	renewed, err := st.Update(ctx, made.ID, func(t *api.Todo) error { t.Claim.Until = lease(60); return nil })
	if err != nil || renewed.Claim.Ticket != second.Claim.Ticket {
		t.Fatalf("renewed = %+v (%v), want ticket %d kept", renewed.Claim, err, second.Claim.Ticket)
	}
	if _, err := st.Update(ctx, "billing.TD-nothing", func(*api.Todo) error { return nil }); !errors.Is(err, cwmapi.ErrNotFound) {
		t.Fatalf("update of a to-do not scheduled: %v, want not found", err)
	}

	// Archived to done, then removed.
	refuse := errors.New("held")
	if _, err := st.Archive(ctx, made.ID, func(*api.Todo) error { return refuse }); !errors.Is(err, refuse) {
		t.Fatalf("an archive its check refuses: %v", err)
	}
	if _, err := st.Archive(ctx, made.ID, func(t *api.Todo) error { t.Archived = now(); return nil }); err != nil {
		t.Fatal(err)
	}
	if got, err := st.Get(ctx, made.ID); err != nil || got.Archived == "" {
		t.Fatalf("archived to-do = %+v (%v), want it read from done", got, err)
	}
	if done, _ := st.List(ctx, "billing", true); len(done) != 1 {
		t.Fatalf("done = %d, want 1", len(done))
	}
	if open, _ := st.List(ctx, "billing", false); len(open) != 0 {
		t.Fatalf("scheduled after archive = %d, want 0", len(open))
	}
	if err := st.RemoveDone(ctx, made.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Get(ctx, made.ID); !errors.Is(err, cwmapi.ErrNotFound) {
		t.Fatalf("removed to-do: %v, want not found", err)
	}
}

// TestTheStorePortKeepsOneWaitPerThingAwaited: a wait is kept once per thing
// awaited, rewritten, and removed once.
func TestTheStorePortKeepsOneWaitPerThingAwaited(t *testing.T) {
	ctx := context.Background()
	st := testFileStore(t)
	mk := func(what string) func(string) api.Wait {
		return func(id string) api.Wait {
			return api.Wait{ID: id, Module: "billing", On: "ledger.RQ-001", From: "ledger", What: what, Since: now(), Deadline: now()}
		}
	}
	w, err := st.AddWait(ctx, "billing", "ledger.RQ-001", mk("the ledger's close"))
	if err != nil || !strings.HasPrefix(w.ID, "billing.WT-") {
		t.Fatalf("wait = %+v (%v)", w, err)
	}
	if again, _ := st.AddWait(ctx, "billing", "ledger.RQ-001", mk("asked twice")); again.ID != w.ID || again.What != "the ledger's close" {
		t.Fatalf("the same thing awaited twice = %+v, want %s kept", again, w.ID)
	}
	w.Reminded = now()
	if err := st.PutWait(ctx, w); err != nil {
		t.Fatal(err)
	}
	if all, _ := st.ListWaits(ctx, "billing"); len(all) != 1 || all[0].Reminded == "" {
		t.Fatalf("waits = %+v, want the one, reminded", all)
	}
	if err := st.RemoveWait(ctx, w.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.RemoveWait(ctx, w.ID); !errors.Is(err, cwmapi.ErrNotFound) {
		t.Fatalf("a wait removed twice: %v, want not found", err)
	}
}
