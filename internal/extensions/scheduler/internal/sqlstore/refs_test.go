// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

const refRepo = "00000000-0000-7000-8000-000000000001"

// TestTheReferenceColumnsKeepFullNames: a to-do's request, target and object
// wait, and a wait's request, are kept as full names - each parses with
// cwmapi.ParseFullName to the row's repository and the authored id - so a
// renamed project key changes nothing; an id qualified by another
// repository's key is refused; a file of the first schema step has its
// references brought to full names by the second.
func TestTheReferenceColumnsKeepFullNames(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st := openWith(t, dir, repo(refRepo))
	td, err := st.Create(ctx, api.Todo{Module: "billing", Target: "billing.F-001", Request: "billing.RQ-001", Title: "x", State: api.StateReady})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Update(ctx, td.ID, func(t *api.Todo) error {
		t.Held = &api.HeldOn{Kind: api.HeldObject, Target: "shipping.RQ-002", Reason: "its answer"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddWait(ctx, "billing", "shipping.RQ-002", func(id string) api.Wait {
		return api.Wait{ID: id, Module: "billing", On: "shipping.RQ-002"}
	}); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"request": "billing.RQ-001", "target": "billing.F-001", "held_target": "shipping.RQ-002"}
	for col, id := range want {
		var got string
		if err := st.db.QueryRow("SELECT "+col+" FROM todos WHERE id = ?", td.ID).Scan(&got); err != nil {
			t.Fatalf("%s: %v", col, err)
		}
		if p, back, err := cwmapi.ParseFullName(got); err != nil || p != refRepo || back != id {
			t.Errorf("%s = %q: want the full name of %s (%v)", col, got, id, err)
		}
	}
	var on string
	if err := st.db.QueryRow("SELECT on_what FROM waits").Scan(&on); err != nil || on != cwmapi.FullName(refRepo, "shipping.RQ-002") {
		t.Errorf("a wait's on_what = %q (%v), want its full name", on, err)
	}
	// The record keeps the authored id: the API speaks it.
	if got, _ := st.Get(ctx, td.ID); got.Target != "billing.F-001" || got.Request != "billing.RQ-001" {
		t.Errorf("read back %+v: want the authored ids", got)
	}
	if _, err := st.Create(ctx, api.Todo{Module: "billing", Target: "other:billing.F-001", Title: "x", State: api.StateReady}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Errorf("a key-qualified target: %v, want refused", err)
	}

	// A file at the first schema step: its rows are brought to full names.
	old := t.TempDir()
	db, err := sql.Open("sqlite", "file:"+old+"/"+File)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(steps[0].sql); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 1;
		INSERT INTO todos(id, repository, module, state, request, rev, body) VALUES ('billing.TD-1', '` + refRepo + `', 'billing', 'ready', 'billing.RQ-001', 1,
			'{"id":"billing.TD-1","module":"billing","target":"billing.F-001","held":{"kind":"person","target":"jdoe"}}');
		INSERT INTO waits(id, repository, module, on_what, body) VALUES ('billing.WT-1', '` + refRepo + `', 'billing', 'shipping.RQ-002', '{}')`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	up := openWith(t, old, repo(refRepo))
	var req, target string
	var held sql.NullString
	if err := up.db.QueryRow("SELECT request, target, held_target FROM todos").Scan(&req, &target, &held); err != nil {
		t.Fatal(err)
	}
	if req != cwmapi.FullName(refRepo, "billing.RQ-001") || target != cwmapi.FullName(refRepo, "billing.F-001") || held.Valid {
		t.Errorf("after the schema step: request %q target %q held %v; want full names, no held object", req, target, held)
	}
	if err := up.db.QueryRow("SELECT on_what FROM waits").Scan(&on); err != nil || on != cwmapi.FullName(refRepo, "shipping.RQ-002") {
		t.Errorf("after the schema step: on_what %q (%v)", on, err)
	}
}
