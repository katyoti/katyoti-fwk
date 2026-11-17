// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

func open(t *testing.T, dir string) *Store {
	t.Helper()
	var n atomic.Int64
	st, err := Open(dir, Options{
		NewID:  func(module, kind string) string { return fmt.Sprintf("%s.%s-%06d", module, kind, n.Add(1)) },
		Now:    func() string { return time.Now().UTC().Format(time.RFC3339Nano) },
		RepoOf: func(context.Context, string) (string, error) { return "repo-1", nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// TestTheSchemaIsRunOnceUnderUserVersion: a new file runs every schema step,
// each recorded with its pack, PRAGMA user_version at the last; opening it
// again runs none and finds what was written; a file from a newer program is
// refused, naming it.
func TestTheSchemaIsRunOnceUnderUserVersion(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st := open(t, dir)
	td, err := st.Create(ctx, api.Todo{Module: "billing", Title: "x", State: api.StateReady})
	if err != nil {
		t.Fatal(err)
	}
	var version, n int
	var pack string
	if err := st.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != len(steps) {
		t.Fatalf("user_version = %d (%v), want %d", version, err, len(steps))
	}
	if err := st.db.QueryRow("SELECT count(*), max(pack) FROM schema_steps").Scan(&n, &pack); err != nil || n != len(steps) || pack != Pack {
		t.Fatalf("schema steps = %d of %q (%v)", n, pack, err)
	}
	var mode string
	if err := st.db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("journal mode = %q (%v), want wal", mode, err)
	}
	_ = st.Close()

	again := open(t, dir)
	if got, err := again.Get(ctx, td.ID); err != nil || got.Repository != "repo-1" || got.Revision != 1 {
		t.Fatalf("reopened: %+v (%v)", got, err)
	}
	if err := again.db.QueryRow("SELECT count(*) FROM schema_steps").Scan(&n); err != nil || n != len(steps) {
		t.Fatalf("schema steps after reopening = %d (%v), want %d", n, err, len(steps))
	}
	if _, err := again.db.Exec(fmt.Sprintf("PRAGMA user_version = %d", len(steps)+1)); err != nil {
		t.Fatal(err)
	}
	_ = again.Close()
	if _, err := Open(dir, Options{NewID: func(m, k string) string { return m }, Now: func() string { return "" }, RepoOf: func(context.Context, string) (string, error) { return "", nil }}); err == nil || !strings.Contains(err.Error(), File) {
		t.Fatalf("a newer schema: %v, want refused naming the file", err)
	}
}

// TestEveryPooledConnectionIsSynchronousFull: the store is opened with
// synchronous=FULL on every connection of its pool - an acknowledged write is
// on disk before the commit returns, so it survives an OS crash or a power
// cut. Two connections held at once are both read: the setting rides on the
// DSN, applied to each new connection, so one connection alone could miss a
// pool path.
func TestEveryPooledConnectionIsSynchronousFull(t *testing.T) {
	ctx := context.Background()
	st := open(t, t.TempDir())
	defer st.Close()
	first, err := st.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := st.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	for i, c := range []interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	}{first, second} {
		var mode int
		if err := c.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&mode); err != nil || mode != 2 {
			t.Fatalf("connection %d: PRAGMA synchronous = %d (%v), want 2 (FULL)", i+1, mode, err)
		}
	}
	if s := st.db.Stats(); s.InUse < 2 {
		t.Fatalf("connections in use = %d, want two held at once", s.InUse)
	}
}
