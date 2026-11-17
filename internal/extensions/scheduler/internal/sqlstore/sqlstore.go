// SPDX-License-Identifier: Apache-2.0

// Package sqlstore is the scheduler's SQLite store: the store port's adapter
// that keeps the to-dos, their claims and the waits in one database file
// (SchedulerDir()/scheduler.db), the only package that speaks the SQLite
// driver (an architecture test refuses its import anywhere else).
//
// A row carries its key columns - id, repository (the project uuid of its
// module's project; NULL for a row with no project), module, state, its
// reference columns (request, target, held_target: each object it names by
// its full name, <repository uuid>.<module>.<local>), the held claim's ticket and until, its revision - beside the to-do record
// itself, kept whole as JSON: a field the to-do gains flows through with no
// schema step.
//
// Every write names the revision it read (UPDATE ... WHERE id=? AND rev=?) and
// is refused stale once another write came between; a claim takes a fencing
// ticket from one store-wide increasing sequence, a renewal keeps it, and a
// write naming a ticket that is no longer the held claim's is refused. A claim
// ends by time (its until), never by a process's liveness. Atomically runs a
// whole pull in one BEGIN IMMEDIATE transaction: no other write - from this
// process or another on the same file - comes between its reads and its claim.
package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	_ "modernc.org/sqlite" // the pure-Go SQLite driver, "sqlite"
)

// File is the store's file name in its folder (SchedulerDir()).
const File = "scheduler.db"

// Pack is the pack the schema steps below belong to.
const Pack = "scheduler"

// busyTimeout is how long a write waits for another's transaction.
const busyTimeout = 10 * time.Second

// Options are what the store asks of the scheduler: the ids it mints, the time
// it stamps, and the repository a module's rows are keyed by.
type Options struct {
	// NewID mints an id of module of a kind (TD a to-do, WT a wait).
	NewID func(module, kind string) string
	// Now is the time a new to-do is made at.
	Now func() string
	// RepoOf is the repository (project uuid) a module's rows carry; "" is a
	// row with no project (NULL).
	RepoOf func(ctx context.Context, module string) (string, error)
}

// Store is the SQLite adapter of the scheduler's store port.
type Store struct {
	db   *sql.DB
	path string
	opt  Options
}

// Open opens - and creates, or brings its schema up to date - the store of dir
// (dir/scheduler.db): in WAL with synchronous=FULL on every pooled connection
// (a commit is flushed to disk before it is acknowledged, so an acknowledged
// write survives an OS crash or a power cut), every transaction BEGIN
// IMMEDIATE, a writer waiting busyTimeout for another. The file sits on local
// disk only.
func Open(dir string, opt Options) (*Store, error) {
	if opt.NewID == nil || opt.Now == nil || opt.RepoOf == nil {
		return nil, fmt.Errorf("%w: the scheduler's store needs NewID, Now and RepoOf", cwmapi.ErrInvalidInput)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, File)
	dsn := "file:" + filepath.ToSlash(path) +
		fmt.Sprintf("?_txlock=immediate&_pragma=busy_timeout(%d)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)", busyTimeout.Milliseconds())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("the scheduler's store %s: %w", path, err)
	}
	s := &Store{db: db, path: path, opt: opt}
	if err := s.checkIntegrity(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := s.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("the scheduler's store %s: %w", path, err)
	}
	return s, nil
}

// checkIntegrity runs PRAGMA quick_check on the file before anything reads it:
// a damaged page, a malformed record or a file SQLite cannot read - anything
// but "ok" - is refused, naming it, never recreated, truncated or read in part.
// quick_check skips what is costly (that each index holds exactly its table's
// rows, UNIQUE constraints): the full PRAGMA integrity_check runs on demand,
// through Doctor (todos doctor), not on every open.
func (s *Store) checkIntegrity(ctx context.Context) error {
	faults, err := pragmaCheck(ctx, s.db, "PRAGMA quick_check")
	if err != nil {
		return damaged(s.path, err.Error())
	}
	if len(faults) > 0 {
		return damaged(s.path, faults[0])
	}
	return nil
}

// pragmaCheck runs an integrity pragma and answers the faults it lists: none
// when its one row is "ok".
func pragmaCheck(ctx context.Context, db *sql.DB, pragma string) ([]string, error) {
	rows, err := db.QueryContext(ctx, pragma)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var faults []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return nil, err
		}
		if line != "ok" {
			faults = append(faults, line)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return faults, nil
}

// ErrDamaged is a store file that fails its integrity check.
var ErrDamaged = errors.New("the scheduler's store is damaged")

// Repair is what brings a damaged store back: the file moved aside, the to-dos
// imported again from the export refs.
const Repair = "move the file aside (with its -wal and -shm files) and import the to-dos again from the export ref (todos import)"

func damaged(path, msg string) error {
	return fmt.Errorf("%w: %s fails its integrity check (%s); it is left as it is - %s", ErrDamaged, path, msg, Repair)
}

// Fault is one fault the full integrity check found, and its repair.
type Fault struct {
	Fault  string
	Repair string
}

// Doctor runs the full PRAGMA integrity_check on the store of dir
// (dir/scheduler.db) - every page, every record, every index against its table
// - and answers each fault it lists with its repair; none on a sound store.
// It opens the file read-only, outside Open: a damaged file is examined, never
// migrated, written or created. found is false when dir holds no store.
func Doctor(ctx context.Context, dir string) (path string, found bool, faults []Fault, err error) {
	path = filepath.Join(dir, File)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return path, false, nil, nil
	} else if err != nil {
		return path, false, nil, err
	}
	dsn := "file:" + filepath.ToSlash(path) +
		fmt.Sprintf("?mode=ro&_pragma=busy_timeout(%d)", busyTimeout.Milliseconds())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return path, true, []Fault{{Fault: err.Error(), Repair: Repair}}, nil
	}
	defer db.Close()
	lines, err := pragmaCheck(ctx, db, "PRAGMA integrity_check")
	if err != nil {
		lines = []string{err.Error()}
	}
	for _, l := range lines {
		faults = append(faults, Fault{Fault: l, Repair: Repair})
	}
	return path, true, faults, nil
}

// Path is the store's file.
func (s *Store) Path() string { return s.path }

// Close closes the store.
func (s *Store) Close() error { return s.db.Close() }

// --- the schema --------------------------------------------------------------

// step is a schema step: the pack it belongs to and what it runs. Step i
// brings PRAGMA user_version from i to i+1; a step once shipped never changes.
type step struct {
	pack string
	sql  string
}

var steps = []step{
	{Pack, `
CREATE TABLE schema_steps (
	version INTEGER PRIMARY KEY,
	pack    TEXT NOT NULL,
	applied TEXT NOT NULL
);
CREATE TABLE counters (
	name  TEXT PRIMARY KEY,
	value INTEGER NOT NULL
);
INSERT INTO counters(name, value) VALUES ('ticket', 0);
CREATE TABLE todos (
	id          TEXT PRIMARY KEY,
	repository  TEXT,
	module      TEXT NOT NULL,
	done        INTEGER NOT NULL DEFAULT 0,
	state       TEXT NOT NULL,
	request     TEXT,
	key         TEXT,
	rev         INTEGER NOT NULL,
	ticket      INTEGER,
	claim_until TEXT,
	body        TEXT NOT NULL
);
CREATE INDEX todos_of_module ON todos(repository, module, done);
CREATE TABLE waits (
	id         TEXT PRIMARY KEY,
	repository TEXT,
	module     TEXT NOT NULL,
	on_what    TEXT NOT NULL,
	body       TEXT NOT NULL
);
CREATE INDEX waits_of_module ON waits(repository, module);
`},
	// The reference columns keep full names: the request, the
	// target and a blocked report's object wait of a to-do, the request a wait
	// is on - each <repository uuid>.<module>.<local>, never a path or a
	// project key (a renamed key leaves them resolving).
	{Pack, `
ALTER TABLE todos ADD COLUMN target TEXT;
ALTER TABLE todos ADD COLUMN held_target TEXT;
UPDATE todos SET
	target = CASE
		WHEN coalesce(json_extract(body, '$.target'), '') = '' THEN NULL
		WHEN repository IS NULL THEN json_extract(body, '$.target')
		ELSE repository || '.' || json_extract(body, '$.target') END,
	held_target = CASE
		WHEN json_extract(body, '$.held.kind') IS NOT 'object' OR coalesce(json_extract(body, '$.held.target'), '') = '' THEN NULL
		WHEN repository IS NULL THEN json_extract(body, '$.held.target')
		ELSE repository || '.' || json_extract(body, '$.held.target') END,
	request = CASE WHEN request IS NULL OR repository IS NULL THEN request ELSE repository || '.' || request END;
UPDATE waits SET on_what = repository || '.' || on_what WHERE repository IS NOT NULL;
CREATE INDEX todos_by_target ON todos(target);
CREATE INDEX todos_by_request ON todos(request);
`},
	// Every revision a write makes is kept (: todos history):
	// the to-do's record at that revision; the rows already there are their
	// current revision. A done to-do pruned leaves with its revisions.
	{Pack, `
CREATE TABLE todo_revisions (
	id   TEXT NOT NULL,
	rev  INTEGER NOT NULL,
	body TEXT NOT NULL,
	PRIMARY KEY (id, rev)
);
INSERT INTO todo_revisions(id, rev, body) SELECT id, rev, body FROM todos;
`},
}

// migrate runs the schema steps the file has not run yet, each in its own
// transaction with the version it reaches. A file that already holds a schema
// (user_version > 0) is snapshotted once before its first pending step; a new
// file has nothing to keep.
func (s *Store) migrate(ctx context.Context) error {
	snapped := false // a new file (user_version 0 on entry) has nothing to keep
	for first := true; ; first = false {
		var v int
		if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); err != nil {
			return err
		}
		if first && v == 0 {
			snapped = true
		}
		if v > len(steps) {
			return fmt.Errorf("its schema is at version %d, newer than this program's %d", v, len(steps))
		}
		if v == len(steps) {
			return nil
		}
		if v > 0 && !snapped {
			if _, err := s.Snapshot(ctx, SnapshotSchema); err != nil {
				return err
			}
			snapped = true
		}
		err := s.inTx(ctx, func(q querier) error {
			var again int
			if err := q.QueryRowContext(ctx, "PRAGMA user_version").Scan(&again); err != nil || again != v {
				return err // another opener ran it: look again
			}
			if _, err := q.ExecContext(ctx, steps[v].sql); err != nil {
				return fmt.Errorf("schema step %d (%s): %w", v+1, steps[v].pack, err)
			}
			if _, err := q.ExecContext(ctx, "INSERT INTO schema_steps(version, pack, applied) VALUES (?, ?, ?)", v+1, steps[v].pack, time.Now().UTC().Format(time.RFC3339)); err != nil {
				return err
			}
			_, err := q.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", v+1))
			return err
		})
		if err != nil {
			return err
		}
	}
}

// --- transactions ------------------------------------------------------------

type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// txKey is the context key of this store's running transaction.
type txKey struct{ s *Store }

func (s *Store) txOf(ctx context.Context) *sql.Tx {
	tx, _ := ctx.Value(txKey{s}).(*sql.Tx)
	return tx
}

// reader is the running transaction, or the database.
func (s *Store) reader(ctx context.Context) querier {
	if tx := s.txOf(ctx); tx != nil {
		return tx
	}
	return s.db
}

// inTx runs fn in the running transaction, or in one of its own it commits
// when fn succeeds.
func (s *Store) inTx(ctx context.Context, fn func(q querier) error) error {
	if tx := s.txOf(ctx); tx != nil {
		return fn(tx)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Atomically runs fn in one BEGIN IMMEDIATE transaction: every call of this
// store fn makes with the context it is given reads and writes in it, and no
// other writer comes between. fn's error rolls it back.
func (s *Store) Atomically(ctx context.Context, fn func(ctx context.Context) error) error {
	if s.txOf(ctx) != nil {
		return fn(ctx)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(context.WithValue(ctx, txKey{s}, tx)); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// --- to-dos ------------------------------------------------------------------

// nullable is s, or NULL when empty.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ticketOf is the held claim's ticket column, NULL when none.
func ticketOf(t api.Todo) any {
	if t.Claim == nil || t.Claim.Ticket == 0 {
		return nil
	}
	return t.Claim.Ticket
}

func untilOf(t api.Todo) any {
	if t.Claim == nil {
		return nil
	}
	return nullable(t.Claim.Until)
}

// ref is what a reference column keeps of an object a row of repo names: its
// full name (cwmapi.FullName: the repository uuid and the authored id) - never
// a path, never a project key; the authored id in a row of no repository;
// NULL when it names none. An id qualified by another repository's key is
// refused: a row names the objects of its own repository.
func ref(repo, id string) (any, error) {
	switch {
	case id == "":
		return nil, nil
	case strings.Contains(id, ":"):
		return nil, fmt.Errorf("%w: %q names an object of another repository; a row keeps references of its own", cwmapi.ErrInvalidInput, id)
	case repo == "":
		return id, nil
	}
	return cwmapi.FullName(repo, id), nil
}

// refs are a to-do's reference columns: its request, its target and the
// object its blocked report waits on.
func refs(t api.Todo) (request, target, held any, err error) {
	if request, err = ref(t.Repository, t.Request); err != nil {
		return
	}
	if target, err = ref(t.Repository, t.Target); err != nil {
		return
	}
	if t.Held != nil && t.Held.Kind == api.HeldObject {
		held, err = ref(t.Repository, t.Held.Target)
	}
	return
}

// encode is a to-do's record: the to-do without what its columns keep.
func encode(t api.Todo) (string, error) {
	t.Revision, t.Repository = 0, ""
	b, err := json.Marshal(t)
	return string(b), err
}

const todoCols = "body, rev, repository"

func scanTodo(sc interface{ Scan(...any) error }) (api.Todo, error) {
	var body string
	var rev int64
	var repo sql.NullString
	if err := sc.Scan(&body, &rev, &repo); err != nil {
		return api.Todo{}, err
	}
	var t api.Todo
	if err := json.Unmarshal([]byte(body), &t); err != nil {
		return api.Todo{}, err
	}
	t.Revision, t.Repository = rev, repo.String
	return t, nil
}

func (s *Store) Get(ctx context.Context, id string) (api.Todo, error) {
	t, err := scanTodo(s.reader(ctx).QueryRowContext(ctx, "SELECT "+todoCols+" FROM todos WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return api.Todo{}, fmt.Errorf("%w: to-do %s", cwmapi.ErrNotFound, id)
	}
	return t, err
}

// scheduledRow reads a to-do still scheduled.
func scheduledRow(ctx context.Context, q querier, id string) (api.Todo, error) {
	t, err := scanTodo(q.QueryRowContext(ctx, "SELECT "+todoCols+" FROM todos WHERE id = ? AND done = 0", id))
	if errors.Is(err, sql.ErrNoRows) {
		return api.Todo{}, fmt.Errorf("%w: to-do %s is not scheduled", cwmapi.ErrNotFound, id)
	}
	return t, err
}

// keyOf is the key of a module's rows: the repository its reference
// resolves to (RepoOf) and its bare module - a reference qualified by its
// project (<key>:<module>) names that project's module, its rows
// keeping the module bare beside the repository.
func (s *Store) keyOf(ctx context.Context, of string) (repo, module string, err error) {
	if repo, err = s.opt.RepoOf(ctx, of); err != nil {
		return "", "", err
	}
	if _, module, err = cwmapi.SplitReference(of); err != nil {
		return "", "", fmt.Errorf("module %q: %w", of, err)
	}
	return repo, module, nil
}

func (s *Store) List(ctx context.Context, of string, done bool) ([]api.Todo, error) {
	repo, module, err := s.keyOf(ctx, of)
	if err != nil {
		return nil, err
	}
	rows, err := s.reader(ctx).QueryContext(ctx, "SELECT "+todoCols+" FROM todos WHERE repository IS ? AND module = ? AND done = ? ORDER BY id", nullable(repo), module, done)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []api.Todo
	for rows.Next() {
		t, err := scanTodo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) Create(ctx context.Context, t api.Todo) (api.Todo, error) {
	repo, module, err := s.keyOf(ctx, t.Module)
	if err != nil {
		return api.Todo{}, err
	}
	t.Module = module
	var out api.Todo
	err = s.inTx(ctx, func(q querier) error {
		// A key an open to-do of the module carries: that to-do, put once.
		if t.Key != "" {
			got, err := scanTodo(q.QueryRowContext(ctx, "SELECT "+todoCols+" FROM todos WHERE repository IS ? AND module = ? AND done = 0 AND key = ?", nullable(repo), t.Module, t.Key))
			if err == nil {
				out = got
				return nil
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		t.ID, t.Made, t.Revision, t.Repository = s.opt.NewID(t.Module, "TD"), s.opt.Now(), 1, repo
		body, err := encode(t)
		if err != nil {
			return err
		}
		request, target, held, err := refs(t)
		if err != nil {
			return err
		}
		if _, err := q.ExecContext(ctx, `INSERT INTO todos(id, repository, module, done, state, request, target, held_target, key, rev, ticket, claim_until, body)
			VALUES (?, ?, ?, 0, ?, ?, ?, ?, ?, 1, ?, ?, ?)`,
			t.ID, nullable(repo), t.Module, t.State, request, target, held, nullable(t.Key), ticketOf(t), untilOf(t), body); err != nil {
			return err
		}
		if err := keep(ctx, q, t.ID, 1, body); err != nil {
			return err
		}
		out = t
		return nil
	})
	return out, err
}

// save writes t over the scheduled row read at rev (and holding ticket when
// fenced): refused stale when no row matched.
func save(ctx context.Context, q querier, t api.Todo, rev int64, done bool) error {
	body, err := encode(t)
	if err != nil {
		return err
	}
	request, target, held, err := refs(t)
	if err != nil {
		return err
	}
	res, err := q.ExecContext(ctx, `UPDATE todos SET done = ?, state = ?, request = ?, target = ?, held_target = ?, key = ?, rev = ?, ticket = ?, claim_until = ?, body = ?
		WHERE id = ? AND done = 0 AND rev = ?`,
		done, t.State, request, target, held, nullable(t.Key), t.Revision, ticketOf(t), untilOf(t), body, t.ID, rev)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n != 1 {
		return fmt.Errorf("%w: %s read at revision %d", api.ErrStaleRevision, t.ID, rev)
	}
	return keep(ctx, q, t.ID, t.Revision, body)
}

// keep records a to-do's record at a revision, in the write that made it; a
// revision already kept (an import run again) is left as it is.
func keep(ctx context.Context, q querier, id string, rev int64, body string) error {
	_, err := q.ExecContext(ctx, "INSERT INTO todo_revisions(id, rev, body) VALUES (?, ?, ?) ON CONFLICT(id, rev) DO NOTHING", id, rev, body)
	return err
}

// Revisions reads every revision of a to-do the store wrote, oldest first.
func (s *Store) Revisions(ctx context.Context, id string) ([]api.Todo, error) {
	cur, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	rows, err := s.reader(ctx).QueryContext(ctx, "SELECT body, rev FROM todo_revisions WHERE id = ? ORDER BY rev", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []api.Todo
	for rows.Next() {
		var body string
		var rev int64
		if err := rows.Scan(&body, &rev); err != nil {
			return nil, err
		}
		var t api.Todo
		if err := json.Unmarshal([]byte(body), &t); err != nil {
			return nil, err
		}
		t.Revision, t.Repository = rev, cur.Repository
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) Put(ctx context.Context, t api.Todo) (api.Todo, error) {
	var out api.Todo
	err := s.inTx(ctx, func(q querier) error {
		cur, err := scheduledRow(ctx, q, t.ID)
		if err != nil {
			return err
		}
		if cur.Revision != t.Revision {
			return fmt.Errorf("%w: %s read at revision %d, now %d", api.ErrStaleRevision, t.ID, t.Revision, cur.Revision)
		}
		if t.Claim != nil && t.Claim.Ticket != 0 && (cur.Claim == nil || cur.Claim.Ticket != t.Claim.Ticket) {
			return fmt.Errorf("%w: %s written with ticket %d", api.ErrSupersededTicket, t.ID, t.Claim.Ticket)
		}
		next := t
		next.Module, next.Repository, next.Revision = cur.Module, cur.Repository, cur.Revision+1
		if err := save(ctx, q, next, cur.Revision, false); err != nil {
			return err
		}
		out = next
		return nil
	})
	return out, err
}

func (s *Store) Update(ctx context.Context, id string, fn func(t *api.Todo) error) (api.Todo, error) {
	return s.rewrite(ctx, id, false, false, fn)
}

func (s *Store) Claim(ctx context.Context, id string, fn func(t *api.Todo) error) (api.Todo, error) {
	return s.rewrite(ctx, id, true, false, fn)
}

func (s *Store) Archive(ctx context.Context, id string, fn func(t *api.Todo) error) (api.Todo, error) {
	return s.rewrite(ctx, id, false, true, fn)
}

// rewrite reads a scheduled to-do, lets fn change it and writes it at the next
// revision - moved to done when archive; a claim taken in it gets the next
// ticket of the store's sequence.
func (s *Store) rewrite(ctx context.Context, id string, claim, archive bool, fn func(t *api.Todo) error) (api.Todo, error) {
	var out api.Todo
	err := s.inTx(ctx, func(q querier) error {
		cur, err := scheduledRow(ctx, q, id)
		if err != nil {
			return err
		}
		t := cur
		if err := clone(cur, &t); err != nil {
			return err
		}
		if err := fn(&t); err != nil {
			return err
		}
		t.ID, t.Module, t.Repository, t.Revision = cur.ID, cur.Module, cur.Repository, cur.Revision+1
		if claim && t.Claim != nil {
			var ticket int64
			if err := q.QueryRowContext(ctx, "UPDATE counters SET value = value + 1 WHERE name = 'ticket' RETURNING value").Scan(&ticket); err != nil {
				return err
			}
			c := *t.Claim
			c.Ticket = ticket
			t.Claim = &c
		}
		if err := save(ctx, q, t, cur.Revision, archive); err != nil {
			return err
		}
		out = t
		return nil
	})
	return out, err
}

// clone gives fn a to-do no reader shares.
func clone(from api.Todo, to *api.Todo) error {
	b, err := json.Marshal(from)
	if err != nil {
		return err
	}
	*to = api.Todo{}
	if err := json.Unmarshal(b, to); err != nil {
		return err
	}
	to.Revision, to.Repository = from.Revision, from.Repository
	return nil
}

func (s *Store) RemoveDone(ctx context.Context, id string) error {
	return s.inTx(ctx, func(q querier) error {
		res, err := q.ExecContext(ctx, "DELETE FROM todos WHERE id = ? AND done = 1", id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("%w: done to-do %s", cwmapi.ErrNotFound, id)
		}
		_, err = q.ExecContext(ctx, "DELETE FROM todo_revisions WHERE id = ?", id)
		return err
	})
}

// RemoveOpen deletes an open to-do's row and its revisions - the migration's
// follow of an open to-do file the file store removed during the move; a done
// row is never touched.
func (s *Store) RemoveOpen(ctx context.Context, id string) error {
	return s.inTx(ctx, func(q querier) error {
		res, err := q.ExecContext(ctx, "DELETE FROM todos WHERE id = ? AND done = 0", id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("%w: open to-do %s", cwmapi.ErrNotFound, id)
		}
		_, err = q.ExecContext(ctx, "DELETE FROM todo_revisions WHERE id = ?", id)
		return err
	})
}

// --- waits -------------------------------------------------------------------

func scanWait(sc interface{ Scan(...any) error }) (api.Wait, error) {
	var body string
	if err := sc.Scan(&body); err != nil {
		return api.Wait{}, err
	}
	var w api.Wait
	err := json.Unmarshal([]byte(body), &w)
	return w, err
}

func (s *Store) AddWait(ctx context.Context, of, on string, mk func(id string) api.Wait) (api.Wait, error) {
	repo, module, err := s.keyOf(ctx, of)
	if err != nil {
		return api.Wait{}, err
	}
	onRef, err := ref(repo, on)
	if err != nil {
		return api.Wait{}, err
	}
	var out api.Wait
	err = s.inTx(ctx, func(q querier) error {
		got, err := scanWait(q.QueryRowContext(ctx, "SELECT body FROM waits WHERE repository IS ? AND module = ? AND on_what = ?", nullable(repo), module, onRef))
		if err == nil {
			out = got
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		w := mk(s.opt.NewID(module, "WT"))
		b, err := json.Marshal(w)
		if err != nil {
			return err
		}
		wRef, err := ref(repo, w.On)
		if err != nil {
			return err
		}
		if _, err := q.ExecContext(ctx, "INSERT INTO waits(id, repository, module, on_what, body) VALUES (?, ?, ?, ?, ?)", w.ID, nullable(repo), module, wRef, string(b)); err != nil {
			return err
		}
		out = w
		return nil
	})
	return out, err
}

func (s *Store) ListWaits(ctx context.Context, of string) ([]api.Wait, error) {
	repo, module, err := s.keyOf(ctx, of)
	if err != nil {
		return nil, err
	}
	rows, err := s.reader(ctx).QueryContext(ctx, "SELECT body FROM waits WHERE repository IS ? AND module = ? ORDER BY id", nullable(repo), module)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []api.Wait
	for rows.Next() {
		w, err := scanWait(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *Store) PutWait(ctx context.Context, w api.Wait) error {
	repo, module, err := s.keyOf(ctx, w.Module)
	if err != nil {
		return err
	}
	b, err := json.Marshal(w)
	if err != nil {
		return err
	}
	onRef, err := ref(repo, w.On)
	if err != nil {
		return err
	}
	return s.inTx(ctx, func(q querier) error {
		_, err := q.ExecContext(ctx, `INSERT INTO waits(id, repository, module, on_what, body) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET on_what = excluded.on_what, body = excluded.body`, w.ID, nullable(repo), module, onRef, string(b))
		return err
	})
}

func (s *Store) RemoveWait(ctx context.Context, id string) error {
	return s.inTx(ctx, func(q querier) error {
		res, err := q.ExecContext(ctx, "DELETE FROM waits WHERE id = ?", id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("%w: wait %s", cwmapi.ErrNotFound, id)
		}
		return nil
	})
}

// --- import ------------------------------------------------------------------

// Imported is what an import brought: the rows it inserted and the rows whose
// id the store already held.
type Imported struct {
	Imported int `json:"imported"`
	Present  int `json:"present"`
}

// ImportTodos brings to-dos in by their id - done marks them done - in one
// transaction: a to-do whose id the store holds is left as it is and counted
// present, so an import run again adds nothing and one run after a partial run
// completes it. Each keeps its revision (1 when it has none) and its claim's
// ticket, the ticket sequence moved above the highest; its repository is its
// module's.
func (s *Store) ImportTodos(ctx context.Context, todos []api.Todo, done bool) (Imported, error) {
	var out Imported
	err := s.inTx(ctx, func(q querier) error {
		var top int64
		for _, t := range todos {
			if t.ID == "" || t.Module == "" {
				return fmt.Errorf("%w: an imported to-do needs its id and its module (%q of %q)", cwmapi.ErrInvalidInput, t.ID, t.Module)
			}
			repo, err := s.opt.RepoOf(ctx, t.Module)
			if err != nil {
				return err
			}
			if t.Revision == 0 {
				t.Revision = 1
			}
			body, err := encode(t)
			if err != nil {
				return err
			}
			t.Repository = repo
			request, target, held, err := refs(t)
			if err != nil {
				return err
			}
			res, err := q.ExecContext(ctx, `INSERT INTO todos(id, repository, module, done, state, request, target, held_target, key, rev, ticket, claim_until, body)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO NOTHING`,
				t.ID, nullable(repo), t.Module, done, t.State, request, target, held, nullable(t.Key), t.Revision, ticketOf(t), untilOf(t), body)
			if err != nil {
				return err
			}
			if n, err := res.RowsAffected(); err != nil {
				return err
			} else if n == 1 {
				if err := keep(ctx, q, t.ID, t.Revision, body); err != nil {
					return err
				}
				out.Imported++
			} else {
				out.Present++
			}
			if t.Claim != nil && t.Claim.Ticket > top {
				top = t.Claim.Ticket
			}
		}
		_, err := q.ExecContext(ctx, "UPDATE counters SET value = max(value, ?) WHERE name = 'ticket'", top)
		return err
	})
	if err != nil {
		return Imported{}, err
	}
	return out, nil
}

// ImportWaits brings waits in by their id, in one transaction: a wait whose id
// the store holds is left as it is and counted present.
func (s *Store) ImportWaits(ctx context.Context, waits []api.Wait) (Imported, error) {
	var out Imported
	err := s.inTx(ctx, func(q querier) error {
		for _, w := range waits {
			if w.ID == "" || w.Module == "" {
				return fmt.Errorf("%w: an imported wait needs its id and its module (%q of %q)", cwmapi.ErrInvalidInput, w.ID, w.Module)
			}
			repo, err := s.opt.RepoOf(ctx, w.Module)
			if err != nil {
				return err
			}
			b, err := json.Marshal(w)
			if err != nil {
				return err
			}
			onRef, err := ref(repo, w.On)
			if err != nil {
				return err
			}
			res, err := q.ExecContext(ctx, "INSERT INTO waits(id, repository, module, on_what, body) VALUES (?, ?, ?, ?, ?) ON CONFLICT(id) DO NOTHING", w.ID, nullable(repo), w.Module, onRef, string(b))
			if err != nil {
				return err
			}
			if n, err := res.RowsAffected(); err != nil {
				return err
			} else if n == 1 {
				out.Imported++
			} else {
				out.Present++
			}
		}
		return nil
	})
	if err != nil {
		return Imported{}, err
	}
	return out, nil
}
