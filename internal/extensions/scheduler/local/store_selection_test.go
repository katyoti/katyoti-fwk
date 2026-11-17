// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	schedcore "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/internal/core"
	schedlocal "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// : an application's scheduler runs on one store, chosen at its
// first use - the SQLite store in SchedulerDir() on a universe that holds no
// planning file and whose modules belong to a project with a uuid, the file
// store otherwise - and the to-dos, the waits and export/import share it.

// dirEnv is the universe's environment with its scheduler folder in dir.
type dirEnv struct {
	application.AppEnvironment
	dir string
}

func (e dirEnv) SchedulerDir() string { return e.dir }

// selectionWorld is a universe whose scheduler folder is schedDir, its project
// given a uuid when withUUID, with the module billing; before runs ahead of the
// scheduler's first use. It answers the context, the to-dos and the waits.
func selectionWorld(t *testing.T, schedDir string, withUUID bool, before func(ctx context.Context, cwm cwmapi.CWMService)) (context.Context, cwmapi.CWMService, api.TodoService, api.WaitService) {
	t.Helper()
	return selectionWorldOf(t, schedDir, withUUID, []string{"billing"}, before)
}

// selectionWorldOf is selectionWorld with the modules given (none: a universe
// with no keyed module).
func selectionWorldOf(t *testing.T, schedDir string, withUUID bool, mods []string, before func(ctx context.Context, cwm cwmapi.CWMService), extra ...application.Extension) (context.Context, cwmapi.CWMService, api.TodoService, api.WaitService) {
	t.Helper()
	exts := []application.Extension{
		func(init *application.ApplicationInit) {
			init.AddContribute("rq035", ".env", func(app *application.Application) error {
				e, _ := application.AppUseService[application.AppEnvironment](app)
				application.PutService[application.AppEnvironment](app, "default", dirEnv{AppEnvironment: e, dir: schedDir})
				return nil
			})
		},
		cwmapp.Pack(fstest.MapFS{"object-types/module.yaml": {Data: []byte("name: module\ncapabilities: [module]\n")}}),
		schedlocal.Extension(),
		func(init *application.ApplicationInit) {
			init.AddContribute("rq035", ".collector", func(app *application.Application) error {
				return schedlocal.ContributeCollector(app, api.Collector{Owner: "test", Collect: func(ctx context.Context, td api.Todo) error {
					svc, err := application.UseService[api.TodoService](ctx)
					if err != nil {
						return err
					}
					_, err = svc.Archive(ctx, api.ArchiveInput{ID: td.ID, Note: "collected"})
					return err
				}})
			})
		},
	}
	ctx, cwm := cwmapp.New(t, append(exts, extra...)...)
	// Closed before its folders are removed: the SQLite handle released.
	t.Cleanup(func() { appcore.CloseApplicationScope(ctx) })
	if withUUID {
		if _, err := cwm.ProjectInit(ctx, cwmapi.ProjectInitInput{Name: "acme", Title: "Acme"}); err != nil {
			t.Fatalf("ProjectInit: %v", err)
		}
	}
	for _, m := range mods {
		cwmapp.Module(t, ctx, cwm, m)
	}
	if before != nil {
		before(ctx, cwm)
	}
	todos, err := application.UseService[api.TodoService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	waits, err := application.UseService[api.WaitService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, cwm, todos, waits
}

// work puts, pulls and reports a to-do of billing and records a wait of it.
func work(t *testing.T, ctx context.Context, todos api.TodoService, waits api.WaitService) {
	t.Helper()
	td := put(t, ctx, todos, api.Todo{Module: "billing", Title: "implement", Writes: []string{"billing"}})
	got, err := todos.Pull(ctx, api.PullInput{By: alice, Filter: api.Filter{Module: "billing"}})
	if err != nil || got.Todo == nil || got.Todo.ID != td.ID {
		t.Fatalf("pull = %+v (%v), want %s", got, err, td.ID)
	}
	if _, err := todos.Report(ctx, api.ReportInput{ID: td.ID, Result: api.Result{Status: api.StatusSuccess, Summary: "built", By: alice}}); err != nil {
		t.Fatal(err)
	}
	if done, err := todos.Get(ctx, api.GetInput{ID: td.ID}); err != nil || done.Todo.Archived == "" {
		t.Fatalf("the reported to-do = %+v (%v), want collected to done", done.Todo, err)
	}
	if _, err := waits.Add(ctx, api.AddWaitInput{Module: "billing", On: "shipping.RQ-001", From: "shipping", What: "the carrier", By: alice}); err != nil {
		t.Fatal(err)
	}
	if ws, err := waits.List(ctx, api.ListWaitsInput{Module: "billing"}); err != nil || len(ws.Waits) != 1 {
		t.Fatalf("billing's waits = %+v (%v), want the one added", ws, err)
	}
}

// planningFiles is the files of billing's to-do, done and waits folders.
func planningFiles(t *testing.T, ctx context.Context, cwm cwmapi.CWMService) []string {
	t.Helper()
	space := planning(t, ctx, cwm, "billing")
	var out []string
	for _, sub := range []string{"todo", "done", "waiting"} {
		entries, _ := os.ReadDir(filepath.Join(space, sub))
		for _, e := range entries {
			out = append(out, filepath.Join(sub, e.Name()))
		}
	}
	return out
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }

// TestAUniverseWithNothingToMigrateRunsOnTheSQLiteStore: no planning file, a
// project with a uuid - the to-dos and the waits land in SchedulerDir()/
// scheduler.db, no file in the module's planning space, and export/import
// answer instead of refusing the file store.
func TestAUniverseWithNothingToMigrateRunsOnTheSQLiteStore(t *testing.T) {
	dir := t.TempDir()
	ctx, cwm, todos, waits := selectionWorld(t, dir, true, nil)
	work(t, ctx, todos, waits)
	if !exists(filepath.Join(dir, "scheduler.db")) {
		t.Fatalf("no scheduler.db in %s: the verbs did not run on the SQLite store", dir)
	}
	if files := planningFiles(t, ctx, cwm); len(files) != 0 {
		t.Fatalf("planning files written on the SQLite store: %v", files)
	}
	exp, err := todos.Export(ctx, api.ExportInput{})
	if err != nil {
		t.Fatalf("export on the SQLite store: %v", err)
	}
	if len(exp.Refs)+len(exp.Skipped) == 0 {
		t.Fatalf("export = %+v, want billing's repository exported or said skipped", exp)
	}
	if _, err := todos.Import(ctx, api.ImportInput{}); err != nil {
		t.Fatalf("import on the SQLite store: %v", err)
	}
}

// TestAUniverseHoldingPlanningFilesKeepsTheFileStore: one .ktw/todo file
// written before the first use keeps the file store - the work lands in files,
// no scheduler.db is created.
func TestAUniverseHoldingPlanningFilesKeepsTheFileStore(t *testing.T) {
	dir := t.TempDir()
	ctx, cwm, todos, waits := selectionWorld(t, dir, true, func(ctx context.Context, cwm cwmapi.CWMService) {
		todo := filepath.Join(planning(t, ctx, cwm, "billing"), "todo")
		if err := os.MkdirAll(todo, 0o755); err != nil {
			t.Fatal(err)
		}
		left := "id: billing.TD-20260101-000000000000-0000\nmodule: billing\ntitle: left\nowner: test\nfor: agent\nstart: manual\npriority: P3\nstate: waiting\n"
		if err := os.WriteFile(filepath.Join(todo, "TD-20260101-000000000000-0000.yaml"), []byte(left), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	work(t, ctx, todos, waits)
	if exists(filepath.Join(dir, "scheduler.db")) {
		t.Fatal("a scheduler.db was created beside planning files to migrate")
	}
	if files := planningFiles(t, ctx, cwm); len(files) < 3 {
		t.Fatalf("planning files %v, want the left one, the done one and the wait", files)
	}
	if _, err := todos.Export(ctx, api.ExportInput{}); err == nil {
		t.Fatal("export on the file store answered, want its refusal")
	}
}

// TestAUniverseWhoseProjectHasNoUUIDKeepsTheFileStore: rows that could not be
// keyed by repository keep the universe on the file store.
func TestAUniverseWhoseProjectHasNoUUIDKeepsTheFileStore(t *testing.T) {
	dir := t.TempDir()
	ctx, cwm, todos, waits := selectionWorld(t, dir, false, nil)
	work(t, ctx, todos, waits)
	if exists(filepath.Join(dir, "scheduler.db")) {
		t.Fatal("a scheduler.db was created for a project with no uuid")
	}
	if files := planningFiles(t, ctx, cwm); len(files) < 2 {
		t.Fatalf("planning files %v, want the done one and the wait", files)
	}
}

// : a scheduler.db already in SchedulerDir (and no planning
// file beside it) keeps the universe on the SQLite store whatever the keying of
// its modules - the keying can change with no planning file appearing, and two
// processes or two runs must never write two stores.

// storeOf is the store the application ctx carries chose.
func storeOf(t *testing.T, ctx context.Context) *schedcore.SchedulerStore {
	t.Helper()
	st, err := application.UseService[*schedcore.SchedulerStore](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// aFirstRun leaves a scheduler.db in dir: a universe with a keyed module does
// its work on the SQLite store, then closes.
func aFirstRun(t *testing.T, dir string) {
	t.Helper()
	ctx, _, todos, waits := selectionWorld(t, dir, true, nil)
	work(t, ctx, todos, waits)
	appcore.CloseApplicationScope(ctx)
	if !exists(filepath.Join(dir, "scheduler.db")) {
		t.Fatalf("the first run left no scheduler.db in %s", dir)
	}
}

// TestAnExistingSchedulerDBIsKeptWithNoKeyedModule: a later run on a universe
// with no module at all (none keyed) stays on the scheduler.db the first run
// left, instead of choosing the file store.
func TestAnExistingSchedulerDBIsKeptWithNoKeyedModule(t *testing.T) {
	dir := t.TempDir()
	aFirstRun(t, dir)
	ctx, _, _, _ := selectionWorldOf(t, dir, false, nil, nil)
	if !storeOf(t, ctx).SQLite() {
		t.Fatal("a universe with scheduler.db and no keyed module chose the file store, want SQLite")
	}
}

// TestAnExistingSchedulerDBIsKeptWithAnUnkeyedModule: a later run whose module
// belongs to a project with no uuid stays on SQLite, and that module's put is
// refused naming it - never moved silently to planning files.
func TestAnExistingSchedulerDBIsKeptWithAnUnkeyedModule(t *testing.T) {
	dir := t.TempDir()
	aFirstRun(t, dir)
	ctx, cwm, todos, _ := selectionWorld(t, dir, false, nil)
	if !storeOf(t, ctx).SQLite() {
		t.Fatal("a universe with scheduler.db and an unkeyed module chose the file store, want SQLite")
	}
	_, err := todos.Put(ctx, api.PutInput{Todo: api.Todo{Module: "billing", Target: "billing", Title: "implement", Owner: "test", For: api.ForAgent, Start: api.StartAuto, Priority: "P2", State: api.StateReady}})
	if err == nil || !strings.Contains(err.Error(), "billing") {
		t.Fatalf("an unkeyed module's put = %v, want refused naming billing", err)
	}
	if files := planningFiles(t, ctx, cwm); len(files) != 0 {
		t.Fatalf("planning files written beside scheduler.db: %v", files)
	}
}

// TestPlanningFilesBesideSchedulerDBKeepTheFileStore: planning files to migrate
// beside a scheduler.db still keep the file store (the migration's case,
// ) and leave the database unopened.
func TestPlanningFilesBesideSchedulerDBKeepTheFileStore(t *testing.T) {
	dir := t.TempDir()
	aFirstRun(t, dir)
	db := filepath.Join(dir, "scheduler.db")
	before, err := os.Stat(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cwm, todos, waits := selectionWorld(t, dir, true, func(ctx context.Context, cwm cwmapi.CWMService) {
		todo := filepath.Join(planning(t, ctx, cwm, "billing"), "todo")
		if err := os.MkdirAll(todo, 0o755); err != nil {
			t.Fatal(err)
		}
		left := "id: billing.TD-20260101-000000000000-0000\nmodule: billing\ntitle: left\nowner: test\nfor: agent\nstart: manual\npriority: P3\nstate: waiting\n"
		if err := os.WriteFile(filepath.Join(todo, "TD-20260101-000000000000-0000.yaml"), []byte(left), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	if storeOf(t, ctx).SQLite() {
		t.Fatal("planning files beside scheduler.db chose SQLite, want the file store until the migration")
	}
	work(t, ctx, todos, waits)
	if files := planningFiles(t, ctx, cwm); len(files) < 3 {
		t.Fatalf("planning files %v, want the left one, the done one and the wait", files)
	}
	if after, err := os.Stat(db); err != nil || !after.ModTime().Equal(before.ModTime()) || after.Size() != before.Size() {
		t.Fatalf("scheduler.db changed beside planning files (%v)", err)
	}
}

// : the store is chosen at its first use, not while the
// services are built. The CLI resolves the scheduler's services while the
// application is still building them - before CWM is configured, before any
// module is read - and a choice made then saw no module and kept the file
// store for good.

// TestTheStoreIsChosenAtFirstUseNotWhileTheServicesAreBuilt: the scheduler's
// store and its to-dos are resolved at init, the way the CLI resolves them,
// before the universe's project and module exist; the first use comes once
// the module of a project with a uuid is there - the SQLite store is chosen.
func TestTheStoreIsChosenAtFirstUseNotWhileTheServicesAreBuilt(t *testing.T) {
	dir := t.TempDir()
	early := func(init *application.ApplicationInit) {
		init.AddContribute("rq1003", ".early", func(app *application.Application) error {
			if _, err := application.AppUseService[*schedcore.SchedulerStore](app); err != nil {
				return err
			}
			_, err := application.AppUseService[api.TodoService](app)
			return err
		})
	}
	ctx, cwm, todos, waits := selectionWorldOf(t, dir, true, []string{"billing"}, nil, early)
	work(t, ctx, todos, waits)
	if !storeOf(t, ctx).SQLite() {
		t.Fatal("a store resolved while the services were built kept the file store on a fresh keyed universe, want SQLite")
	}
	if !exists(filepath.Join(dir, "scheduler.db")) {
		t.Fatalf("no scheduler.db in %s: the verbs did not run on the SQLite store", dir)
	}
	if files := planningFiles(t, ctx, cwm); len(files) != 0 {
		t.Fatalf("planning files written on a fresh keyed universe: %v", files)
	}
}
