// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// THE FILE STORE — the store port's first adapter: a module's to-dos are YAML
// files of its planning space (`.ktw/todo/`, then `.ktw/done/`), its waits
// those of `.ktw/waiting/`; every write holds the module's lock (locked) and
// lands through a rename. Under the lock a write reads the file again, so the
// revision it compares is the one on disk.
type fileStore struct {
	// spaceOf is a module's planning space: planningSpace, a test's folder.
	spaceOf func(ctx context.Context, module string) (string, error)
	// refuse refuses a write once the planning files were moved into the
	// SQLite store: a process chosen the file store before
	// the migration writes no planning file after it. nil: never.
	refuse func(ctx context.Context) error
}

var _ Store = (*fileStore)(nil)

func newFileStore() *fileStore {
	return &fileStore{spaceOf: planningSpace, refuse: refuseAfterMigration}
}

// refuseAfterMigration refuses a file store's write in a universe whose
// planning files were moved into the SQLite store (its end marker).
func refuseAfterMigration(ctx context.Context) error {
	env, _ := application.UseService[application.AppEnvironment](ctx)
	if at := migratedAt(env); at != "" {
		return fmt.Errorf("%w: this process still runs on the file store, but the planning files were moved into the scheduler's store on %s: restart it - no to-do or wait file is written any more", api.ErrPlanningFileAfterMigration, at)
	}
	return nil
}

// writable is nil when the file store may write.
func (f *fileStore) writable(ctx context.Context) error {
	if f.refuse == nil {
		return nil
	}
	return f.refuse(ctx)
}

// spaceOfTodo is the planning space of the module a to-do id names.
func (f *fileStore) spaceOfTodo(ctx context.Context, id string) (string, error) {
	module, err := moduleOfID(id)
	if err != nil {
		return "", err
	}
	return f.spaceOf(ctx, module)
}

func (f *fileStore) Get(ctx context.Context, id string) (api.Todo, error) {
	space, err := f.spaceOfTodo(ctx, id)
	if err != nil {
		return api.Todo{}, err
	}
	for _, dir := range []string{todoDir, doneDir} {
		t, err := read(filepath.Join(space, dir, fileOf(id)))
		if err == nil {
			return t, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return api.Todo{}, err
		}
	}
	return api.Todo{}, fmt.Errorf("%w: to-do %s", cwmapi.ErrNotFound, id)
}

func (f *fileStore) List(ctx context.Context, module string, done bool) ([]api.Todo, error) {
	space, err := f.spaceOf(ctx, module)
	if err != nil {
		return nil, err
	}
	dir := todoDir
	if done {
		dir = doneDir
	}
	return listDir(filepath.Join(space, dir))
}

func (f *fileStore) Create(ctx context.Context, t api.Todo) (api.Todo, error) {
	if err := f.writable(ctx); err != nil {
		return api.Todo{}, err
	}
	space, err := f.spaceOf(ctx, t.Module)
	if err != nil {
		return api.Todo{}, err
	}
	out := t
	err = locked(space, func() error {
		// A key an open to-do of the module carries: that to-do, put once.
		if t.Key != "" {
			open, err := listDir(filepath.Join(space, todoDir))
			if err != nil {
				return err
			}
			for _, o := range open {
				if o.Key == t.Key {
					out = o
					return nil
				}
			}
		}
		// A module qualified by its project (<key>:<module>) is found in that
		// project's planning space; its to-do keeps the bare module.
		t.Module = bareModule(t.Module)
		t.ID, t.Made, t.Revision = newID(t.Module), now(), 1
		out = t
		return write(filepath.Join(space, todoDir), t)
	})
	return out, err
}

func (f *fileStore) Put(ctx context.Context, t api.Todo) (api.Todo, error) {
	return f.rewrite(ctx, t.ID, false, func(cur *api.Todo) error {
		if cur.Revision != t.Revision {
			return fmt.Errorf("%w: %s read at revision %d, now %d", api.ErrStaleRevision, t.ID, t.Revision, cur.Revision)
		}
		if t.Claim != nil && t.Claim.Ticket != 0 && (cur.Claim == nil || cur.Claim.Ticket != t.Claim.Ticket) {
			return fmt.Errorf("%w: %s written with ticket %d", api.ErrSupersededTicket, t.ID, t.Claim.Ticket)
		}
		*cur = t
		return nil
	})
}

func (f *fileStore) Update(ctx context.Context, id string, fn func(t *api.Todo) error) (api.Todo, error) {
	return f.rewrite(ctx, id, false, fn)
}

func (f *fileStore) Claim(ctx context.Context, id string, fn func(t *api.Todo) error) (api.Todo, error) {
	return f.rewrite(ctx, id, true, fn)
}

// rewrite reads a scheduled to-do under its module's lock, lets fn change it
// and writes it at the next revision; a claim taken in it gets that revision
// as its ticket.
func (f *fileStore) rewrite(ctx context.Context, id string, claim bool, fn func(t *api.Todo) error) (api.Todo, error) {
	if err := f.writable(ctx); err != nil {
		return api.Todo{}, err
	}
	space, err := f.spaceOfTodo(ctx, id)
	if err != nil {
		return api.Todo{}, err
	}
	var out api.Todo
	err = locked(space, func() error {
		cur, err := read(filepath.Join(space, todoDir, fileOf(id)))
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: to-do %s is not scheduled", cwmapi.ErrNotFound, id)
		}
		if err != nil {
			return err
		}
		t := cloneTodo(cur)
		if err := fn(&t); err != nil {
			return err
		}
		t.Revision = cur.Revision + 1
		if claim && t.Claim != nil {
			c := *t.Claim
			c.Ticket = t.Revision
			t.Claim = &c
		}
		if err := write(filepath.Join(space, todoDir), t); err != nil {
			return err
		}
		out = t
		return nil
	})
	return out, err
}

func (f *fileStore) Archive(ctx context.Context, id string, fn func(t *api.Todo) error) (api.Todo, error) {
	if err := f.writable(ctx); err != nil {
		return api.Todo{}, err
	}
	space, err := f.spaceOfTodo(ctx, id)
	if err != nil {
		return api.Todo{}, err
	}
	var out api.Todo
	err = locked(space, func() error {
		path := filepath.Join(space, todoDir, fileOf(id))
		t, err := read(path)
		if err != nil {
			return err
		}
		rev := t.Revision
		if err := fn(&t); err != nil {
			return err
		}
		t.Revision = rev + 1
		if err := write(filepath.Join(space, doneDir), t); err != nil {
			return err
		}
		out = t
		return os.Remove(path)
	})
	return out, err
}

// Revisions is the to-do's file as it is: the one revision the file store
// keeps - the earlier ones are in the git archive.
func (f *fileStore) Revisions(ctx context.Context, id string) ([]api.Todo, error) {
	t, err := f.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return []api.Todo{t}, nil
}

func (f *fileStore) RemoveDone(ctx context.Context, id string) error {
	space, err := f.spaceOfTodo(ctx, id)
	if err != nil {
		return err
	}
	return os.Remove(filepath.Join(space, doneDir, fileOf(id)))
}

func (f *fileStore) AddWait(ctx context.Context, module, on string, mk func(id string) api.Wait) (api.Wait, error) {
	if err := f.writable(ctx); err != nil {
		return api.Wait{}, err
	}
	space, err := f.spaceOf(ctx, module)
	if err != nil {
		return api.Wait{}, err
	}
	var out api.Wait
	err = locked(space, func() error {
		all, err := listAs[api.Wait](filepath.Join(space, waitingDir))
		if err != nil {
			return err
		}
		for _, x := range all {
			if x.On == on {
				out = x
				return nil
			}
		}
		out = mk(idOf(bareModule(module), "WT"))
		return writeAs(filepath.Join(space, waitingDir), out.ID, out)
	})
	return out, err
}

func (f *fileStore) ListWaits(ctx context.Context, module string) ([]api.Wait, error) {
	space, err := f.spaceOf(ctx, module)
	if err != nil {
		return nil, err
	}
	return listAs[api.Wait](filepath.Join(space, waitingDir))
}

func (f *fileStore) PutWait(ctx context.Context, w api.Wait) error {
	if err := f.writable(ctx); err != nil {
		return err
	}
	space, err := f.spaceOf(ctx, w.Module)
	if err != nil {
		return err
	}
	return locked(space, func() error { return writeAs(filepath.Join(space, waitingDir), w.ID, w) })
}

func (f *fileStore) RemoveWait(ctx context.Context, id string) error {
	module, err := moduleOfWait(id)
	if err != nil {
		return err
	}
	space, err := f.spaceOf(ctx, module)
	if err != nil {
		return err
	}
	err = locked(space, func() error { return os.Remove(filepath.Join(space, waitingDir, fileOf(id))) })
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: wait %s", cwmapi.ErrNotFound, id)
	}
	return err
}

// Atomically runs fn as it is: the file store holds one module's lock per
// write, never the whole store - a pull's look and its claim are two holds
// (the SQLite store makes them one transaction).
func (f *fileStore) Atomically(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}
