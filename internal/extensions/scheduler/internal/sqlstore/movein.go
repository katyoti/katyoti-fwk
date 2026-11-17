// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// THE MOVE OF THE PLANNING FILES IN - the files of the
// modules' .ktw/todo, .ktw/done and .ktw/waiting are the truth until the
// migration ends: each run makes the store's rows say what the files say.
// Unlike an import from the export ref (ImportTodos), a row the store already
// holds is brought up to its file when the file is further on - a later
// revision, or the same revision moved to done (a to-do brought in open by a
// partial run, archived since) - so a run after a partial one, or after work
// went on in the files, completes it, and a run again changes nothing. A row
// further on than its file (the store's own later write) is kept, named.

// MovedIn is what a move of planning files into the store did.
type MovedIn struct {
	Added     int `json:"added"`
	Updated   int `json:"updated"`
	Unchanged int `json:"unchanged"`
	// Kept are the rows further on than their file, left as they are.
	Kept []string `json:"kept,omitempty"`
}

// memoRepo is RepoOf asked once per module.
func (s *Store) memoRepo() func(ctx context.Context, module string) (string, error) {
	seen := map[string]string{}
	return func(ctx context.Context, module string) (string, error) {
		if r, ok := seen[module]; ok {
			return r, nil
		}
		r, err := s.opt.RepoOf(ctx, module)
		if err != nil {
			return "", err
		}
		seen[module] = r
		return r, nil
	}
}

// MoveInTodos brings the to-dos of planning files in - done marks them done -
// in one transaction: a to-do the store does not hold is added at its
// revision; one it holds is rewritten when its file is further on (a later
// revision, or done while the row is open), left as it is when equal, kept
// and named when the row is further on.
func (s *Store) MoveInTodos(ctx context.Context, todos []api.Todo, done bool) (MovedIn, error) {
	var out MovedIn
	repoOf := s.memoRepo()
	err := s.inTx(ctx, func(q querier) error {
		var top int64
		for _, t := range todos {
			if t.ID == "" || t.Module == "" {
				return fmt.Errorf("%w: a to-do moved in needs its id and its module (%q of %q)", cwmapi.ErrInvalidInput, t.ID, t.Module)
			}
			repo, err := repoOf(ctx, t.Module)
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
			var rowRev int64
			var rowDone bool
			var rowBody string
			err = q.QueryRowContext(ctx, "SELECT rev, done, body FROM todos WHERE id = ?", t.ID).Scan(&rowRev, &rowDone, &rowBody)
			switch {
			case errors.Is(err, sql.ErrNoRows):
				if _, err := q.ExecContext(ctx, `INSERT INTO todos(id, repository, module, done, state, request, target, held_target, key, rev, ticket, claim_until, body)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					t.ID, nullable(repo), t.Module, done, t.State, request, target, held, nullable(t.Key), t.Revision, ticketOf(t), untilOf(t), body); err != nil {
					return err
				}
				out.Added++
			case err != nil:
				return err
			case rowRev == t.Revision && rowDone == done && rowBody == body:
				out.Unchanged++
				continue
			case t.Revision > rowRev || (t.Revision == rowRev && done && !rowDone):
				if _, err := q.ExecContext(ctx, `UPDATE todos SET repository = ?, module = ?, done = ?, state = ?, request = ?, target = ?, held_target = ?, key = ?, rev = ?, ticket = ?, claim_until = ?, body = ?
					WHERE id = ?`,
					nullable(repo), t.Module, done, t.State, request, target, held, nullable(t.Key), t.Revision, ticketOf(t), untilOf(t), body, t.ID); err != nil {
					return err
				}
				out.Updated++
			default:
				out.Kept = append(out.Kept, t.ID)
				continue
			}
			if err := keep(ctx, q, t.ID, t.Revision, body); err != nil {
				return err
			}
			if t.Claim != nil && t.Claim.Ticket > top {
				top = t.Claim.Ticket
			}
		}
		_, err := q.ExecContext(ctx, "UPDATE counters SET value = max(value, ?) WHERE name = 'ticket'", top)
		return err
	})
	if err != nil {
		return MovedIn{}, err
	}
	return out, nil
}

// MoveInWaits brings the waits of planning files in, in one transaction: a
// wait the store does not hold is added; one it holds with another record is
// rewritten when overwrite (the files are still the truth), kept and named
// otherwise (a wait file written after the migration ended).
func (s *Store) MoveInWaits(ctx context.Context, waits []api.Wait, overwrite bool) (MovedIn, error) {
	var out MovedIn
	repoOf := s.memoRepo()
	err := s.inTx(ctx, func(q querier) error {
		for _, w := range waits {
			if w.ID == "" || w.Module == "" {
				return fmt.Errorf("%w: a wait moved in needs its id and its module (%q of %q)", cwmapi.ErrInvalidInput, w.ID, w.Module)
			}
			repo, err := repoOf(ctx, w.Module)
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
			var rowBody string
			err = q.QueryRowContext(ctx, "SELECT body FROM waits WHERE id = ?", w.ID).Scan(&rowBody)
			switch {
			case errors.Is(err, sql.ErrNoRows):
				if _, err := q.ExecContext(ctx, "INSERT INTO waits(id, repository, module, on_what, body) VALUES (?, ?, ?, ?, ?)", w.ID, nullable(repo), w.Module, onRef, string(b)); err != nil {
					return err
				}
				out.Added++
			case err != nil:
				return err
			case rowBody == string(b):
				out.Unchanged++
			case overwrite:
				if _, err := q.ExecContext(ctx, "UPDATE waits SET repository = ?, module = ?, on_what = ?, body = ? WHERE id = ?", nullable(repo), w.Module, onRef, string(b), w.ID); err != nil {
					return err
				}
				out.Updated++
			default:
				out.Kept = append(out.Kept, w.ID)
			}
		}
		return nil
	})
	if err != nil {
		return MovedIn{}, err
	}
	return out, nil
}

// Row is what the store holds of a to-do: its revision and whether it is done.
type Row struct {
	Rev  int64
	Done bool
}

// Rows answers, for each id the store holds, its row; an id it does not hold
// is absent.
func (s *Store) Rows(ctx context.Context) (map[string]Row, error) {
	rows, err := s.reader(ctx).QueryContext(ctx, "SELECT id, rev, done FROM todos")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Row{}
	for rows.Next() {
		var id string
		var r Row
		if err := rows.Scan(&id, &r.Rev, &r.Done); err != nil {
			return nil, err
		}
		out[id] = r
	}
	return out, rows.Err()
}

// Held is a row the store holds, with its module: a to-do (open or done) or a
// wait.
type Held struct {
	ID     string
	Module string
	Done   bool // a done to-do
	Wait   bool // a wait, not a to-do
}

// HeldRows is every to-do and wait row the store holds, each with its module:
// what the migration's sweep compares with the files of the modules it read.
func (s *Store) HeldRows(ctx context.Context) ([]Held, error) {
	var out []Held
	for _, q := range []struct {
		sql  string
		wait bool
	}{{"SELECT id, module, done FROM todos", false}, {"SELECT id, module, 0 FROM waits", true}} {
		rows, err := s.reader(ctx).QueryContext(ctx, q.sql)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			h := Held{Wait: q.wait}
			if err := rows.Scan(&h.ID, &h.Module, &h.Done); err != nil {
				_ = rows.Close()
				return nil, err
			}
			out = append(out, h)
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// WaitIDs is the id of every wait the store holds.
func (s *Store) WaitIDs(ctx context.Context) (map[string]bool, error) {
	rows, err := s.reader(ctx).QueryContext(ctx, "SELECT id FROM waits")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}
