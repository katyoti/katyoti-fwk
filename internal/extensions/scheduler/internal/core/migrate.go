// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/atomicio"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/core/oslock"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/internal/sqlstore"
)

// THE MIGRATION OF THE PLANNING FILES - `todos migrate` moves
// every module's .ktw/todo, .ktw/done and .ktw/waiting, and the universe's own
// requests space, into the SQLite store of SchedulerDir(): the files are the
// truth until it ends, each run makes the rows say what they say (a run again
// changes nothing; a run after a partial one, or after work went on in the
// files, completes it) and checks every file has its row - files before, rows
// after. With remove, once none is missing, it deletes the files (git history
// is their archive) and writes the end marker beside the store: from then on
// the store selection refuses a planning file found there (written after the
// migration) instead of going back to the file store. The real run is a
// person's go: no worker may write to-dos while it runs.

// MigratedMarker is the file beside scheduler.db that records the migration's
// end: its presence makes a planning file a refusal.
const MigratedMarker = "scheduler.migrated"

// planningSpaces is every planning space of the universe: the universe's own
// requests space, then each module's, each once, with the module whose
// to-dos it holds.
func planningSpaces(ctx context.Context, env application.AppEnvironment) ([]spaceOf, error) {
	names, err := modules(ctx)
	if err != nil {
		return nil, fmt.Errorf("the modules could not be read: %w", err)
	}
	out := []spaceOf{{module: api.RequestsModule, dir: filepath.Join(env.SchedulerDir(), api.RequestsModule)}}
	seen := map[string]int{filepath.Clean(out[0].dir): 0}
	for _, m := range names {
		dir, err := planningSpace(ctx, m)
		if err != nil {
			return nil, fmt.Errorf("module %s: its planning space: %w", m, err)
		}
		if i, ok := seen[filepath.Clean(dir)]; ok {
			out[i].shares = append(out[i].shares, m)
			continue
		}
		seen[filepath.Clean(dir)] = len(out)
		out = append(out, spaceOf{module: m, dir: dir})
	}
	return out, nil
}

type spaceOf struct {
	module string
	dir    string
	shares []string // the other modules whose planning space this is
}

// filesOf is what a planning space's files hold.
type filesOf struct {
	space       spaceOf
	open, done  []api.Todo
	waits       []api.Wait
	repairs     []api.MigrateRepair
	openDropped map[string]bool   // file names of todo/ left out (a done one exists)
	read        map[string][]byte // each file read, by path: the bytes the rows were made of
}

// readSpace reads a space's to-do, done and waits files, repairing what the
// move cannot take as it is: a to-do in both todo and done (an archive cut
// short) keeps its done one; a to-do naming no module takes its folder's.
func readSpace(sp spaceOf) (filesOf, error) {
	f := filesOf{space: sp, openDropped: map[string]bool{}, read: map[string][]byte{}}
	open, err := readAll[api.Todo](filepath.Join(sp.dir, todoDir), f.read)
	if err != nil {
		return f, err
	}
	done, err := readAll[api.Todo](filepath.Join(sp.dir, doneDir), f.read)
	if err != nil {
		return f, err
	}
	if f.waits, err = readAll[api.Wait](filepath.Join(sp.dir, waitingDir), f.read); err != nil {
		return f, err
	}
	isDone := map[string]bool{}
	for i := range done {
		fixModule(&f, &done[i], doneDir)
		isDone[done[i].ID] = true
	}
	for i := range open {
		if isDone[open[i].ID] {
			f.openDropped[fileOf(open[i].ID)] = true
			f.repairs = append(f.repairs, api.MigrateRepair{File: filepath.Join(sp.dir, todoDir, fileOf(open[i].ID)),
				Repair: "in both todo and done (an archive cut short): its done record is moved in, the open file removed with the others"})
			continue
		}
		fixModule(&f, &open[i], todoDir)
		f.open = append(f.open, open[i])
	}
	f.done = done
	for i := range f.waits {
		if f.waits[i].Module == "" {
			f.waits[i].Module = sp.module
			f.repairs = append(f.repairs, api.MigrateRepair{File: filepath.Join(sp.dir, waitingDir, fileOf(f.waits[i].ID)), Repair: "names no module: its folder's, " + sp.module})
		}
	}
	return f, nil
}

// readAll reads every file of a planning folder from its bytes, and keeps the
// bytes by path in read: a file is removed only while it still holds what its
// row was made of (never through the listing cache, whose size-and-time check
// could answer an older reading).
func readAll[T any](dir string, read map[string][]byte) ([]T, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []T
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		raw, err := atomicio.ReadAtomic(path)
		if errors.Is(err, os.ErrNotExist) {
			continue // moved away meanwhile: a later run takes it where it went
		}
		if err != nil {
			return nil, err
		}
		var v T
		if err := marshal.Unmarshal(raw, marshal.YAML, &v); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		read[path] = raw
		out = append(out, v)
	}
	return out, nil
}

func fixModule(f *filesOf, t *api.Todo, sub string) {
	if t.Module != "" {
		return
	}
	if m, err := moduleOfID(t.ID); err == nil {
		t.Module = m
	} else {
		t.Module = f.space.module
	}
	f.repairs = append(f.repairs, api.MigrateRepair{File: filepath.Join(f.space.dir, sub, fileOf(t.ID)), Repair: "names no module: " + t.Module})
}

// unreadSpaces are the planning folders beside the spaces read - every
// <modules folder>/*/.ktw - that hold a to-do or wait file and are no space
// read: a module the list missed (unlisted, unreadable) whose files would be
// left behind, and lost to the store once the migration ended.
func unreadSpaces(spaces []spaceOf) []string {
	read := map[string]bool{}
	parents := map[string]bool{}
	for _, sp := range spaces {
		read[filepath.Clean(sp.dir)] = true
		if filepath.Base(sp.dir) == ".ktw" {
			parents[filepath.Dir(filepath.Dir(sp.dir))] = true
		}
	}
	var out []string
	for p := range parents {
		found, _ := filepath.Glob(filepath.Join(p, "*", ".ktw"))
		for _, dir := range found {
			if !read[filepath.Clean(dir)] && filesIn(dir) != "" {
				out = append(out, dir)
			}
		}
	}
	sort.Strings(out)
	return out
}

// errDryRun undoes a dry run's transaction.
var errDryRun = errors.New("dry run")

// Migrate moves the planning files into the SQLite store.
func (s *Service) Migrate(ctx context.Context, in api.MigrateInput) (api.MigrateOutput, error) {
	env, _ := application.UseService[application.AppEnvironment](ctx)
	if env == nil || env.SchedulerDir() == "" {
		return api.MigrateOutput{}, invalid("the environment names no scheduler folder (SchedulerDir): there is no SQLite store to move the planning files into")
	}
	spaces, err := planningSpaces(ctx, env)
	if err != nil {
		return api.MigrateOutput{}, err
	}
	out := api.MigrateOutput{DryRun: in.DryRun, Spaces: []api.MigratedSpace{}, Store: filepath.Join(env.SchedulerDir(), sqlstore.File)}
	var all []filesOf
	var open, done []api.Todo
	var waits []api.Wait
	for _, sp := range spaces {
		f, err := readSpace(sp)
		if err != nil {
			return api.MigrateOutput{}, err
		}
		all = append(all, f)
		out.Spaces = append(out.Spaces, api.MigratedSpace{Module: sp.module, Dir: sp.dir, Files: api.MigratedCounts{Open: len(f.open), Done: len(f.done), Waits: len(f.waits)}})
		open, done, waits = append(open, f.open...), append(done, f.done...), append(waits, f.waits...)
		out.Repairs = append(out.Repairs, f.repairs...)
	}
	out.Files = api.MigratedCounts{Open: len(open), Done: len(done), Waits: len(waits)}
	out.Unread = unreadSpaces(spaces)
	migrated := migratedAt(env)
	spared, err := removedIDs(env)
	if err != nil {
		return api.MigrateOutput{}, err
	}

	if in.DryRun {
		if _, err := os.Stat(out.Store); err != nil {
			out.Added = len(open) + len(done) + len(waits) // no store yet: every file would be added
			return out, nil
		}
	}
	st, err := OpenSQLiteStore(env)
	if err != nil {
		return api.MigrateOutput{}, err
	}
	defer st.Close()
	if !in.DryRun {
		// A real run rewrites the store: a checked copy of it first.
		if _, err := st.Snapshot(ctx, sqlstore.SnapshotMigrate); err != nil {
			return api.MigrateOutput{}, err
		}
	}
	err = st.Atomically(ctx, func(ctx context.Context) error {
		for _, part := range []func() (sqlstore.MovedIn, error){
			func() (sqlstore.MovedIn, error) { return st.MoveInTodos(ctx, open, false) },
			func() (sqlstore.MovedIn, error) { return st.MoveInTodos(ctx, done, true) },
			func() (sqlstore.MovedIn, error) { return st.MoveInWaits(ctx, waits, migrated == "") },
		} {
			m, err := part()
			if err != nil {
				return err
			}
			out.Added, out.Updated, out.Unchanged = out.Added+m.Added, out.Updated+m.Updated, out.Unchanged+m.Unchanged
			out.Kept = append(out.Kept, m.Kept...)
		}
		if migrated == "" && out.Files != (api.MigratedCounts{}) {
			repairs, err := sweep(ctx, st, all, spared)
			if err != nil {
				return err
			}
			out.Repairs = append(out.Repairs, repairs...)
		}
		if err := check(ctx, st, open, done, waits, &out); err != nil {
			return err
		}
		if in.DryRun {
			return errDryRun
		}
		return nil
	})
	if err != nil && !errors.Is(err, errDryRun) {
		return api.MigrateOutput{}, err
	}
	if in.DryRun || !in.Remove {
		return out, nil
	}
	if len(out.Unread) > 0 {
		return out, invalid("%d planning folders hold files no listed module owns (unread): the files are kept - make their modules readable, then run again", len(out.Unread))
	}
	if len(out.Missing) > 0 {
		return out, invalid("%d planning files have no row of their revision after the move (missing): the files are kept", len(out.Missing))
	}
	removed, left, gone, err := removeFiles(all, func(ids []string) error { return recordRemoved(env, ids) })
	out.Removed = removed
	if err != nil {
		return out, err
	}
	if len(gone) > 0 {
		var repairs []api.MigrateRepair
		var unresolved []string
		err := st.Atomically(ctx, func(ctx context.Context) error {
			var err error
			repairs, unresolved, err = dropGone(ctx, st, gone)
			return err
		})
		if err != nil {
			return out, err
		}
		out.Repairs = append(out.Repairs, repairs...)
		for _, p := range unresolved {
			if !contains(left, p) {
				left = append(left, p)
			}
		}
	}
	if len(left) > 0 {
		out.Missing = append(out.Missing, left...)
		return out, invalid("%d planning files appeared or changed during the move (kept, no end marker): run the migration again", len(left))
	}
	if migrated != "" {
		out.Migrated = migrated // a later run keeps the first migration's date and totals
		clearSpaces(all)
		return out, nil
	}
	at := time.Now().UTC().Format(time.RFC3339)
	marker := fmt.Sprintf("migrated: %s\nopen: %d\ndone: %d\nwaits: %d\n", at, out.Rows.Open, out.Rows.Done, out.Rows.Waits)
	if err := atomicio.WriteAtomic(filepath.Join(env.SchedulerDir(), MigratedMarker), []byte(marker), 0o644); err != nil {
		return out, err
	}
	out.Migrated = at
	_ = os.Remove(filepath.Join(env.SchedulerDir(), RemovedList)) // the marker ends the sweeps: the list has no reader left
	clearSpaces(all)
	return out, nil
}

// clearSpaces removes, once the end marker is written, what the file store
// leaves behind in each planning space: the lock file of .ktw/todo (no file
// store writes there any more - a planning file found there is refused) and
// the emptied .ktw/todo, .ktw/done and .ktw/waiting folders. A folder still
// holding anything stays, as does a lock a live process holds; nothing here
// fails the migration (the refusal keys on the marker, never on the folders).
func clearSpaces(all []filesOf) {
	for _, f := range all {
		lock := filepath.Join(f.space.dir, todoDir, lockFile)
		if held, err := oslock.Held(lock); err == nil && !held {
			_ = os.Remove(lock)
		}
		for _, sub := range []string{todoDir, doneDir, waitingDir} {
			_ = os.Remove(filepath.Join(f.space.dir, sub)) // only an empty folder goes
		}
	}
}

// check counts the rows the store holds of the files and names every file
// with no row of its revision and folder: an open file needs a row at its
// revision or further on, a done file a done row at its revision or further.
func check(ctx context.Context, st *sqlstore.Store, open, done []api.Todo, waits []api.Wait, out *api.MigrateOutput) error {
	rows, err := st.Rows(ctx)
	if err != nil {
		return err
	}
	have, err := st.WaitIDs(ctx)
	if err != nil {
		return err
	}
	out.Rows, out.Missing = api.MigratedCounts{}, nil
	rev := func(t api.Todo) int64 {
		if t.Revision == 0 {
			return 1
		}
		return t.Revision
	}
	for _, t := range open {
		r, ok := rows[t.ID]
		switch {
		case ok && !r.Done && r.Rev >= rev(t):
			out.Rows.Open++
		case ok && r.Done:
			out.Rows.Done++ // done since in the store: further on than its file
		default:
			out.Missing = append(out.Missing, t.ID)
		}
	}
	for _, t := range done {
		if r, ok := rows[t.ID]; ok && r.Done && r.Rev >= rev(t) {
			out.Rows.Done++
		} else {
			out.Missing = append(out.Missing, t.ID)
		}
	}
	for _, w := range waits {
		if have[w.ID] {
			out.Rows.Waits++
		} else {
			out.Missing = append(out.Missing, w.ID)
		}
	}
	return nil
}

// removeFiles deletes the planning files read that still hold what was read,
// each space under its module's lock (the one every file-store write holds):
// a file written during the move - new, or changed since it was read - is
// kept and answered in left, for a run again to bring in; a file read but
// gone at remove time - its wait cleared, its to-do archived, its done record
// removed by the file store meanwhile - is answered in gone, for its row to
// follow the file store (dropGone): never skipped silently, or its row would
// outlive it.
//
// record, when given, is told the ids of a space's files before any of them
// is removed (RemovedList): a later run's sweep keeps their rows.
func removeFiles(all []filesOf, record func(ids []string) error) (removed int, left []string, gone []goneFile, err error) {
	for _, f := range all {
		if len(f.read) == 0 && filesIn(f.space.dir) == "" {
			continue // nothing read, nothing there: no lock folder to make
		}
		err := locked(f.space.dir, func() error {
			if record != nil && len(f.read) > 0 {
				if err := record(f.ids()); err != nil {
					return err
				}
			}
			seen := map[string]bool{}
			for _, sub := range []string{todoDir, doneDir, waitingDir} {
				dir := filepath.Join(f.space.dir, sub)
				entries, err := os.ReadDir(dir)
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				if err != nil {
					return err
				}
				for _, e := range entries {
					if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
						continue
					}
					path := filepath.Join(dir, e.Name())
					was, ok := f.read[path]
					if !ok {
						left = append(left, path)
						continue
					}
					now, err := atomicio.ReadAtomic(path)
					if errors.Is(err, os.ErrNotExist) {
						continue // listed then gone: named with the unseen below
					}
					if err != nil {
						return err
					}
					seen[path] = true
					if !bytes.Equal(now, was) {
						left = append(left, path)
						continue
					}
					if err := os.Remove(path); err != nil {
						return err
					}
					removed++
				}
			}
			paths := make([]string, 0, len(f.read))
			for path := range f.read {
				if !seen[path] {
					paths = append(paths, path)
				}
			}
			sort.Strings(paths)
			for _, path := range paths {
				if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
					if err != nil {
						return err
					}
					left = append(left, path) // there again: a run again brings it in
					continue
				}
				g, err := goneOf(f.space, path, f.read[path])
				if err != nil {
					return err
				}
				gone = append(gone, g)
			}
			return nil
		})
		if err != nil {
			return removed, left, gone, err
		}
	}
	return removed, left, gone, nil
}

// goneFile is a planning file read but gone at remove time: its folder and the
// id its row carries.
type goneFile struct {
	space spaceOf
	path  string
	sub   string // todoDir, doneDir or waitingDir
	id    string
}

func goneOf(sp spaceOf, path string, raw []byte) (goneFile, error) {
	g := goneFile{space: sp, path: path, sub: filepath.Base(filepath.Dir(path))}
	var head struct {
		ID string `json:"id" yaml:"id"`
	}
	if err := marshal.Unmarshal(raw, marshal.YAML, &head); err != nil {
		return g, fmt.Errorf("%s: %w", path, err)
	}
	if head.ID == "" {
		return g, fmt.Errorf("%s: names no id", path)
	}
	g.id = head.ID
	return g, nil
}

// dropGone makes the store follow the file store for each file gone during
// the move - its removal is the truth: a gone wait (cleared by its event)
// loses its row; a gone done record loses its done row; a gone open to-do
// loses its open row unless it is done - a done row, or a done file written
// meanwhile (an archive: that file is answered in unresolved, kept, for a run
// again to bring in; no end marker until then). A repair is named only for a
// row really removed: a gone file whose row never was is none.
func dropGone(ctx context.Context, st *sqlstore.Store, gone []goneFile) (repairs []api.MigrateRepair, unresolved []string, err error) {
	rows, err := st.Rows(ctx)
	if err != nil {
		return nil, nil, err
	}
	notFound := func(err error) bool { return err == nil || errors.Is(err, cwmapi.ErrNotFound) }
	for _, g := range gone {
		switch g.sub {
		case waitingDir:
			err := st.RemoveWait(ctx, g.id)
			if !notFound(err) {
				return nil, nil, err
			}
			if err == nil {
				repairs = append(repairs, api.MigrateRepair{File: g.path, Repair: "removed during the move (its wait cleared): its row removed"})
			}
		case doneDir:
			err := st.RemoveDone(ctx, g.id)
			if !notFound(err) {
				return nil, nil, err
			}
			if err == nil {
				repairs = append(repairs, api.MigrateRepair{File: g.path, Repair: "removed during the move: its done row removed"})
			}
		default:
			if r, ok := rows[g.id]; ok && r.Done {
				continue // done in the store: further on than its file
			}
			done := filepath.Join(g.space.dir, doneDir, fileOf(g.id))
			if _, err := os.Stat(done); err == nil {
				unresolved = append(unresolved, done) // archived meanwhile: a run again brings its done record in
				continue
			}
			err := st.RemoveOpen(ctx, g.id)
			if !notFound(err) {
				return nil, nil, err
			}
			if err == nil {
				repairs = append(repairs, api.MigrateRepair{File: g.path, Repair: "removed during the move: its open row removed"})
			}
		}
	}
	return repairs, unresolved, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// migratedAt is when the planning files were moved into the store, "" when
// they never were (no end marker beside it).
func migratedAt(env application.AppEnvironment) string {
	if env == nil || env.SchedulerDir() == "" {
		return ""
	}
	raw, err := os.ReadFile(filepath.Join(env.SchedulerDir(), MigratedMarker))
	if err != nil {
		return ""
	}
	first, _, _ := strings.Cut(string(raw), "\n")
	if at := strings.TrimSpace(strings.TrimPrefix(first, "migrated:")); at != "" {
		return at
	}
	return "unknown"
}

// RemovedList is the file beside scheduler.db that names, one id a line, the
// to-dos and waits whose planning files a migration run removed (each space's
// ids written before its files go): before the end marker, a later run's
// sweep keeps their rows - the migration removed their files, not the file
// store. The end marker removes it.
const RemovedList = "scheduler.removed"

// removedIDs is the ids RemovedList names (none when there is no list).
func removedIDs(env application.AppEnvironment) (map[string]bool, error) {
	out := map[string]bool{}
	raw, err := os.ReadFile(filepath.Join(env.SchedulerDir(), RemovedList))
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Fields(string(raw)) {
		if id := strings.TrimSpace(line); id != "" {
			out[id] = true
		}
	}
	return out, nil
}

// recordRemoved adds ids to RemovedList, before their files are removed.
func recordRemoved(env application.AppEnvironment, ids []string) error {
	fh, err := os.OpenFile(filepath.Join(env.SchedulerDir(), RemovedList), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := fh.WriteString(strings.Join(ids, "\n") + "\n"); err != nil {
		_ = fh.Close()
		return err
	}
	if err := fh.Sync(); err != nil {
		_ = fh.Close()
		return err
	}
	return fh.Close()
}

// ids is the id of every file a space's read found.
func (f filesOf) ids() []string {
	out := make([]string, 0, len(f.open)+len(f.done)+len(f.waits)+len(f.openDropped))
	for _, t := range f.open {
		out = append(out, t.ID)
	}
	for _, t := range f.done {
		out = append(out, t.ID) // an open file left out for its done one has this id too
	}
	for _, w := range f.waits {
		out = append(out, w.ID)
	}
	return out
}

// sweep makes the store follow the files of the modules a run read, before
// the end marker - the files are the truth until then, and a run reads them
// all: a row of a module read whose id has no file of its kind anywhere the
// run read is a file removed between two runs (a wait cleared, a to-do
// removed, a done record dropped), so its row goes, named in the repairs. A
// to-do row stays while a to-do file of its id - open or done - was read (the
// move made it follow that file); a row of a module no space was read for
// (unlisted) and a row whose file the migration itself removed (spared, from
// RemovedList) stay. Migrate runs it only before the end marker and only when
// the run read a file: a universe with no planning file and no marker may
// already run on the SQLite store, its rows the truth.
func sweep(ctx context.Context, st *sqlstore.Store, all []filesOf, spared map[string]bool) ([]api.MigrateRepair, error) {
	dirOf := map[string]string{}
	todoIDs, waitIDs := map[string]bool{}, map[string]bool{}
	for _, f := range all {
		dirOf[f.space.module] = f.space.dir
		for _, m := range f.space.shares {
			dirOf[m] = f.space.dir
		}
		for _, t := range f.open {
			todoIDs[t.ID] = true
		}
		for _, t := range f.done {
			todoIDs[t.ID] = true
		}
		for _, w := range f.waits {
			waitIDs[w.ID] = true
		}
	}
	held, err := st.HeldRows(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(held, func(i, j int) bool { return held[i].ID < held[j].ID })
	var repairs []api.MigrateRepair
	for _, h := range held {
		dir, read := dirOf[h.Module]
		if !read || spared[h.ID] {
			continue
		}
		var sub, what string
		switch {
		case h.Wait:
			if waitIDs[h.ID] {
				continue
			}
			err, sub, what = st.RemoveWait(ctx, h.ID), waitingDir, "its wait row removed"
		case todoIDs[h.ID]:
			continue
		case h.Done:
			err, sub, what = st.RemoveDone(ctx, h.ID), doneDir, "its done row removed"
		default:
			err, sub, what = st.RemoveOpen(ctx, h.ID), todoDir, "its open row removed"
		}
		if err != nil {
			return nil, err
		}
		repairs = append(repairs, api.MigrateRepair{File: filepath.Join(dir, sub, fileOf(h.ID)), Repair: "removed between two runs of the migration (no file left): " + what})
	}
	return repairs, nil
}
