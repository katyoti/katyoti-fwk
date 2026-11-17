// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// filled is a closed store of dir holding a few to-dos (its WAL checkpointed
// into the file by the close).
func filled(t *testing.T, dir string) {
	t.Helper()
	ctx := context.Background()
	st := open(t, dir)
	for i := 0; i < 40; i++ {
		if _, err := st.Create(ctx, api.Todo{Module: "billing", Title: strings.Repeat("x", 200), State: api.StateReady}); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
}

// rootPage is the root page of a table or index of the closed store of dir,
// and the file's page size.
func rootPage(t *testing.T, dir, name string) (page, size int64) {
	t.Helper()
	st := open(t, dir)
	defer st.Close()
	if err := st.db.QueryRow("SELECT rootpage FROM sqlite_master WHERE name = ?", name).Scan(&page); err != nil {
		t.Fatal(err)
	}
	if err := st.db.QueryRow("PRAGMA page_size").Scan(&size); err != nil {
		t.Fatal(err)
	}
	return page, size
}

// TestOpenRunsTheQuickCheckOnly: a store whose index no longer matches its
// table - a fault only the full integrity_check sees (quick_check does not
// compare an index with its table) - still opens: Open runs quick_check, not
// the full check; the doctor's full check names the fault and its repair.
func TestOpenRunsTheQuickCheckOnly(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	filled(t, dir)
	st := open(t, dir)
	// The index's declared columns change under its stored entries: every
	// entry of todos_of_module no longer matches the row it indexes.
	for _, q := range []string{
		"PRAGMA writable_schema = ON",
		"UPDATE sqlite_master SET sql = 'CREATE INDEX todos_of_module ON todos(state, module, done)' WHERE name = 'todos_of_module'",
		"PRAGMA writable_schema = OFF",
	} {
		if _, err := st.db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	_ = st.Close()

	again, err := Open(dir, testOptions())
	if err != nil {
		t.Fatalf("open of a store only the full check faults: %v, want opened (quick_check only)", err)
	}
	_ = again.Close()

	_, found, faults, err := Doctor(ctx, dir)
	if err != nil || !found {
		t.Fatalf("doctor: found %v (%v)", found, err)
	}
	if len(faults) == 0 {
		t.Fatal("doctor: no fault, want the index's entries named")
	}
	for _, f := range faults {
		if f.Fault == "" || !strings.Contains(f.Repair, "todos import") {
			t.Fatalf("fault %+v: want the fault named with its repair", f)
		}
	}
	if !strings.Contains(faults[0].Fault, "todos_of_module") {
		t.Fatalf("fault %q: want the index named", faults[0].Fault)
	}
}

// TestADamagedPageIsRefusedOnOpenAndNamedByTheDoctor: a store copy whose table
// page is overwritten is refused by Open (ErrDamaged, naming the file and its
// repair), left as it is; the doctor names the fault and its repair. A sound
// store gives the doctor no fault, a folder with no store none either.
func TestADamagedPageIsRefusedOnOpenAndNamedByTheDoctor(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	filled(t, dir)

	if path, found, faults, err := Doctor(ctx, dir); err != nil || !found || len(faults) != 0 || path != filepath.Join(dir, File) {
		t.Fatalf("doctor of a sound store: %s found %v faults %v (%v), want none", path, found, faults, err)
	}
	if _, found, faults, err := Doctor(ctx, t.TempDir()); err != nil || found || len(faults) != 0 {
		t.Fatalf("doctor of no store: found %v faults %v (%v)", found, faults, err)
	}

	page, size := rootPage(t, dir, "todos")
	path := filepath.Join(dir, File)
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Garbage over the table's root page: its page type and cell pointers.
	junk := []byte(strings.Repeat("\xa5", 64))
	if _, err := f.WriteAt(junk, (page-1)*size); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	_, err = Open(dir, testOptions())
	if !errors.Is(err, ErrDamaged) || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "todos import") {
		t.Fatalf("open of a damaged store: %v, want ErrDamaged naming %s and its repair", err, path)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatalf("the damaged file was changed by the refused open (%v)", err)
	}

	_, found, faults, err := Doctor(ctx, dir)
	if err != nil || !found || len(faults) == 0 {
		t.Fatalf("doctor of a damaged store: found %v faults %v (%v), want the fault named", found, faults, err)
	}
	if faults[0].Fault == "" || !strings.Contains(faults[0].Repair, "todos import") {
		t.Fatalf("fault %+v: want the fault named with its repair", faults[0])
	}
}

// testOptions are the options a reopening of a test store takes.
func testOptions() Options {
	return Options{
		NewID:  func(module, kind string) string { return module + "." + kind },
		Now:    func() string { return "" },
		RepoOf: func(context.Context, string) (string, error) { return "repo-1", nil },
	}
}
