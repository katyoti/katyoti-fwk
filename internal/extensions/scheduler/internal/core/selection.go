// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/internal/sqlstore"
)

// THE STORE SELECTION — one store per application, chosen once at its first
// use - never while the services are built - and shared by the to-dos, the
// waits and the internal scheduler's own callers (its tick, the follow
// observer, the serving hook): the SQLite store in SchedulerDir() on a universe
// that has nothing to migrate and already holds it or whose rows can be keyed,
// the file store otherwise. The choice only reads the universe: it never moves,
// rewrites or removes a file (moving them is).

// SchedulerStore is the store one application's scheduler runs on: an
// application service (local declares it), closed with the application's
// scope - the SQLite handle released, so its folder can be removed.
//
// It is itself a Store whose inner store is chosen at its FIRST USE, not when
// the service is built: an application builds its services before they are
// configured (the CLI resolves the scheduler while CWM is not configured yet),
// and a choice made then saw no module and kept the file store for good
type SchedulerStore struct {
	// ctx is a clean context carrying only the application: the factory's own
	// context holds the resolution stack of the services being built.
	ctx context.Context

	once   sync.Once
	mu     sync.Mutex // guards closed and closer against Close
	closed bool
	store  Store
	closer io.Closer
	err    error
}

var (
	_ io.Closer = (*SchedulerStore)(nil)
	_ Store     = (*SchedulerStore)(nil)
)

// OpenSchedulerStore is the scheduler's store service of the application ctx
// carries: it chooses nothing yet - chooseStore runs at the store's first use.
func OpenSchedulerStore(ctx context.Context) (*SchedulerStore, error) {
	clean := ctx
	if app := application.AppFromContext(ctx); app != nil {
		clean = application.WithApplication(context.Background(), app)
	}
	return &SchedulerStore{ctx: clean}, nil
}

// chosen is the inner store, chosen once, on the first call.
func (s *SchedulerStore) chosen() (Store, error) {
	s.once.Do(func() {
		st, closer, err := chooseStore(s.ctx)
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.closed {
			if closer != nil {
				_ = closer.Close()
			}
			s.err = fmt.Errorf("the scheduler's store is closed")
			return
		}
		s.store, s.closer, s.err = st, closer, err
	})
	return s.store, s.err
}

// Close releases the SQLite store, when the choice opened it.
func (s *SchedulerStore) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	c := s.closer
	s.closer = nil
	if c == nil {
		return nil
	}
	return c.Close()
}

// SQLite reports whether the choice is the SQLite store (making the choice
// when none is made yet).
func (s *SchedulerStore) SQLite() bool {
	st, err := s.chosen()
	if err != nil {
		return false
	}
	_, ok := st.(*sqlstore.Store)
	return ok
}

// innerOf is the store a SchedulerStore chose, st itself otherwise: what a
// capability of the SQLite store (dumper) is asked of.
func innerOf(st Store) Store {
	if s, ok := st.(*SchedulerStore); ok {
		if in, err := s.chosen(); err == nil {
			return in
		}
	}
	return st
}

// chooseStore chooses the store of the application ctx carries:
//  1. the environment names no SchedulerDir() -> the file store;
//  2. a module's planning space, or the universe's own requests space, holds a
//     file in .ktw/todo, .ktw/done or .ktw/waiting -> the file store (a
//     scheduler.db beside such files is warned about, never opened);
//  3. SchedulerDir()/scheduler.db exists -> the SQLite store, whatever the
//     keying of the modules: a store once chosen is never left by a later
//     process or run (an unkeyed module's put is then refused, naming it);
//  4. no module of a project with a uuid, or a module whose rows cannot be
//     keyed (its project has no uuid) -> the file store;
//  5. otherwise -> the SQLite store, SchedulerDir()/scheduler.db.
func chooseStore(ctx context.Context) (Store, io.Closer, error) {
	env, _ := application.UseService[application.AppEnvironment](ctx)
	if err := refuseFilesAfterMigration(ctx, env); err != nil {
		return nil, nil, err
	}
	why := fileStoreReason(ctx, env)
	if why != "" {
		slog.Debug("scheduler: the file store", "why", why)
		return newFileStore(), nil, nil
	}
	st, err := OpenSQLiteStore(env)
	if err != nil {
		return nil, nil, err
	}
	slog.Debug("scheduler: the SQLite store", "dir", env.SchedulerDir())
	return st, st, nil
}

// fileStoreReason says why the universe keeps the file store, "" when it
// takes the SQLite store.
func fileStoreReason(ctx context.Context, env application.AppEnvironment) string {
	if env == nil || env.SchedulerDir() == "" {
		return "the environment names no scheduler folder (SchedulerDir)"
	}
	mods, err := modules(ctx)
	if err != nil {
		return fmt.Sprintf("the modules could not be read: %v", err)
	}
	names := mods
	db := filepath.Join(env.SchedulerDir(), sqlstore.File)
	spaces := []string{filepath.Join(env.SchedulerDir(), api.RequestsModule)}
	for _, m := range names {
		space, err := planningSpace(ctx, m)
		if err != nil {
			return fmt.Sprintf("module %s: its planning space: %v", m, err)
		}
		spaces = append(spaces, space)
	}
	for _, space := range spaces {
		if dir := filesIn(space); dir != "" {
			if _, err := os.Stat(db); err == nil {
				slog.Warn("scheduler: a SQLite store sits beside planning files; the file store is kept and the database is left unopened until the migration",
					"db", db, "files", dir)
			}
			return "planning files to migrate in " + dir
		}
	}
	// A store already chosen stays chosen: the keying of the modules can change
	// with no planning file appearing (a project attached, a module of a project
	// with no uuid added), and two processes or two runs must never write two
	// stores.
	if _, err := os.Stat(db); err == nil {
		return ""
	}
	keyed := 0
	for _, m := range names {
		repo, err := repositoryOf(ctx, m)
		if err != nil {
			return fmt.Sprintf("module %s: its rows cannot be keyed: %v", m, err)
		}
		if repo != "" {
			keyed++
		}
	}
	if keyed == 0 {
		return "no module of a project with a uuid"
	}
	return ""
}

// filesIn is the first of a planning space's to-do, done or waits folders that
// holds a to-do or wait file (*.yaml), "" when none does (an empty folder, the
// settings or the folder's lock file do not count).
func filesIn(space string) string {
	for _, sub := range []string{todoDir, doneDir, waitingDir} {
		dir := filepath.Join(space, sub)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
				return dir
			}
		}
	}
	return ""
}

// refuseFilesAfterMigration is the guard of a migrated universe (its end
// marker beside the store): a planning file found in a
// module's .ktw/todo, .ktw/done or .ktw/waiting was written after the
// migration - by hand, by a process still on the file store, by a pull of
// another clone's files - and is refused, naming it, rather than sending the
// universe back to the file store beside the rows (two stores).
func refuseFilesAfterMigration(ctx context.Context, env application.AppEnvironment) error {
	at := migratedAt(env)
	if at == "" {
		return nil
	}
	spaces, err := planningSpaces(ctx, env)
	if err != nil {
		return nil // the choice below says why the modules cannot be read
	}
	for _, sp := range spaces {
		if dir := filesIn(sp.dir); dir != "" {
			return fmt.Errorf("%w: a to-do or wait file sits in %s, but the planning files were moved into the scheduler's store on %s: to-dos are no longer files - bring it in with `todos migrate --remove`, or remove it", api.ErrPlanningFileAfterMigration, dir, at)
		}
	}
	return nil
}

// storeIn is the store of the application ctx carries.
func storeIn(ctx context.Context) (Store, error) {
	h, err := application.UseService[*SchedulerStore](ctx)
	if err != nil {
		return nil, fmt.Errorf("the scheduler's store: %w", err)
	}
	return h, nil
}

// ServiceIn is the scheduler over the store of the application ctx carries,
// with the application's pull record: a Service is built on every call, the
// record lives as long as the application.
func ServiceIn(ctx context.Context) (*Service, error) {
	st, err := storeIn(ctx)
	if err != nil {
		return nil, err
	}
	return &Service{store: st, pulls: pullRecordIn(ctx)}, nil
}

// WaitsIn is the waits over the store of the application ctx carries.
func WaitsIn(ctx context.Context) (*Waits, error) {
	st, err := storeIn(ctx)
	if err != nil {
		return nil, err
	}
	return &Waits{store: st}, nil
}
