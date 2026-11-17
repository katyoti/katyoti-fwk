// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// THE STORE PORT — the one way the scheduler reads and writes what it keeps:
// the to-dos (scheduled, then done), their claims, and the waits. The
// scheduler's verbs decide; the store keeps. Two adapters: the file store
// (fileStore: a YAML file per to-do in the module's planning space, written
// under the module's operating-system lock), and the SQLite store (package
// sqlstore, SchedulerDir()/scheduler.db, OpenSQLiteStore). An application
// runs on one of them, chosen once (OpenSchedulerStore): SQLite on a universe
// with no planning file to migrate whose rows can be keyed, files otherwise
// until the migration moves them in.
//
// Every to-do the store answers carries its revision; a write carries the
// revision it read and is refused once another write came between
// (api.ErrStaleRevision). A claim takes a fencing ticket - the revision it was
// written at - and a write naming a ticket that is no longer the held claim's
// is refused (api.ErrSupersededTicket).
type Store interface {
	// Get reads one to-do, scheduled or else done; cwmapi.ErrNotFound when
	// neither holds it.
	Get(ctx context.Context, id string) (api.Todo, error)
	// List reads a module's scheduled to-dos, or its done ones.
	List(ctx context.Context, module string, done bool) ([]api.Todo, error)
	// Create puts a new to-do of t.Module, its id, made time and first
	// revision given by the store - or answers the open to-do of the module
	// that carries t.Key, put once.
	Create(ctx context.Context, t api.Todo) (api.Todo, error)
	// Put writes a scheduled to-do read at t.Revision: refused stale when
	// another write came since, superseded when t's claim names a ticket that
	// is not the held claim's. It answers the to-do at its new revision.
	Put(ctx context.Context, t api.Todo) (api.Todo, error)
	// Update reads a scheduled to-do, lets fn change it and writes it at the
	// revision read, in one hold of the store: cwmapi.ErrNotFound when it is
	// not scheduled; fn's error is answered as it is, nothing written.
	Update(ctx context.Context, id string, fn func(t *api.Todo) error) (api.Todo, error)
	// Claim is Update for a claim fn sets: the claim gets its fencing ticket.
	Claim(ctx context.Context, id string, fn func(t *api.Todo) error) (api.Todo, error)
	// Archive moves a scheduled to-do to done once fn accepted and changed it.
	Archive(ctx context.Context, id string, fn func(t *api.Todo) error) (api.Todo, error)
	// RemoveDone removes a done to-do.
	RemoveDone(ctx context.Context, id string) error
	// Revisions reads the revisions of a to-do the store keeps, oldest first:
	// the file store keeps its current one only (git keeps the others), the
	// SQLite store every one it wrote; cwmapi.ErrNotFound when it keeps none.
	Revisions(ctx context.Context, id string) ([]api.Todo, error)

	// AddWait records a wait of module - or answers the one already kept on
	// on; mk makes the new one (its id given by the store).
	AddWait(ctx context.Context, module, on string, mk func(id string) api.Wait) (api.Wait, error)
	// ListWaits reads a module's waits.
	ListWaits(ctx context.Context, module string) ([]api.Wait, error)
	// PutWait rewrites a wait.
	PutWait(ctx context.Context, w api.Wait) error
	// RemoveWait ends a wait; cwmapi.ErrNotFound when none has its id.
	RemoveWait(ctx context.Context, id string) error

	// Atomically runs fn in one hold of the whole store: every call of the
	// store fn makes with the context it is given reads and writes one state,
	// and no other writer comes between - a pull's queue, its module locks and
	// its claim are one transaction. fn's error undoes what it wrote.
	Atomically(ctx context.Context, fn func(ctx context.Context) error) error
}
