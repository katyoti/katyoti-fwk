// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/journal/api"
	journalcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/journal/contrib"
	journallocal "github.com/katyoti/katyoti-fwk/internal/extensions/journal/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
	"github.com/katyoti/katyoti-fwk/internal/test/hubharness"
)

// member makes a repository holding a project object of key and a module m.
func member(t *testing.T, root, key string) string {
	t.Helper()
	dir := filepath.Join(root, "repo-"+key)
	if err := os.MkdirAll(filepath.Join(dir, "modules", "m"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, cwmapi.ProjectFileName), []byte("id: "+key+"\ntype: project\nname: "+key+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// ONE JOURNAL PER PROJECT. Two served
// projects, alpha and beta, each hold; alpha alone holds. Each
// write is one record, in the journal of the project holding the object
// (<key>:sys-journal); Times and History on a qualified id answer that
// project's own records, and a bare id held by one project reads that one.
func TestEachProjectKeepsItsOwnJournal(t *testing.T) {
	root := hubharness.TempDir(t)
	alpha, beta := member(t, root, "alpha"), member(t, root, "beta")
	ctx, cwm := cwmapp.Members(t, root, []string{alpha, beta},
		application.DeclareSimpleService[eventbus.EventBus](func(context.Context) (eventbus.EventBus, error) {
			return eventbus.NewMemoryEventBus(), nil
		}),
		journalcontrib.Pack(""),
		cwmapp.SystemModules(t),
		journallocal.Extension(),
	)
	svc, err := application.UseService[api.JournalService](ctx)
	if err != nil {
		t.Fatalf("resolve JournalService: %v", err)
	}
	note := func(module, id string) {
		t.Helper()
		o := cwmapi.Object{ID: id, Type: "note"}
		o.Name = id
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: o}); err != nil {
			t.Fatalf("create %s in %s: %v", id, module, err)
		}
	}
	// historyOf waits for the observer to have written the object's records.
	historyOf := func(id string, n int) []api.Record {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			out, err := svc.History(ctx, api.HistoryInput{ID: id})
			if err != nil {
				t.Fatalf("History(%s): %v", id, err)
			}
			if len(out.Records) >= n || time.Now().After(deadline) {
				return out.Records
			}
			time.Sleep(20 * time.Millisecond)
		}
	}

	note("alpha:m", "m.F-1")
	historyOf("alpha:m.F-1", 1)
	time.Sleep(50 * time.Millisecond) // beta's is created later
	note("beta:m", "m.F-1")
	historyOf("beta:m.F-1", 1)
	note("alpha:m", "m.F-2")
	historyOf("m.F-2", 1)
	time.Sleep(200 * time.Millisecond) // a second record of a write would land by now

	// Each write is one record, in its own project's journal.
	all, err := svc.Query(ctx, api.QueryInput{Module: "m"})
	if err != nil {
		t.Fatal(err)
	}
	events := map[string]bool{}
	for _, r := range all.Records {
		if events[r.EventID] {
			t.Errorf("event %s recorded twice", r.EventID)
		}
		events[r.EventID] = true
	}
	if len(all.Records) != 3 {
		t.Errorf("every project's journal holds %d records about m, want 3 (one per write): %+v", len(all.Records), all.Records)
	}
	for key, want := range map[string]int{"alpha": 2, "beta": 1} {
		got, err := svc.Query(ctx, api.QueryInput{Module: key + ":m"})
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Records) != want {
			t.Errorf("%s's journal holds %d records about m, want %d", key, len(got.Records), want)
		}
		if _, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: key + ":" + api.Module}); err != nil {
			t.Errorf("%s has no journal of its own: %v", key, err)
		}
	}

	// History on a qualified id holds that project's records only.
	ha, hb := historyOf("alpha:m.F-1", 1), historyOf("beta:m.F-1", 1)
	if len(ha) != 1 || len(hb) != 1 {
		t.Fatalf("History(alpha:m.F-1) = %d records, History(beta:m.F-1) = %d; want one each", len(ha), len(hb))
	}
	if ha[0].EventID == hb[0].EventID || !hb[0].At.After(ha[0].At) {
		t.Fatalf("beta's record %+v is not its own, created after alpha's %+v", hb[0], ha[0])
	}

	// Times on each qualified id answers its own project's times.
	for id, rec := range map[string]api.Record{"alpha:m.F-1": ha[0], "beta:m.F-1": hb[0]} {
		got, err := svc.Times(ctx, api.TimesInput{ID: id})
		if err != nil {
			t.Fatalf("Times(%s): %v", id, err)
		}
		if !got.CreatedAt.Equal(rec.At) || !got.UpdatedAt.Equal(rec.At) {
			t.Errorf("Times(%s) = created %v updated %v, want its own %v", id, got.CreatedAt, got.UpdatedAt, rec.At)
		}
	}
	// A bare id held by one project still answers.
	if got, err := svc.Times(ctx, api.TimesInput{ID: "m.F-2"}); err != nil || got.CreatedAt.IsZero() {
		t.Errorf("Times(m.F-2) = %+v, %v; want alpha's record", got, err)
	}

	// An action run is recorded in the journal of the project holding the
	// object: named by its qualified id, or bare when one project holds it.
	for _, id := range []string{"beta:m.F-1", "m.F-2"} {
		if _, err := svc.Invoked(ctx, api.InvokedInput{ID: id, URI: "ktw-test:do", Status: "ok"}); err != nil {
			t.Fatalf("Invoked(%s): %v", id, err)
		}
	}
	for key, want := range map[string]string{"alpha": "m.F-2", "beta": "m.F-1"} {
		got, err := svc.Query(ctx, api.QueryInput{Module: key + ":m", Actions: []string{api.ActionInvoked}})
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Records) != 1 || got.Records[0].ID != want {
			t.Errorf("%s's journal holds the action runs %+v, want one on %s", key, got.Records, want)
		}
	}
}
