// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// THE WHOLE STORE, OUT AND BACK - a repository's rows as the
// store keeps them: every to-do, open and done, with its claim's ticket and
// until, every revision kept of it, every wait, and the ticket sequence. Dump
// reads them in one transaction; Restore brings a dump back - into an empty
// store the same rows with the same revisions, into one already holding some
// a row whose id is there left as it is - so a restore run again adds nothing.

// TodoRow is a to-do's row: its key columns as they are kept and its record
// (body, the JSON the store wrote, kept byte for byte).
type TodoRow struct {
	ID         string `json:"id"`
	Repository string `json:"repository,omitempty"`
	Module     string `json:"module"`
	Done       bool   `json:"done,omitempty"`
	State      string `json:"state"`
	Request    string `json:"request,omitempty"`
	Target     string `json:"target,omitempty"`
	HeldTarget string `json:"held_target,omitempty"`
	Key        string `json:"key,omitempty"`
	Rev        int64  `json:"rev"`
	Ticket     int64  `json:"ticket,omitempty"`
	ClaimUntil string `json:"claim_until,omitempty"`
	Body       string `json:"body"`
}

// RevisionRow is one revision kept of a to-do: its record at that revision.
type RevisionRow struct {
	ID   string `json:"id"`
	Rev  int64  `json:"rev"`
	Body string `json:"body"`
}

// WaitRow is a wait's row.
type WaitRow struct {
	ID         string `json:"id"`
	Repository string `json:"repository,omitempty"`
	Module     string `json:"module"`
	OnWhat     string `json:"on_what"`
	Body       string `json:"body"`
}

// Dump is a repository's rows ("" the rows of no project): its to-dos ordered
// by id, their revisions by id then revision, its waits by id, and the
// store's ticket sequence when it was read.
type Dump struct {
	Repository string        `json:"repository,omitempty"`
	Ticket     int64         `json:"ticket"`
	Todos      []TodoRow     `json:"-"`
	Revisions  []RevisionRow `json:"-"`
	Waits      []WaitRow     `json:"-"`
}

// Restored is what a restore brought: to-dos and waits inserted or already
// present, the revisions it added.
type Restored struct {
	Todos     Imported `json:"todos"`
	Revisions int      `json:"revisions"`
	Waits     Imported `json:"waits"`
}

// Repositories answers the repositories the store holds rows of, sorted; ""
// stands for the rows of no project.
func (s *Store) Repositories(ctx context.Context) ([]string, error) {
	rows, err := s.reader(ctx).QueryContext(ctx, "SELECT DISTINCT coalesce(repository, '') FROM todos UNION SELECT DISTINCT coalesce(repository, '') FROM waits")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	sort.Strings(out)
	return out, rows.Err()
}

func text(n sql.NullString) string { return n.String }

// Dump reads every row of a repository in one transaction: what it answers is
// one state of the store.
func (s *Store) Dump(ctx context.Context, repository string) (Dump, error) {
	out := Dump{Repository: repository}
	err := s.inTx(ctx, func(q querier) error {
		if err := q.QueryRowContext(ctx, "SELECT value FROM counters WHERE name = 'ticket'").Scan(&out.Ticket); err != nil {
			return err
		}
		rows, err := q.QueryContext(ctx, `SELECT id, repository, module, done, state, request, target, held_target, key, rev, ticket, claim_until, body
			FROM todos WHERE repository IS ? ORDER BY id`, nullable(repository))
		if err != nil {
			return err
		}
		for rows.Next() {
			var r TodoRow
			var repo, request, target, held, key, until sql.NullString
			var ticket sql.NullInt64
			if err := rows.Scan(&r.ID, &repo, &r.Module, &r.Done, &r.State, &request, &target, &held, &key, &r.Rev, &ticket, &until, &r.Body); err != nil {
				rows.Close()
				return err
			}
			r.Repository, r.Request, r.Target, r.HeldTarget, r.Key, r.ClaimUntil, r.Ticket = text(repo), text(request), text(target), text(held), text(key), text(until), ticket.Int64
			out.Todos = append(out.Todos, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		rows, err = q.QueryContext(ctx, `SELECT r.id, r.rev, r.body FROM todo_revisions r JOIN todos t ON t.id = r.id
			WHERE t.repository IS ? ORDER BY r.id, r.rev`, nullable(repository))
		if err != nil {
			return err
		}
		for rows.Next() {
			var r RevisionRow
			if err := rows.Scan(&r.ID, &r.Rev, &r.Body); err != nil {
				rows.Close()
				return err
			}
			out.Revisions = append(out.Revisions, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		rows, err = q.QueryContext(ctx, "SELECT id, repository, module, on_what, body FROM waits WHERE repository IS ? ORDER BY id", nullable(repository))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var w WaitRow
			var repo sql.NullString
			if err := rows.Scan(&w.ID, &repo, &w.Module, &w.OnWhat, &w.Body); err != nil {
				return err
			}
			w.Repository = text(repo)
			out.Waits = append(out.Waits, w)
		}
		return rows.Err()
	})
	if err != nil {
		return Dump{}, fmt.Errorf("the scheduler's store %s: dump: %w", s.path, err)
	}
	return out, nil
}

// Restore brings a dump back in one transaction: each to-do and wait by its
// id - one the store holds is left as it is and counted present - with its
// columns as dumped; each revision kept of a to-do it inserted; the ticket
// sequence moved up to the dump's when it is behind. A row of another
// repository than the dump's is refused: a dump is one repository's.
func (s *Store) Restore(ctx context.Context, d Dump) (Restored, error) {
	var out Restored
	err := s.inTx(ctx, func(q querier) error {
		inserted := map[string]bool{}
		for _, r := range d.Todos {
			if r.ID == "" || r.Module == "" || r.Body == "" || r.Rev < 1 {
				return fmt.Errorf("%w: a restored to-do needs its id, module, record and revision (%q of %q)", cwmapi.ErrInvalidInput, r.ID, r.Module)
			}
			if r.Repository != d.Repository {
				return fmt.Errorf("%w: to-do %s of repository %q in the dump of %q", cwmapi.ErrInvalidInput, r.ID, r.Repository, d.Repository)
			}
			var ticket any
			if r.Ticket != 0 {
				ticket = r.Ticket
			}
			res, err := q.ExecContext(ctx, `INSERT INTO todos(id, repository, module, done, state, request, target, held_target, key, rev, ticket, claim_until, body)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO NOTHING`,
				r.ID, nullable(r.Repository), r.Module, r.Done, r.State, nullable(r.Request), nullable(r.Target), nullable(r.HeldTarget), nullable(r.Key), r.Rev, ticket, nullable(r.ClaimUntil), r.Body)
			if err != nil {
				return err
			}
			if n, err := res.RowsAffected(); err != nil {
				return err
			} else if n == 1 {
				inserted[r.ID] = true
				out.Todos.Imported++
			} else {
				out.Todos.Present++
			}
		}
		for _, r := range d.Revisions {
			if !inserted[r.ID] {
				continue // a to-do the store held keeps its own revisions
			}
			res, err := q.ExecContext(ctx, "INSERT INTO todo_revisions(id, rev, body) VALUES (?, ?, ?) ON CONFLICT(id, rev) DO NOTHING", r.ID, r.Rev, r.Body)
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n == 1 {
				out.Revisions++
			}
		}
		for _, w := range d.Waits {
			if w.ID == "" || w.Module == "" || w.Body == "" {
				return fmt.Errorf("%w: a restored wait needs its id, module and record (%q of %q)", cwmapi.ErrInvalidInput, w.ID, w.Module)
			}
			if w.Repository != d.Repository {
				return fmt.Errorf("%w: wait %s of repository %q in the dump of %q", cwmapi.ErrInvalidInput, w.ID, w.Repository, d.Repository)
			}
			res, err := q.ExecContext(ctx, "INSERT INTO waits(id, repository, module, on_what, body) VALUES (?, ?, ?, ?, ?) ON CONFLICT(id) DO NOTHING",
				w.ID, nullable(w.Repository), w.Module, w.OnWhat, w.Body)
			if err != nil {
				return err
			}
			if n, err := res.RowsAffected(); err != nil {
				return err
			} else if n == 1 {
				out.Waits.Imported++
			} else {
				out.Waits.Present++
			}
		}
		_, err := q.ExecContext(ctx, "UPDATE counters SET value = max(value, ?) WHERE name = 'ticket'", d.Ticket)
		return err
	})
	if err != nil {
		return Restored{}, fmt.Errorf("the scheduler's store %s: restore: %w", s.path, err)
	}
	return out, nil
}

// Mark is a cheap reading of how far the store has moved: the to-dos it holds,
// the sum of their revisions, its waits and the to-dos claimed now (a lease
// not passed). Two marks
// apart tell about how many changes were made between them (Changes).
type Mark struct {
	Todos   int64 `json:"todos"`
	Revs    int64 `json:"revs"`
	Waits   int64 `json:"waits"`
	Claimed int64 `json:"claimed"`
}

// Changes is about how many writes lie between two marks.
func (m Mark) Changes(since Mark) int64 {
	abs := func(v int64) int64 {
		if v < 0 {
			return -v
		}
		return v
	}
	return abs(m.Todos-since.Todos) + abs(m.Revs-since.Revs) + abs(m.Waits-since.Waits)
}

// Mark reads the store's mark.
func (s *Store) Mark(ctx context.Context) (Mark, error) {
	var m Mark
	q := s.reader(ctx)
	if err := q.QueryRowContext(ctx, "SELECT count(*), coalesce(sum(rev), 0), coalesce(sum(CASE WHEN state = 'claimed' AND NOT done AND (claim_until IS NULL OR claim_until > ?) THEN 1 ELSE 0 END), 0) FROM todos", time.Now().UTC().Format(time.RFC3339Nano)).Scan(&m.Todos, &m.Revs, &m.Claimed); err != nil {
		return m, fmt.Errorf("the scheduler's store %s: mark: %w", s.path, err)
	}
	if err := q.QueryRowContext(ctx, "SELECT count(*) FROM waits").Scan(&m.Waits); err != nil {
		return m, fmt.Errorf("the scheduler's store %s: mark: %w", s.path, err)
	}
	return m, nil
}
