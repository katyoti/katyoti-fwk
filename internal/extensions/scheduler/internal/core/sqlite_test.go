// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/internal/sqlstore"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// : the scheduler's store is a SQLite database under
// SchedulerDir(): every write carries the revision it read and a stale one is
// refused, every claim takes a fencing ticket and a superseded holder's write
// is refused, a claim ends at its ticket's end, and a pull is one transaction
// that holds module exclusivity.

// openSQL opens a SQLite store in dir whose modules all belong to repository
// repo.
func openSQL(t *testing.T, dir, repo string) *sqlstore.Store {
	t.Helper()
	st, err := sqlstore.Open(dir, sqlstore.Options{NewID: idOf, Now: now, RepoOf: func(context.Context, string) (string, error) { return repo, nil }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// adapters are the store port's adapters, each on a fresh place.
func adapters(t *testing.T) map[string]Store {
	return map[string]Store{"file": testFileStore(t), "sqlite": openSQL(t, t.TempDir(), "repo-1")}
}

func claimAs(by cwmapi.Actor, until string) func(*api.Todo) error {
	return func(t *api.Todo) error {
		if !pullable(*t) {
			return errTaken
		}
		t.State, t.Claim = api.StateClaimed, &api.Claim{By: by, At: now(), Until: until}
		return nil
	}
}

var (
	alice = cwmapi.Actor{ID: "alice", Kind: "agent"}
	bob   = cwmapi.Actor{ID: "bob", Kind: "agent"}
)

func passed() string { return time.Now().Add(-time.Second).UTC().Format(time.RFC3339Nano) }

// TestEveryStoreAdapterRefusesAStaleRevisionAndASupersededTicket: on both
// adapters a to-do is created at revision 1 and put once at a key; a write at
// a passed revision is refused stale; a claim takes a ticket a later claim
// supersedes, the superseded holder's write refused, a renewal keeping the
// ticket; archived, it is read from done, then removed.
func TestEveryStoreAdapterRefusesAStaleRevisionAndASupersededTicket(t *testing.T) {
	for name, st := range adapters(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			made, err := st.Create(ctx, api.Todo{Module: "billing", Title: "invoice", Owner: "planification", For: api.ForAgent, Start: api.StartAuto, Priority: "P1", State: api.StateReady, Key: "k1", Work: map[string]any{"step": "implement"}})
			if err != nil || made.Revision != 1 || !strings.HasPrefix(made.ID, "billing.TD-") {
				t.Fatalf("created %+v (%v)", made, err)
			}
			if again, err := st.Create(ctx, api.Todo{Module: "billing", Title: "again", Key: "k1"}); err != nil || again.ID != made.ID {
				t.Fatalf("a second create at key k1 = %s (%v), want %s", again.ID, err, made.ID)
			}
			read, err := st.Get(ctx, made.ID)
			if err != nil || read.Work["step"] != "implement" {
				t.Fatalf("read %+v (%v)", read, err)
			}
			read.Title = "renamed"
			if put, err := st.Put(ctx, read); err != nil || put.Revision != 2 {
				t.Fatalf("put at revision 1 = %d (%v), want 2", put.Revision, err)
			}
			if _, err := st.Put(ctx, read); !errors.Is(err, api.ErrStaleRevision) || !errors.Is(err, cwmapi.ErrConflict) {
				t.Fatalf("put at the passed revision: %v, want stale", err)
			}

			first, err := st.Claim(ctx, made.ID, claimAs(alice, passed())) // alice's lease already ended
			if err != nil || first.Claim.Ticket == 0 {
				t.Fatalf("alice's claim = %+v (%v), want a ticket", first.Claim, err)
			}
			second, err := st.Claim(ctx, made.ID, claimAs(bob, lease(60)))
			if err != nil || second.Claim.Ticket <= first.Claim.Ticket {
				t.Fatalf("bob's claim = %+v (%v), want a ticket above %d", second.Claim, err, first.Claim.Ticket)
			}
			late, _ := st.Get(ctx, made.ID)
			late.Claim = first.Claim
			if _, err := st.Put(ctx, late); !errors.Is(err, api.ErrSupersededTicket) || !errors.Is(err, cwmapi.ErrConflict) {
				t.Fatalf("alice writes with her superseded ticket: %v, want superseded", err)
			}
			mine, _ := st.Get(ctx, made.ID)
			mine.Note = "bob's write"
			if got, err := st.Put(ctx, mine); err != nil || got.Claim.Ticket != second.Claim.Ticket {
				t.Fatalf("bob writes with his ticket: %+v (%v)", got.Claim, err)
			}
			renewed, err := st.Update(ctx, made.ID, func(t *api.Todo) error { t.Claim.Until = lease(120); return nil })
			if err != nil || renewed.Claim.Ticket != second.Claim.Ticket {
				t.Fatalf("renewed = %+v (%v), want ticket %d kept", renewed.Claim, err, second.Claim.Ticket)
			}

			if _, err := st.Archive(ctx, made.ID, func(t *api.Todo) error { t.Archived = now(); return nil }); err != nil {
				t.Fatal(err)
			}
			if got, err := st.Get(ctx, made.ID); err != nil || got.Archived == "" {
				t.Fatalf("archived = %+v (%v)", got, err)
			}
			if _, err := st.Update(ctx, made.ID, func(*api.Todo) error { return nil }); !errors.Is(err, cwmapi.ErrNotFound) {
				t.Fatalf("update of a done to-do: %v, want not found", err)
			}
			if done, _ := st.List(ctx, "billing", true); len(done) != 1 {
				t.Fatalf("done = %d, want 1", len(done))
			}
			if err := st.RemoveDone(ctx, made.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := st.Get(ctx, made.ID); !errors.Is(err, cwmapi.ErrNotFound) {
				t.Fatalf("removed: %v, want not found", err)
			}

			mk := func(id string) api.Wait {
				return api.Wait{ID: id, Module: "billing", On: "ledger.RQ-001", What: "close"}
			}
			w, err := st.AddWait(ctx, "billing", "ledger.RQ-001", mk)
			if err != nil || !strings.HasPrefix(w.ID, "billing.WT-") {
				t.Fatalf("wait %+v (%v)", w, err)
			}
			if again, _ := st.AddWait(ctx, "billing", "ledger.RQ-001", mk); again.ID != w.ID {
				t.Fatalf("one wait per thing awaited: %s, want %s", again.ID, w.ID)
			}
			w.Reminded = now()
			if err := st.PutWait(ctx, w); err != nil {
				t.Fatal(err)
			}
			if all, _ := st.ListWaits(ctx, "billing"); len(all) != 1 || all[0].Reminded == "" {
				t.Fatalf("waits = %+v", all)
			}
			if err := st.RemoveWait(ctx, w.ID); err != nil {
				t.Fatal(err)
			}
			if err := st.RemoveWait(ctx, w.ID); !errors.Is(err, cwmapi.ErrNotFound) {
				t.Fatalf("removed twice: %v, want not found", err)
			}
		})
	}
}

// TestAClaimEndsAtItsTicketsEnd: a claim whose until has not passed is its
// holder's - another claim is refused; once it passed, the to-do is anyone's
// again and the next claim supersedes the first's ticket.
func TestAClaimEndsAtItsTicketsEnd(t *testing.T) {
	for name, st := range adapters(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			td, err := st.Create(ctx, api.Todo{Module: "billing", Title: "x", State: api.StateReady, For: api.ForAgent})
			if err != nil {
				t.Fatal(err)
			}
			held, err := st.Claim(ctx, td.ID, claimAs(alice, lease(60)))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := st.Claim(ctx, td.ID, claimAs(bob, lease(60))); !errors.Is(err, errTaken) {
				t.Fatalf("bob claims alice's held to-do: %v, want taken", err)
			}
			if _, err := st.Update(ctx, td.ID, func(t *api.Todo) error { t.Claim.Until = passed(); return nil }); err != nil {
				t.Fatal(err)
			}
			got, err := st.Claim(ctx, td.ID, claimAs(bob, lease(60)))
			if err != nil || got.Claim.By.ID != bob.ID || got.Claim.Ticket <= held.Claim.Ticket {
				t.Fatalf("bob after alice's ticket ended = %+v (%v)", got.Claim, err)
			}
		})
	}
}

// TestTheSQLiteStoreKeysRowsByRepository: two repositories' modules of one
// name keep their rows and their locks apart.
func TestTheSQLiteStoreKeysRowsByRepository(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	one, two := openSQL(t, dir, "repo-1"), openSQL(t, dir, "repo-2")
	a, err := one.Create(ctx, api.Todo{Module: "billing", Title: "one's", State: api.StateReady, Key: "k"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := two.Create(ctx, api.Todo{Module: "billing", Title: "two's", State: api.StateReady, Key: "k"})
	if err != nil || b.ID == a.ID {
		t.Fatalf("the same key in another repository = %s (%v), want its own to-do", b.ID, err)
	}
	if got, _ := one.List(ctx, "billing", false); len(got) != 1 || got[0].ID != a.ID || got[0].Repository != "repo-1" {
		t.Fatalf("repo-1's billing = %+v", got)
	}
	l := &locks{}
	l.takeIn("repo-1", nil, []string{"billing"})
	if why := l.conflictIn("repo-2", nil, []string{"billing"}); why != "" {
		t.Fatalf("repo-2's billing waits on repo-1's: %s", why)
	}
	if why := l.conflictIn("repo-1", []string{"billing"}, nil); why == "" {
		t.Fatal("repo-1's billing read while written")
	}
}

// TestTwoPullsNeverBothWinAThirdModule: two pulls in different queues - two
// handles on one store file, as two processes - each want a to-do that writes
// a third module; both have read the queue before either claims, yet exactly
// one wins: the pull is one transaction.
func TestTwoPullsNeverBothWinAThirdModule(t *testing.T) {
	ctx, cwm := cwmapp.New(t, cwmapp.Pack(fstest.MapFS{"object-types/module.yaml": {Data: []byte("name: module\ncapabilities: [module]\n")}}))
	for _, m := range []string{"billing", "shipping", "ledger"} {
		cwmapp.Module(t, ctx, cwm, m)
	}
	dir := t.TempDir()
	first, second := NewOver(openSQL(t, dir, "repo-1")), NewOver(openSQL(t, dir, "repo-1"))
	for _, m := range []string{"billing", "shipping"} {
		if _, err := first.Put(ctx, api.PutInput{Todo: api.Todo{Module: m, Target: m, Title: "post to the ledger", Owner: "test", For: api.ForAgent, Start: api.StartAuto, Priority: "P1", State: api.StateReady, Writes: []string{"ledger"}}}); err != nil {
			t.Fatal(err)
		}
	}

	// Each pull waits, after its queue, for the other to have read its own -
	// or, when the other cannot read yet (the store holds it out), a while.
	var arrived atomic.Int32
	defer func() { afterQueue = nil }()
	afterQueue = func() {
		arrived.Add(1)
		for end := time.Now().Add(400 * time.Millisecond); arrived.Load() < 2 && time.Now().Before(end); {
			time.Sleep(5 * time.Millisecond)
		}
	}
	var wg sync.WaitGroup
	won := make([]*api.Todo, 2)
	errs := make([]error, 2)
	for i, pull := range []struct {
		svc    *Service
		by     cwmapi.Actor
		module string
	}{{first, alice, "billing"}, {second, bob, "shipping"}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := pull.svc.Pull(ctx, api.PullInput{By: pull.by, Filter: api.Filter{Module: pull.module}})
			won[i], errs[i] = out.Todo, err
		}()
	}
	wg.Wait()
	wins := 0
	for i := range won {
		if errs[i] != nil {
			t.Fatalf("pull %d: %v", i, errs[i])
		}
		if won[i] != nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("%d pulls took a to-do writing ledger, want exactly 1 (the pull is one transaction)", wins)
	}
}

// schedulerEnv is an environment whose scheduler folder is dir.
type schedulerEnv struct {
	application.AppEnvironment
	dir string
}

func (e schedulerEnv) SchedulerDir() string { return e.dir }

// TestTheSQLiteStoreOpensInSchedulerDir: the scheduler's store is the file in
// its environment's SchedulerDir() and nowhere else; an environment naming no
// scheduler folder is refused.
func TestTheSQLiteStoreOpensInSchedulerDir(t *testing.T) {
	dir := t.TempDir()
	st, err := OpenSQLiteStore(schedulerEnv{dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if want := filepath.Join(dir, sqlstore.File); st.Path() != want {
		t.Fatalf("the store's file = %s, want %s", st.Path(), want)
	}
	if _, err := OpenSQLiteStore(schedulerEnv{}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("no scheduler folder: %v, want refused", err)
	}
}
