// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"context"
	"errors"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// TestTheStoreKeepsEveryRevision: each write of a to-do keeps its record at the
// revision it made (todos history) - created, claimed
// archived read back oldest first, each as it was; an import keeps the
// revision it brings; a done to-do pruned leaves with its revisions.
func TestTheStoreKeepsEveryRevision(t *testing.T) {
	ctx := context.Background()
	st := open(t, t.TempDir())
	t.Cleanup(func() { _ = st.Close() })
	td, err := st.Create(ctx, api.Todo{Module: "billing", Title: "x", State: api.StateReady})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Claim(ctx, td.ID, func(t *api.Todo) error {
		t.State, t.Claim = api.StateClaimed, &api.Claim{By: cwmapi.Actor{ID: "a", Kind: "agent"}, Until: "2999-01-01T00:00:00Z"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Archive(ctx, td.ID, func(t *api.Todo) error { t.State, t.Archived = api.StateReported, "2026-11-04T00:00:00Z"; return nil }); err != nil {
		t.Fatal(err)
	}
	revs, err := st.Revisions(ctx, td.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(revs) != 3 {
		t.Fatalf("revisions %d; want 3 (created, claimed, archived): %+v", len(revs), revs)
	}
	for i, want := range []string{api.StateReady, api.StateClaimed, api.StateReported} {
		if revs[i].Revision != int64(i+1) || revs[i].State != want || revs[i].Repository != "repo-1" {
			t.Errorf("revision %d: rev %d state %s repo %q; want rev %d %s repo-1", i, revs[i].Revision, revs[i].State, revs[i].Repository, i+1, want)
		}
	}
	if revs[1].Claim == nil || revs[1].Claim.Ticket == 0 || revs[0].Archived != "" || revs[2].Archived == "" {
		t.Errorf("each revision as written: %+v", revs)
	}

	if _, err := st.ImportTodos(ctx, []api.Todo{{ID: "billing.TD-imported", Module: "billing", State: api.StateReady, Revision: 7}}, false); err != nil {
		t.Fatal(err)
	}
	if got, err := st.Revisions(ctx, "billing.TD-imported"); err != nil || len(got) != 1 || got[0].Revision != 7 {
		t.Errorf("an imported to-do's revisions: %+v (%v); want its revision 7", got, err)
	}

	if err := st.RemoveDone(ctx, td.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := st.db.QueryRow("SELECT count(*) FROM todo_revisions WHERE id = ?", td.ID).Scan(&n); err != nil || n != 0 {
		t.Errorf("revisions left after the prune: %d (%v)", n, err)
	}
	if _, err := st.Revisions(ctx, td.ID); !errors.Is(err, cwmapi.ErrNotFound) {
		t.Errorf("revisions of a pruned to-do: %v; want not found", err)
	}
}
