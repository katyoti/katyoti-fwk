// SPDX-License-Identifier: Apache-2.0

package core

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// : the scheduler's SQLite store keys its rows through the
// real repositoryOf - the requests module no project serves answers no
// repository (NULL); any other module that resolves to no object, and any
// module whose project has no uuid, is refused, naming it.

// TestOpenSQLiteStoreKeysAProjectlessRequestsModuleNull: in a universe that
// serves no project, a holding-list to-do of the requests module (a delegated
// child's question to its parent) is put through OpenSQLiteStore
// and the real repositoryOf, stored with a NULL repository and read back; a
// to-do of an unknown other module, or of a module whose project has no uuid,
// is refused, naming it.
func TestOpenSQLiteStoreKeysAProjectlessRequestsModuleNull(t *testing.T) {
	ctx, cwm := cwmapp.New(t, cwmapp.Pack(fstest.MapFS{"object-types/module.yaml": {Data: []byte("name: module\ncapabilities: [module]\n")}}))
	st, err := OpenSQLiteStore(schedulerEnv{dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	held, err := st.Create(ctx, api.Todo{Module: api.RequestsModule, Title: "the child's question to its parent", Owner: "test", For: api.ForAgent, Start: api.StartAuto, Priority: "P1", State: api.StateReady, Key: "ask-1"})
	if err != nil {
		t.Fatalf("a holding-list to-do of %s in a universe serving no project: %v, want stored", api.RequestsModule, err)
	}
	got, err := st.List(ctx, api.RequestsModule, false)
	if err != nil || len(got) != 1 || got[0].ID != held.ID || got[0].Repository != "" {
		t.Fatalf("%s's to-dos = %+v (%v), want %s of no repository", api.RequestsModule, got, err, held.ID)
	}
	db, err := sql.Open("sqlite", st.Path())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var repo *string
	if err := db.QueryRowContext(ctx, "SELECT repository FROM todos WHERE id = ?", held.ID).Scan(&repo); err != nil || repo != nil {
		t.Fatalf("its repository = %v (%v), want NULL", repo, err)
	}

	_, err = st.Create(ctx, api.Todo{Module: "nosuch", Title: "lost", State: api.StateReady, Key: "k"})
	if err == nil || !errors.Is(err, cwmapi.ErrNotFound) || !strings.Contains(err.Error(), "nosuch") {
		t.Fatalf("a to-do of a module resolving to no object = %v, want refused naming nosuch", err)
	}
	cwmapp.Module(t, ctx, cwm, "billing") // a module of a project with no uuid
	_, err = st.Create(ctx, api.Todo{Module: "billing", Title: "invoice", State: api.StateReady, Key: "kb"})
	if !errors.Is(err, cwmapi.ErrInvalidInput) || !strings.Contains(err.Error(), "billing") {
		t.Fatalf("a to-do of a module whose project has no uuid = %v, want refused naming billing", err)
	}
}
