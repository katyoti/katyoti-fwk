// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	schedcore "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/internal/core"
)

// : a shipped migration step moves every module's.ktw/todo
// .ktw/done and .ktw/waiting into the SQLite store - files before, rows after;
// a run again changes nothing, a run after a partial one completes it; with
// remove the files go (git history their archive) and a to-do file written
// there again is refused.

// aLeftFile is a to-do file a universe still on files holds before its first
// use: the file store is chosen.
func aLeftFile(t *testing.T, ctx context.Context, cwm cwmapi.CWMService) {
	t.Helper()
	todo := filepath.Join(planning(t, ctx, cwm, "billing"), "todo")
	if err := os.MkdirAll(todo, 0o755); err != nil {
		t.Fatal(err)
	}
	left := "id: billing.TD-20260101-000000000000-0000\nmodule: billing\ntitle: left\nowner: test\nfor: agent\nstart: manual\npriority: P3\nstate: waiting\nrevision: 4\n"
	if err := os.WriteFile(filepath.Join(todo, "TD-20260101-000000000000-0000.yaml"), []byte(left), 0o644); err != nil {
		t.Fatal(err)
	}
}

// filesWorld is a keyed universe still on the planning files: a left to-do
// file, then a to-do put, pulled, reported and collected (a done file) and a
// wait - all written as files.
func filesWorld(t *testing.T, dir string) (context.Context, cwmapi.CWMService, api.TodoService, api.WaitService) {
	t.Helper()
	ctx, cwm, todos, waits := selectionWorld(t, dir, true, func(ctx context.Context, cwm cwmapi.CWMService) { aLeftFile(t, ctx, cwm) })
	if storeOf(t, ctx).SQLite() {
		t.Fatal("a universe holding planning files chose SQLite, want the file store until the migration")
	}
	work(t, ctx, todos, waits)
	put(t, ctx, todos, api.Todo{Module: "billing", Title: "still open", Labels: []string{"increment:IN-1"}})
	return ctx, cwm, todos, waits
}

// fileTodos reads the to-do files of billing's folder sub.
func fileTodos(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, sub string) []api.Todo {
	t.Helper()
	dir := filepath.Join(planning(t, ctx, cwm, "billing"), sub)
	entries, _ := os.ReadDir(dir)
	var out []api.Todo
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var td api.Todo
		if err := marshal.Unmarshal(raw, marshal.YAML, &td); err != nil {
			t.Fatal(err)
		}
		out = append(out, td)
	}
	return out
}

func migrate(t *testing.T, ctx context.Context, todos api.TodoService, in api.MigrateInput) api.MigrateOutput {
	t.Helper()
	out, err := todos.Migrate(ctx, in)
	if err != nil {
		t.Fatalf("migrate %+v: %v (%+v)", in, err, out)
	}
	return out
}

// asJSON is a to-do as the store keeps its record: what it says, whatever its
// repository column.
func asJSON(t *testing.T, td api.Todo) string {
	t.Helper()
	td.Repository = ""
	b, err := json.Marshal(td)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestTheMigrationMovesEveryPlanningFileIn: the files' open, done and waits
// counts are the rows' after the move, a record is the same field by field, a
// run again changes nothing, and the files stay until remove.
func TestTheMigrationMovesEveryPlanningFileIn(t *testing.T) {
	dir := t.TempDir()
	ctx, cwm, todos, _ := filesWorld(t, dir)
	open, done := fileTodos(t, ctx, cwm, "todo"), fileTodos(t, ctx, cwm, "done")
	if len(open) != 2 || len(done) != 1 {
		t.Fatalf("files: %d open, %d done; want 2 and 1", len(open), len(done))
	}

	dry := migrate(t, ctx, todos, api.MigrateInput{DryRun: true})
	if dry.Added != 4 || exists(filepath.Join(dir, "scheduler.db")) {
		t.Fatalf("dry run = %+v (db written: %v), want 4 to add and no store written", dry, exists(filepath.Join(dir, "scheduler.db")))
	}

	got := migrate(t, ctx, todos, api.MigrateInput{})
	want := api.MigratedCounts{Open: 2, Done: 1, Waits: 1}
	if got.Files != want || got.Rows != want || got.Added != 4 || len(got.Missing) != 0 {
		t.Fatalf("migrate = %+v, want files = rows = %+v, 4 added, none missing", got, want)
	}
	if files := yamlOf(planningFiles(t, ctx, cwm)); len(files) != 4 {
		t.Fatalf("files after a move without remove: %v, want the 4 kept", files)
	}

	st, err := schedcore.OpenSQLiteStore(envOf(t, ctx))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range append(open, done...) {
		row, err := st.Get(ctx, f.ID)
		if err != nil {
			t.Fatalf("row of %s: %v", f.ID, err)
		}
		if asJSON(t, row) != asJSON(t, f) {
			t.Fatalf("row of %s differs from its file:\n row  %s\n file %s", f.ID, asJSON(t, row), asJSON(t, f))
		}
	}
	_ = st.Close()

	again := migrate(t, ctx, todos, api.MigrateInput{})
	if again.Added != 0 || again.Updated != 0 || again.Unchanged != 4 || again.Rows != want {
		t.Fatalf("a run again = %+v, want nothing changed (4 unchanged)", again)
	}
}

// TestARunAfterAPartialOneCompletesIt: a to-do moved in open, then archived in
// its files (todo -> done) before the real run, is done in the store after
// the run again - never left active.
func TestARunAfterAPartialOneCompletesIt(t *testing.T) {
	dir := t.TempDir()
	ctx, _, todos, _ := filesWorld(t, dir)
	migrate(t, ctx, todos, api.MigrateInput{})

	const left = "billing.TD-20260101-000000000000-0000"
	if _, err := todos.Archive(ctx, api.ArchiveInput{ID: left, Note: "dropped"}); err != nil {
		t.Fatal(err)
	}
	got := migrate(t, ctx, todos, api.MigrateInput{})
	if got.Updated != 1 || got.Rows != (api.MigratedCounts{Open: 1, Done: 2, Waits: 1}) || len(got.Missing) != 0 {
		t.Fatalf("the run after the archive = %+v, want the left to-do brought to done (1 updated, 1 open, 2 done)", got)
	}
	st, err := schedcore.OpenSQLiteStore(envOf(t, ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if open, err := st.List(ctx, "billing", false); err != nil || len(open) != 1 || open[0].ID == left {
		t.Fatalf("open rows = %+v (%v), want only the still-open one: the archived to-do must not stay active", open, err)
	}
}

// TestTheRemovedFilesNeverComeBack: with remove, the files go and the end
// marker is written; the next run of the universe is on the SQLite store with
// the same to-dos; a to-do file written there again is refused - at the
// store's choice and by a process still on the file store.
func TestTheRemovedFilesNeverComeBack(t *testing.T) {
	dir := t.TempDir()
	ctx, cwm, todos, waits := filesWorld(t, dir)
	space := planning(t, ctx, cwm, "billing")
	got := migrate(t, ctx, todos, api.MigrateInput{Remove: true})
	if got.Removed != 4 || got.Migrated == "" {
		t.Fatalf("migrate --remove = %+v, want 4 files removed and the end recorded", got)
	}
	if files := yamlOf(planningFiles(t, ctx, cwm)); len(files) != 0 {
		t.Fatalf("files left after remove: %v", files)
	}
	// : the emptied folders and the todo folder's lock go
	// with the files - nothing of the file store is left in the tree.
	for _, sub := range []string{"todo", "done", "waiting"} {
		if _, err := os.Stat(filepath.Join(space, sub)); !os.IsNotExist(err) {
			t.Errorf("billing's .ktw/%s after remove (%v): want the emptied folder gone", sub, err)
		}
	}
	// This process chose the file store before the migration: it writes no
	// planning file after it.
	if _, err := todos.Put(ctx, api.PutInput{Todo: api.Todo{Module: "billing", Title: "late", Owner: "test", For: api.ForAgent, Start: api.StartAuto, Priority: "P2", State: api.StateReady, Target: "billing"}}); !errors.Is(err, api.ErrPlanningFileAfterMigration) {
		t.Fatalf("a put on the file store after the migration = %v, want refused", err)
	}
	if _, err := waits.Add(ctx, api.AddWaitInput{Module: "billing", On: "shipping.RQ-002", From: "shipping", What: "x", By: alice}); !errors.Is(err, api.ErrPlanningFileAfterMigration) {
		t.Fatalf("a wait added on the file store after the migration = %v, want refused", err)
	}
	// The next run (a new choice of the store, as a process started now
	// makes): the SQLite store, the same to-dos.
	next := func() (*schedcore.SchedulerStore, *schedcore.Service) {
		st, err := schedcore.OpenSchedulerStore(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = st.Close() })
		return st, schedcore.NewOver(st)
	}
	st2, svc2 := next()
	if !st2.SQLite() {
		t.Fatal("the universe after the migration is not on the SQLite store")
	}
	if l, err := svc2.List(ctx, api.ListInput{Module: "billing"}); err != nil || len(l.Todos) != 2 {
		t.Fatalf("billing's to-dos after the migration = %+v (%v), want the 2 open ones", l, err)
	}

	// A to-do file written there again: refused, named.
	if err := os.MkdirAll(filepath.Join(space, "todo"), 0o755); err != nil { // the emptied folder went with the files
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(space, "todo", "TD-20260102-000000000000-0000.yaml"),
		[]byte("id: billing.TD-20260102-000000000000-0000\nmodule: billing\ntitle: by hand\nstate: ready\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, svc3 := next()
	if _, err := svc3.List(ctx, api.ListInput{Module: "billing"}); !errors.Is(err, api.ErrPlanningFileAfterMigration) {
		t.Fatalf("a to-do file after the migration = %v, want refused", err)
	}
}

func envOf(t *testing.T, ctx context.Context) application.AppEnvironment {
	t.Helper()
	env, err := application.UseService[application.AppEnvironment](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return env
}

// yamlOf keeps the to-do and wait files (the folder's lock file is none).
func yamlOf(files []string) []string {
	var out []string
	for _, f := range files {
		if filepath.Ext(f) == ".yaml" {
			out = append(out, f)
		}
	}
	return out
}

// TestAPlanningFolderNoListedModuleOwnsKeepsTheFiles: a module folder beside
// billing holds to-do files but no module listed owns it - the move names it
// unread and removes nothing (its files would be lost to the store).
func TestAPlanningFolderNoListedModuleOwnsKeepsTheFiles(t *testing.T) {
	dir := t.TempDir()
	ctx, cwm, todos, _ := filesWorld(t, dir)
	space := planning(t, ctx, cwm, "billing")
	ghost := filepath.Join(filepath.Dir(filepath.Dir(space)), "ghost", ".ktw", "todo")
	if err := os.MkdirAll(ghost, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ghost, "TD-20260103-000000000000-0000.yaml"), []byte("id: ghost.TD-20260103-000000000000-0000\nmodule: ghost\ntitle: unlisted\nstate: ready\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := todos.Migrate(ctx, api.MigrateInput{Remove: true})
	if err == nil || len(got.Unread) != 1 || got.Removed != 0 || got.Migrated != "" {
		t.Fatalf("migrate --remove beside an unread folder = %+v (%v), want refused, the folder named unread, nothing removed", got, err)
	}
	if files := yamlOf(planningFiles(t, ctx, cwm)); len(files) != 4 {
		t.Fatalf("billing's files = %v, want the 4 kept", files)
	}
}

// TestALaterRunKeepsTheFirstMigrationsMarker: a migrate
// --remove after the end - bringing in a stray file - keeps the end marker's
// first date and totals, never its own.
func TestALaterRunKeepsTheFirstMigrationsMarker(t *testing.T) {
	dir := t.TempDir()
	ctx, cwm, todos, _ := filesWorld(t, dir)
	first := migrate(t, ctx, todos, api.MigrateInput{Remove: true})
	if first.Migrated == "" {
		t.Fatalf("migrate --remove = %+v, want the end recorded", first)
	}
	marker := filepath.Join(envOf(t, ctx).SchedulerDir(), schedcore.MigratedMarker)
	before, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	space := planning(t, ctx, cwm, "billing")
	if err := os.MkdirAll(filepath.Join(space, "todo"), 0o755); err != nil { // the emptied folder went with the files
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(space, "todo", "TD-20260104-000000000000-0000.yaml"),
		[]byte("id: billing.TD-20260104-000000000000-0000\nmodule: billing\ntitle: stray\nowner: test\nfor: agent\nstart: manual\npriority: P3\nstate: ready\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond) // a later second: a rewritten marker would say so
	later := migrate(t, ctx, todos, api.MigrateInput{Remove: true})
	if later.Removed != 1 || later.Migrated != first.Migrated {
		t.Fatalf("the later run = %+v, want the stray file removed and the first migration's date %s", later, first.Migrated)
	}
	if after, err := os.ReadFile(marker); err != nil || string(after) != string(before) {
		t.Fatalf("the end marker after a later run:\n%s(%v)\nwant the first one kept:\n%s", after, err, before)
	}
}

// oneFileOf is the path and id of the first file of billing's folder sub.
func oneFileOf(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, sub string) (string, string) {
	t.Helper()
	dir := filepath.Join(planning(t, ctx, cwm, "billing"), sub)
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var head struct {
			ID string `yaml:"id"`
		}
		if err := marshal.Unmarshal(raw, marshal.YAML, &head); err != nil || head.ID == "" {
			t.Fatalf("%s: no id (%v)", path, err)
		}
		return path, head.ID
	}
	t.Fatalf("no file in %s", dir)
	return "", ""
}

// rowsOf is the to-do rows and the wait ids the universe's SQLite store holds.
func rowsOf(t *testing.T, ctx context.Context) (map[string]bool, map[string]bool) {
	t.Helper()
	st, err := schedcore.OpenSQLiteStore(envOf(t, ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	held, err := st.HeldRows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	todos, waits := map[string]bool{}, map[string]bool{}
	for _, h := range held {
		if h.Wait {
			waits[h.ID] = true
		} else {
			todos[h.ID] = true
		}
	}
	return todos, waits
}

func repaired(out api.MigrateOutput, id string) bool {
	for _, r := range out.Repairs {
		if strings.Contains(filepath.Base(r.File), strings.TrimPrefix(id, "billing.")) && strings.Contains(r.Repair, "between two runs") {
			return true
		}
	}
	return false
}

// TestARowWhoseFileWasRemovedBetweenTwoRunsGoes: a plain
// run moves the files in; then the file store clears a wait and removes an
// open to-do; the next run - before the end marker, the files the truth -
// drops both rows, names them, and the end is written.
func TestARowWhoseFileWasRemovedBetweenTwoRunsGoes(t *testing.T) {
	dir := t.TempDir()
	ctx, cwm, todos, _ := filesWorld(t, dir)
	migrate(t, ctx, todos, api.MigrateInput{})
	waitFile, waitID := oneFileOf(t, ctx, cwm, "waiting")
	todoFile, todoID := oneFileOf(t, ctx, cwm, "todo")
	for _, p := range []string{waitFile, todoFile} {
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
	}

	got := migrate(t, ctx, todos, api.MigrateInput{Remove: true})
	if got.Migrated == "" || !repaired(got, waitID) || !repaired(got, todoID) {
		t.Fatalf("migrate --remove after a wait and a to-do file were removed = %+v, want both rows' removal named and the end recorded", got)
	}
	rows, waits := rowsOf(t, ctx)
	if waits[waitID] || rows[todoID] {
		t.Fatalf("rows after the run: wait %s kept %v, to-do %s kept %v - want both gone (they would come back live)", waitID, waits[waitID], todoID, rows[todoID])
	}
	if len(rows) != 2 {
		t.Fatalf("to-do rows = %v, want the 2 whose files stayed (one open, one done)", rows)
	}
	if exists(filepath.Join(dir, schedcore.RemovedList)) {
		t.Fatal("the list of removed files outlives the end marker")
	}
}

// TestARowWhoseFileTheMigrationRemovedStays: a file the
// migration itself removed (named in its list) keeps its row on a later run
// before the end - the store holds it now, not the file.
func TestARowWhoseFileTheMigrationRemovedStays(t *testing.T) {
	dir := t.TempDir()
	ctx, cwm, todos, _ := filesWorld(t, dir)
	migrate(t, ctx, todos, api.MigrateInput{})
	todoFile, todoID := oneFileOf(t, ctx, cwm, "todo")
	if err := os.WriteFile(filepath.Join(dir, schedcore.RemovedList), []byte(todoID+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(todoFile); err != nil {
		t.Fatal(err)
	}
	got := migrate(t, ctx, todos, api.MigrateInput{})
	if rows, _ := rowsOf(t, ctx); !rows[todoID] || repaired(got, todoID) {
		t.Fatalf("a run after the migration removed %s's file = %+v; row kept %v - want it kept and no repair", todoID, got, rows[todoID])
	}
}

// TestAfterTheEndMarkerARunDropsNoRow: once the end is
// written the store is the truth and there are no files - a later run (a stray
// file brought in) drops no row.
func TestAfterTheEndMarkerARunDropsNoRow(t *testing.T) {
	dir := t.TempDir()
	ctx, cwm, todos, _ := filesWorld(t, dir)
	migrate(t, ctx, todos, api.MigrateInput{Remove: true})
	beforeRows, beforeWaits := rowsOf(t, ctx)
	space := planning(t, ctx, cwm, "billing")
	if err := os.MkdirAll(filepath.Join(space, "todo"), 0o755); err != nil { // the emptied folder went with the files
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(space, "todo", "TD-20260105-000000000000-0000.yaml"),
		[]byte("id: billing.TD-20260105-000000000000-0000\nmodule: billing\ntitle: stray\nowner: test\nfor: agent\nstart: manual\npriority: P3\nstate: ready\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := migrate(t, ctx, todos, api.MigrateInput{Remove: true})
	rows, waits := rowsOf(t, ctx)
	if len(rows) != len(beforeRows)+1 || len(waits) != len(beforeWaits) {
		t.Fatalf("a run after the end = %+v: %d to-do rows, %d waits; want %d and %d (the stray added, none dropped)", got, len(rows), len(waits), len(beforeRows)+1, len(beforeWaits))
	}
}

// TestAUniverseOnTheStoreWithNoFileKeepsItsRows: a
// universe already on the SQLite store, no planning file, no end marker - its
// rows are the truth: a migrate run drops none.
func TestAUniverseOnTheStoreWithNoFileKeepsItsRows(t *testing.T) {
	dir := t.TempDir()
	ctx, _, todos, waits := selectionWorld(t, dir, true, nil)
	work(t, ctx, todos, waits)
	beforeRows, beforeWaits := rowsOf(t, ctx)
	if len(beforeRows) == 0 {
		t.Fatal("no row on the SQLite store before the run")
	}
	got := migrate(t, ctx, todos, api.MigrateInput{})
	rows, w := rowsOf(t, ctx)
	if len(rows) != len(beforeRows) || len(w) != len(beforeWaits) {
		t.Fatalf("migrate on a universe with no file = %+v: %d to-do rows, %d waits; want %d and %d", got, len(rows), len(w), len(beforeRows), len(beforeWaits))
	}
}
