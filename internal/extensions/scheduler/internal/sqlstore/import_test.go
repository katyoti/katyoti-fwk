// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// : the store's file is refused when damaged, an import is
// keyed by id and safe to run again, and a row carries the repository uuid -
// never a folder or a universe name.

func openWith(t *testing.T, dir string, repoOf func(context.Context, string) (string, error)) *Store {
	t.Helper()
	n := 0
	st, err := Open(dir, Options{
		NewID:  func(module, kind string) string { n++; return fmt.Sprintf("%s.%s-%06d", module, kind, n) },
		Now:    func() string { return time.Now().UTC().Format(time.RFC3339Nano) },
		RepoOf: repoOf,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func repo(uuid string) func(context.Context, string) (string, error) {
	return func(context.Context, string) (string, error) { return uuid, nil }
}

func todosOf(n int) []api.Todo {
	out := make([]api.Todo, n)
	for i := range out {
		out[i] = api.Todo{ID: fmt.Sprintf("billing.TD-%03d", i), Module: "billing", Title: strings.Repeat("x", 64), State: api.StateReady}
	}
	return out
}

func count(t *testing.T, st *Store) int {
	t.Helper()
	var n int
	if err := st.db.QueryRow("SELECT count(*) FROM todos").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestAnImportRunTwiceAddsNothing: an import inserts by to-do id - run again
// it adds none and counts all present; one interrupted after k rows and run
// again completes it; an imported claim's ticket moves the sequence above it.
func TestAnImportRunTwiceAddsNothing(t *testing.T) {
	ctx := context.Background()
	st := openWith(t, t.TempDir(), repo("repo-1"))
	all := todosOf(5)
	all[4].State, all[4].Claim = api.StateClaimed, &api.Claim{Ticket: 41, Until: time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)}

	if got, err := st.ImportTodos(ctx, all[:2], false); err != nil || got != (Imported{Imported: 2}) {
		t.Fatalf("a partial import = %+v (%v)", got, err)
	}
	if got, err := st.ImportTodos(ctx, all, false); err != nil || got != (Imported{Imported: 3, Present: 2}) {
		t.Fatalf("the import run after the partial one = %+v (%v), want 3 imported, 2 present", got, err)
	}
	if got, err := st.ImportTodos(ctx, all, false); err != nil || got != (Imported{Present: 5}) {
		t.Fatalf("the import run again = %+v (%v), want 0 imported, 5 present", got, err)
	}
	if n := count(t, st); n != 5 {
		t.Fatalf("rows = %d, want 5", n)
	}
	read, err := st.Get(ctx, all[0].ID)
	if err != nil || read.Revision != 1 || read.Repository != "repo-1" {
		t.Fatalf("an imported to-do = %+v (%v), want revision 1 of repo-1", read, err)
	}
	next, err := st.Claim(ctx, all[1].ID, func(t *api.Todo) error { t.State, t.Claim = api.StateClaimed, &api.Claim{}; return nil })
	if err != nil || next.Claim.Ticket <= 41 {
		t.Fatalf("a claim after importing ticket 41 = %+v (%v), want a ticket above it", next.Claim, err)
	}

	waits := []api.Wait{{ID: "billing.WT-1", Module: "billing", On: "ops.RQ-1"}, {ID: "billing.WT-2", Module: "billing", On: "ops.RQ-2"}}
	if got, err := st.ImportWaits(ctx, waits[:1]); err != nil || got != (Imported{Imported: 1}) {
		t.Fatalf("a partial wait import = %+v (%v)", got, err)
	}
	if got, err := st.ImportWaits(ctx, waits); err != nil || got != (Imported{Imported: 1, Present: 1}) {
		t.Fatalf("the wait import run again = %+v (%v)", got, err)
	}
	if ws, _ := st.ListWaits(ctx, "billing"); len(ws) != 2 {
		t.Fatalf("waits = %d, want 2", len(ws))
	}
	if _, err := st.ImportTodos(ctx, []api.Todo{{Module: "billing"}}, false); err == nil {
		t.Fatal("a to-do with no id was imported")
	}
}

// TestADamagedStoreIsRefusedNamingIt: a copy of the store whose pages are
// overwritten is refused at open - the error names its path - and is left as
// it was; the original opens.
func TestADamagedStoreIsRefusedNamingIt(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st := openWith(t, dir, repo("repo-1"))
	if _, err := st.ImportTodos(ctx, todosOf(200), false); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()

	b, err := os.ReadFile(filepath.Join(dir, File))
	if err != nil {
		t.Fatal(err)
	}
	if len(b) < 4*4096 {
		t.Fatalf("the store is %d bytes, too small to damage a page", len(b))
	}
	bad := append([]byte(nil), b...)
	for i := 2 * 4096; i < 4*4096; i++ { // pages 3 and 4, the header and the schema page kept
		bad[i] = 0xA5
	}
	copyDir := t.TempDir()
	damagedPath := filepath.Join(copyDir, File)
	if err := os.WriteFile(damagedPath, bad, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = Open(copyDir, Options{NewID: func(m, k string) string { return m }, Now: func() string { return "" }, RepoOf: repo("repo-1")})
	if !errors.Is(err, ErrDamaged) || !strings.Contains(err.Error(), damagedPath) {
		t.Fatalf("opening the damaged copy: %v, want refused as damaged naming %s", err, damagedPath)
	}
	if after, _ := os.ReadFile(damagedPath); !reflect.DeepEqual(after, bad) {
		t.Fatal("the damaged file was changed by the refused open")
	}
	again := openWith(t, dir, repo("repo-1"))
	if n := count(t, again); n != 200 {
		t.Fatalf("the original holds %d rows, want 200", n)
	}
}

// TestRowsFollowTheRepositoryNotTheCheckout: the store opened from two
// checkouts of one repository (each resolving its modules to the same project
// uuid, the second after the project key changed) sees the same rows; a module
// with no project keeps a NULL repository and is read back.
func TestRowsFollowTheRepositoryNotTheCheckout(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	byKey := map[string]string{"katyoti": "uuid-1", "ktw-agents": "uuid-1"}
	checkout := func(key string) func(context.Context, string) (string, error) {
		return func(context.Context, string) (string, error) { return byKey[key], nil }
	}
	first := openWith(t, dir, checkout("ktw-agents"))
	made, err := first.Create(ctx, api.Todo{Module: "billing", Title: "x", State: api.StateReady})
	if err != nil {
		t.Fatal(err)
	}
	_ = first.Close()
	second := openWith(t, dir, checkout("katyoti"))
	if got, err := second.List(ctx, "billing", false); err != nil || len(got) != 1 || got[0].ID != made.ID || got[0].Repository != "uuid-1" {
		t.Fatalf("from the second checkout: %+v (%v), want %s of uuid-1", got, err, made.ID)
	}
	_ = second.Close()

	none := openWith(t, dir, repo(""))
	held, err := none.Create(ctx, api.Todo{Module: "run-1", Title: "a holding-list to-do", State: api.StateReady})
	if err != nil {
		t.Fatal(err)
	}
	var r *string
	if err := none.db.QueryRow("SELECT repository FROM todos WHERE id = ?", held.ID).Scan(&r); err != nil || r != nil {
		t.Fatalf("its repository = %v (%v), want NULL", r, err)
	}
	if got, err := none.List(ctx, "run-1", false); err != nil || len(got) != 1 || got[0].ID != held.ID {
		t.Fatalf("read back: %+v (%v)", got, err)
	}
}

// TestTheRowTablesHoldNoPathNorUniverse: the row tables' columns are this
// fixed list - a new column (a folder, a universe name) is a decision, not a
// drift.
func TestTheRowTablesHoldNoPathNorUniverse(t *testing.T) {
	st := openWith(t, t.TempDir(), repo("repo-1"))
	want := map[string][]string{
		"todos": {"id", "repository", "module", "done", "state", "request", "key", "rev", "ticket", "claim_until", "body", "target", "held_target"},
		"waits": {"id", "repository", "module", "on_what", "body"},
		// a to-do's record at each revision (todos history)
		"todo_revisions": {"id", "rev", "body"},
	}
	for table, cols := range want {
		rows, err := st.db.Query("SELECT name FROM pragma_table_info(?)", table)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for rows.Next() {
			var c string
			_ = rows.Scan(&c)
			got = append(got, c)
		}
		rows.Close()
		if !reflect.DeepEqual(got, cols) {
			t.Fatalf("%s columns = %v, want %v", table, got, cols)
		}
	}
}
