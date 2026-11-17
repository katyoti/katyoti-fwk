// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// : todos migrate --remove deletes a planning file only while
// it still holds what its row was made of - a file written between the read
// and the remove (the hub's own tick, a worker) is kept and named, for a run
// again to bring in.
func TestAPlanningFileChangedSinceItWasReadIsKept(t *testing.T) {
	space := filepath.Join(t.TempDir(), "billing", ".ktw")
	todo := filepath.Join(space, todoDir)
	if err := os.MkdirAll(todo, 0o755); err != nil {
		t.Fatal(err)
	}
	file := func(n, rev string) string {
		return "id: billing.TD-20260101-000000000000-000" + n + "\nmodule: billing\ntitle: t" + n + "\nstate: ready\nrevision: " + rev + "\n"
	}
	same := filepath.Join(todo, "TD-20260101-000000000000-0001.yaml")
	changed := filepath.Join(todo, "TD-20260101-000000000000-0002.yaml")
	for p, body := range map[string]string{same: file("1", "1"), changed: file("2", "1")} {
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	f, err := readSpace(spaceOf{module: "billing", dir: space})
	if err != nil || len(f.open) != 2 {
		t.Fatalf("readSpace = %d open (%v), want 2", len(f.open), err)
	}
	// Written after the read, same size (only the revision's digit changes):
	// a size-and-time check alone could miss it.
	if err := os.WriteFile(changed, []byte(file("2", "2")), 0o644); err != nil {
		t.Fatal(err)
	}

	var recorded []string // : the ids are listed before any file goes
	removed, left, _, err := removeFiles([]filesOf{f}, func(ids []string) error {
		if _, err := os.Stat(same); err != nil {
			t.Errorf("the ids were listed after a file was removed (%v)", err)
		}
		recorded = append(recorded, ids...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(recorded) != 2 {
		t.Fatalf("listed as removed by the migration: %v, want the 2 ids read", recorded)
	}
	if removed != 1 || len(left) != 1 || left[0] != changed {
		t.Fatalf("removeFiles = %d removed, left %v; want 1 removed and the changed file left", removed, left)
	}
	if _, err := os.Stat(changed); err != nil {
		t.Fatalf("the file changed since it was read was removed: %v", err)
	}
	if _, err := os.Stat(same); !os.IsNotExist(err) {
		t.Fatalf("the file unchanged since it was read is still there (%v)", err)
	}
}

// : a planning file read but gone at remove time - a wait
// cleared by its event, an open to-do removed, an open to-do archived - is
// never skipped silently: the store follows the file store (the wait's row
// and the removed to-do's open row go), and an archive's new done file is
// kept and named for a run again, so the move cannot bring back what the file
// store removed while it ran.
func TestMigrateRemoveDropsTheRowOfAFileRemovedDuringTheMove(t *testing.T) {
	space := filepath.Join(t.TempDir(), "billing", ".ktw")
	for _, sub := range []string{todoDir, doneDir, waitingDir} {
		if err := os.MkdirAll(filepath.Join(space, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	todo := func(n string) string {
		return "id: billing.TD-20260101-000000000000-000" + n + "\nmodule: billing\ntitle: t" + n + "\nstate: ready\nrevision: 1\n"
	}
	const (
		removedID  = "billing.TD-20260101-000000000000-0001"
		archivedID = "billing.TD-20260101-000000000000-0002"
		keptID     = "billing.TD-20260101-000000000000-0003"
		waitID     = "billing.WT-20260101-000000000000-0001"
	)
	removedFile := filepath.Join(space, todoDir, fileOf(removedID))
	archivedFile := filepath.Join(space, todoDir, fileOf(archivedID))
	waitFile := filepath.Join(space, waitingDir, fileOf(waitID))
	for p, body := range map[string]string{
		removedFile:  todo("1"),
		archivedFile: todo("2"),
		filepath.Join(space, todoDir, fileOf(keptID)): todo("3"),
		waitFile: "id: " + waitID + "\nmodule: billing\non: billing.RQ-001\nfrom: ops\nwhat: an answer\n",
	} {
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	f, err := readSpace(spaceOf{module: "billing", dir: space})
	if err != nil || len(f.open) != 3 || len(f.waits) != 1 {
		t.Fatalf("readSpace = %d open, %d waits (%v), want 3 and 1", len(f.open), len(f.waits), err)
	}
	ctx := context.Background()
	st := openSQL(t, t.TempDir(), "")
	if _, err := st.MoveInTodos(ctx, f.open, false); err != nil {
		t.Fatal(err)
	}
	if _, err := st.MoveInWaits(ctx, f.waits, true); err != nil {
		t.Fatal(err)
	}

	// Between the move and the remove: the wait is cleared, one to-do removed,
	// one archived (its done file written, its open file removed).
	if err := os.Remove(waitFile); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(removedFile); err != nil {
		t.Fatal(err)
	}
	archived := filepath.Join(space, doneDir, fileOf(archivedID))
	if err := os.WriteFile(archived, []byte("id: "+archivedID+"\nmodule: billing\ntitle: t2\nstate: done\nrevision: 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(archivedFile); err != nil {
		t.Fatal(err)
	}

	removed, left, gone, err := removeFiles([]filesOf{f}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 || len(gone) != 3 {
		t.Fatalf("removeFiles = %d removed, gone %+v; want the one kept to-do removed and the 3 files gone during the move named", removed, gone)
	}
	var unresolved []string
	if err := st.Atomically(ctx, func(ctx context.Context) error {
		_, unresolved, err = dropGone(ctx, st, gone)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 || left[0] != archived || len(unresolved) != 1 || unresolved[0] != archived {
		t.Fatalf("left %v, unresolved %v; want the archive's new done file named in both (kept for a run again: no end marker)", left, unresolved)
	}
	waits, err := st.WaitIDs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if waits[waitID] {
		t.Fatalf("the wait cleared during the move is still in the store: it would come back live")
	}
	rows, err := st.Rows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := rows[removedID]; ok {
		t.Fatalf("the to-do removed during the move is still an open row: %+v", rows[removedID])
	}
	if _, ok := rows[archivedID]; !ok {
		t.Fatalf("the to-do archived during the move lost its row before its done record came in")
	}
	if _, ok := rows[keptID]; !ok {
		t.Fatalf("the to-do whose file was removed by the move lost its row")
	}
}

// : dropGone names a row removed only when one was - a wait
// file gone during the move whose row never was is no repair.
func TestAGoneWaitThatNeverHadARowIsNoRepair(t *testing.T) {
	ctx := context.Background()
	st := openSQL(t, t.TempDir(), "")
	sp := spaceOf{module: "billing", dir: filepath.Join(t.TempDir(), "billing", ".ktw")}
	gone := []goneFile{
		{space: sp, path: filepath.Join(sp.dir, waitingDir, "WT-20260101-000000000000-0009.yaml"), sub: waitingDir, id: "billing.WT-20260101-000000000000-0009"},
		{space: sp, path: filepath.Join(sp.dir, todoDir, "TD-20260101-000000000000-0009.yaml"), sub: todoDir, id: "billing.TD-20260101-000000000000-0009"},
		{space: sp, path: filepath.Join(sp.dir, doneDir, "TD-20260101-000000000000-0008.yaml"), sub: doneDir, id: "billing.TD-20260101-000000000000-0008"},
	}
	var repairs []api.MigrateRepair
	if err := st.Atomically(ctx, func(ctx context.Context) error {
		var err error
		repairs, _, err = dropGone(ctx, st, gone)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(repairs) != 0 {
		t.Fatalf("dropGone over files whose rows never were = %+v, want no repair named", repairs)
	}
}
