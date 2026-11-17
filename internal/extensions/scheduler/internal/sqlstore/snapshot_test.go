// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// : a snapshot of the store is taken before every change that
// rewrites it in bulk, written by VACUUM INTO and kept only once its
// quick_check passes.

// snapshotsOf lists dir's snapshots, failing on a leftover temporary copy.
func snapshotsOf(t *testing.T, dir string) []string {
	t.Helper()
	got, err := Snapshots(dir)
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, SnapshotDir))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".part") {
			t.Fatalf("a temporary copy was left: %s", e.Name())
		}
	}
	return got
}

// TestASnapshotIsACheckedCopyOfTheStore: the snapshot holds the store's rows,
// passes quick_check, sits in SnapshotDir named by its reason, and the newest
// sorts last.
func TestASnapshotIsACheckedCopyOfTheStore(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	filled(t, dir)
	st := open(t, dir)
	defer st.Close()
	first, err := st.Snapshot(ctx, SnapshotImport)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Create(ctx, api.Todo{Module: "ledger", Title: "after", State: api.StateReady}); err != nil {
		t.Fatal(err)
	}
	second, err := st.Snapshot(ctx, SnapshotMigrate)
	if err != nil {
		t.Fatal(err)
	}
	got := snapshotsOf(t, dir)
	if len(got) != 2 || got[0] != first || got[1] != second {
		t.Fatalf("snapshots = %v, want [%s %s] (the newest last)", got, first, second)
	}
	if filepath.Dir(first) != filepath.Join(dir, SnapshotDir) || !strings.HasSuffix(first, "-"+SnapshotImport+".db") {
		t.Fatalf("snapshot %s: want it in %s, named by its reason", first, SnapshotDir)
	}
	for _, p := range got {
		if err := QuickCheckFile(ctx, p); err != nil {
			t.Fatalf("%s fails quick_check: %v", p, err)
		}
	}
	// The copy holds the rows: opened as a store, the first has the 40 to-dos.
	copyDir := t.TempDir()
	raw, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(copyDir, File), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	restored := open(t, copyDir)
	defer restored.Close()
	if rows, err := restored.List(ctx, "billing", false); err != nil || len(rows) != 40 {
		t.Fatalf("the snapshot holds %d to-dos (%v), want 40", len(rows), err)
	}
}

// TestACorruptedSnapshotIsNotKept: a copy that fails its quick_check is
// deleted, no snapshot is kept, and the error names the fault.
func TestACorruptedSnapshotIsNotKept(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	filled(t, dir)
	page, size := rootPage(t, dir, "todos")
	was := snapshotCopied
	snapshotCopied = func(path string) {
		f, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if _, err := f.WriteAt([]byte(strings.Repeat("\xa5", 64)), (page-1)*size); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { snapshotCopied = was }()
	st := open(t, dir)
	defer st.Close()
	_, err := st.Snapshot(ctx, SnapshotMigrate)
	if err == nil || !strings.Contains(err.Error(), "quick_check") {
		t.Fatalf("snapshot of a copy damaged in between = %v, want refused by its quick_check", err)
	}
	if got := snapshotsOf(t, dir); len(got) != 0 {
		t.Fatalf("a damaged copy was kept: %v", got)
	}
}

// TestAPendingSchemaStepIsSnapshottedFirst: opening a file that holds a schema
// and has a step to run leaves one snapshot, taken before the step (it is at
// the older version); a new file and a file with no pending step leave none.
func TestAPendingSchemaStepIsSnapshottedFirst(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	filled(t, dir) // a new file: every step, no snapshot
	_ = open(t, dir).Close()
	if got := snapshotsOf(t, dir); len(got) != 0 {
		t.Fatalf("a new file and a reopening with no pending step left snapshots: %v", got)
	}

	was := steps
	steps = append(append([]step{}, was...), step{pack: Pack, sql: "CREATE TABLE probe_step (x INTEGER);"})
	defer func() { steps = was }()
	st := open(t, dir)
	_ = st.Close()
	got := snapshotsOf(t, dir)
	if len(got) != 1 || !strings.HasSuffix(got[0], "-"+SnapshotSchema+".db") {
		t.Fatalf("snapshots after a pending step = %v, want one schema snapshot", got)
	}
	if err := QuickCheckFile(ctx, got[0]); err != nil {
		t.Fatalf("the schema snapshot fails quick_check: %v", err)
	}
	snap := openRaw(t, got[0])
	defer snap.Close()
	var v int
	if err := snap.QueryRow("PRAGMA user_version").Scan(&v); err != nil || v != len(was) {
		t.Fatalf("the snapshot is at version %d (%v), want %d: taken before the step", v, err, len(was))
	}
}

// openRaw opens a database file read-only, outside the store.
func openRaw(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	return db
}
