// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/journal/api"
	journalcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/journal/contrib"
	journallocal "github.com/katyoti/katyoti-fwk/internal/extensions/journal/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// THE JOURNAL, end to end: CWM writes → the in-process observer → one record
// per write in sys-journal → the queries.

func setup(t *testing.T) (context.Context, cwmapi.CWMService, api.JournalService) {
	t.Helper()
	ctx, cwm := cwmapp.New(t,
		// The in-process bus CWM publishes on (a host binary registers it).
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
	cwmapp.Module(t, ctx, cwm, "billing")
	cwmapp.Module(t, ctx, cwm, "payments")
	return ctx, cwm, svc
}

// history waits for the observer (its own goroutine) to have written n records
// about id, and returns them.
func history(t *testing.T, ctx context.Context, svc api.JournalService, id string, n int) []api.Record {
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

func actions(recs []api.Record) []string {
	out := make([]string, len(recs))
	for i, r := range recs {
		out[i] = r.Action
	}
	return out
}

func create(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, module, id, title string) cwmapi.Object {
	t.Helper()
	o := cwmapi.Object{ID: id, Type: "note"}
	o.Name = title
	o.Parent = module
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: o}); err != nil {
		t.Fatalf("create %s: %v", id, err)
	}
	return o
}

// TestACreateAndAnUpdateAreOneRecordEach: a tree object publishes on the item
// AND the node stream; the journal keeps one record per write, newest first.
func TestACreateAndAnUpdateAreOneRecordEach(t *testing.T) {
	ctx, cwm, svc := setup(t)
	o := create(t, ctx, cwm, "billing", "billing.invoices", "Invoices")
	o.Description = "sent monthly"
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "billing", Object: o}); err != nil {
		t.Fatal(err)
	}
	recs := history(t, ctx, svc, "billing.invoices", 2)
	time.Sleep(100 * time.Millisecond) // a duplicate would arrive by now
	recs = history(t, ctx, svc, "billing.invoices", 2)
	if got := actions(recs); len(got) != 2 || got[0] != api.ActionUpdated || got[1] != api.ActionCreated {
		t.Fatalf("history = %v, want [updated created]", got)
	}
	if recs[1].Module != "billing" || recs[1].Type != "note" || recs[1].Parent != "billing" {
		t.Errorf("created record = %+v", recs[1])
	}
}

// TestAMoveIsOneRecordAndHistoryFollowsIt: a move across modules (a new id: a
// create and a delete underneath) is ONE `moved` record; the history of the new
// id reaches back to the old one's.
func TestAMoveIsOneRecordAndHistoryFollowsIt(t *testing.T) {
	ctx, cwm, svc := setup(t)
	create(t, ctx, cwm, "billing", "billing.refunds", "Refunds")
	history(t, ctx, svc, "billing.refunds", 1)
	out, err := cwm.ObjectMove(ctx, cwmapi.ObjectMoveInput{ID: "billing.refunds", NewParent: "payments", NewModule: "payments", Reason: "refunds are a payments concern"})
	if err != nil {
		t.Fatalf("ObjectMove: %v", err)
	}
	recs := history(t, ctx, svc, out.ID, 2)
	time.Sleep(100 * time.Millisecond)
	recs = history(t, ctx, svc, out.ID, 2)
	if got := actions(recs); len(got) != 2 || got[0] != api.ActionMoved || got[1] != api.ActionCreated {
		t.Fatalf("history of %s = %v, want [moved created] — the move's own create/delete must not be recorded", out.ID, got)
	}
	m := recs[0]
	if m.FromID != "billing.refunds" || m.Module != "payments" || m.FromModule != "billing" || m.Reason == "" {
		t.Errorf("moved record = %+v", m)
	}
	// Nothing about the old id beyond its creation: no `deleted`.
	old, _ := svc.Query(ctx, api.QueryInput{ObjectID: "billing.refunds", Actions: []string{api.ActionDeleted, api.ActionCreated}})
	for _, r := range old.Records {
		if r.Action == api.ActionDeleted {
			t.Errorf("the move's delete was recorded: %+v", r)
		}
	}
}

// TestTheJournalDoesNotRecordItself: sys-journal's own writes (its days) are
// not in the journal, and the day object holds the records.
func TestTheJournalDoesNotRecordItself(t *testing.T) {
	ctx, cwm, svc := setup(t)
	create(t, ctx, cwm, "billing", "billing.dunning", "Dunning")
	history(t, ctx, svc, "billing.dunning", 1)
	out, err := svc.Query(ctx, api.QueryInput{Module: api.Module})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Records) != 0 {
		t.Fatalf("the journal recorded its own module: %+v", out.Records)
	}
	day := api.DayID(time.Now())
	if _, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: api.Module, ID: day}); err != nil {
		t.Fatalf("no journal-day %s: %v", day, err)
	}
}

// TestQueryFiltersByModuleAndAction.
func TestQueryFiltersByModuleAndAction(t *testing.T) {
	ctx, cwm, svc := setup(t)
	create(t, ctx, cwm, "billing", "billing.a", "A")
	create(t, ctx, cwm, "payments", "payments.b", "B")
	history(t, ctx, svc, "payments.b", 1)
	out, err := svc.Query(ctx, api.QueryInput{Module: "payments", Actions: []string{api.ActionCreated}})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range out.Records {
		if r.Module != "payments" || r.Action != api.ActionCreated {
			t.Errorf("filter leaked %+v", r)
		}
	}
	if len(out.Records) == 0 {
		t.Fatal("no payments record")
	}
}

// TestASplitIsOneRecordAndHistoryCrossesIt: a split that removes its source is
// ONE `split` record (no `deleted` for the source); a successor's history
// reaches back into the source's.
func TestASplitIsOneRecordAndHistoryCrossesIt(t *testing.T) {
	ctx, cwm, svc := setup(t)
	create(t, ctx, cwm, "billing", "billing.export", "Export")
	history(t, ctx, svc, "billing.export", 1)
	out, err := cwm.ObjectSplit(ctx, cwmapi.ObjectSplitInput{ID: "billing.export", Reason: "two formats", DeleteSource: true,
		Successors: []cwmapi.ObjectSplitTarget{{Type: "note", Title: "Export PDF"}, {Type: "note", Title: "Export CSV"}}})
	if err != nil {
		t.Fatalf("ObjectSplit: %v", err)
	}
	recs := history(t, ctx, svc, out.Successors[0], 3)
	time.Sleep(100 * time.Millisecond)
	recs = history(t, ctx, svc, out.Successors[0], 3)
	// Newest first: the split (on the source), the successor's create, the
	// source's create.
	if got := actions(recs); len(got) != 3 || got[0] != api.ActionSplit || got[1] != api.ActionCreated || got[2] != api.ActionCreated {
		t.Fatalf("history of %s = %v, want [split created created]", out.Successors[0], got)
	}
	src, _ := svc.History(ctx, api.HistoryInput{ID: "billing.export"})
	var split *api.Record
	for i := range src.Records {
		switch src.Records[i].Action {
		case api.ActionDeleted:
			t.Errorf("the split's delete of its source was recorded: %+v", src.Records[i])
		case api.ActionSplit:
			split = &src.Records[i]
		}
	}
	if split == nil || len(split.Successors) != 2 || split.SourceKept == nil || *split.SourceKept || split.Reason != "two formats" {
		t.Fatalf("split record = %+v", split)
	}
}

// TestARecordSaysWhyAndWhichRevision: a write made under a cause records it
// (kind, id, depth) with the event's id and the revision it left; a write
// made under none records exactly as before — no cause key on its line; a
// move under a cause is still ONE moved record, carrying it.
func TestARecordSaysWhyAndWhichRevision(t *testing.T) {
	ctx, cwm, svc := setup(t)
	who := cause.Cause{Kind: cause.KindAgent, ID: "run-42", Depth: 1}
	caused := cause.WithCause(ctx, who)
	o := create(t, caused, cwm, "billing", "billing.why", "Why")
	o.Description = "changed with no cause"
	upd, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "billing", Object: o})
	if err != nil {
		t.Fatal(err)
	}
	recs := history(t, ctx, svc, "billing.why", 2)
	if got := actions(recs); len(got) != 2 || got[0] != api.ActionUpdated || got[1] != api.ActionCreated {
		t.Fatalf("history = %v, want [updated created]", got)
	}
	created, updated := recs[1], recs[0]
	if created.Cause == nil || *created.Cause != who {
		t.Fatalf("the caused create records cause %+v, want %+v", created.Cause, who)
	}
	if created.EventID == "" || created.Revision == "" || created.At.IsZero() {
		t.Fatalf("the create record lacks its event id, revision or time: %+v", created)
	}
	if updated.Cause != nil {
		t.Fatalf("a write with no cause recorded one: %+v", updated.Cause)
	}
	if updated.Revision != upd.Revision || updated.EventID == created.EventID {
		t.Fatalf("the update record = %+v, want revision %s and its own event id", updated, upd.Revision)
	}
	if updated.ID != "billing.why" || updated.Module != "billing" || updated.Type != "note" || updated.Parent != "billing" {
		t.Errorf("the uncaused record changed shape: %+v", updated)
	}

	// On the line itself: the caused write carries the cause, the other has no
	// cause key at all — the JSON of uncaused writes is as it was.
	read, err := cwm.ObjectNamedFileRead(ctx, cwmapi.ObjectNamedFileReadInput{ID: api.DayID(time.Now()), Name: api.EntriesFile})
	if err != nil {
		t.Fatal(err)
	}
	var createdLine, updatedLine string
	for _, line := range strings.Split(string(read.Content), "\n") {
		if !strings.Contains(line, `"id":"billing.why"`) {
			continue
		}
		switch {
		case strings.Contains(line, `"action":"created"`):
			createdLine = line
		case strings.Contains(line, `"action":"updated"`):
			updatedLine = line
		}
	}
	if !strings.Contains(createdLine, `"cause":{"kind":"agent","id":"run-42","depth":1}`) {
		t.Errorf("the caused line = %s, want the cause as kind/id/depth", createdLine)
	}
	if updatedLine == "" || strings.Contains(updatedLine, `"cause"`) {
		t.Errorf("the uncaused line = %s, want no cause key", updatedLine)
	}

	// A move under a cause: ONE moved record, carrying it.
	out, err := cwm.ObjectMove(caused, cwmapi.ObjectMoveInput{ID: "billing.why", NewParent: "payments", NewModule: "payments", Reason: "payments asks why"})
	if err != nil {
		t.Fatalf("ObjectMove: %v", err)
	}
	recs = history(t, ctx, svc, out.ID, 3)
	time.Sleep(100 * time.Millisecond)
	recs = history(t, ctx, svc, out.ID, 3)
	if got := actions(recs); len(got) != 3 || got[0] != api.ActionMoved {
		t.Fatalf("history of %s = %v, want [moved updated created] — one record for the move", out.ID, got)
	}
	if recs[0].Cause == nil || *recs[0].Cause != who || recs[0].Revision == "" || recs[0].EventID == "" {
		t.Errorf("the moved record = %+v, want the cause %+v, a revision and an event id", recs[0], who)
	}
}

// TestARaisedEventIsOneEventRecord: an event raised on an object (EventRaise)
// is ONE `event` record — its type, its payload as the note, the cause; an
// undeclared type is refused and records nothing.
func TestARaisedEventIsOneEventRecord(t *testing.T) {
	ctx, cwm := cwmapp.New(t,
		application.DeclareSimpleService[eventbus.EventBus](func(context.Context) (eventbus.EventBus, error) {
			return eventbus.NewMemoryEventBus(), nil
		}),
		journalcontrib.Pack(""),
		cwmapp.SystemModules(t),
		journallocal.Extension(),
		cwmapp.Pack(fstest.MapFS{
			"event-types/code-changed.yaml": {Data: []byte("name: code.changed\npoint: on_code_change\n")},
		}),
	)
	svc, err := application.UseService[api.JournalService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	cwmapp.Module(t, ctx, cwm, "billing")
	create(t, ctx, cwm, "billing", "billing.code", "Code")
	if _, err := cwm.EventRaise(ctx, cwmapi.EventRaiseInput{Type: "code.unknown", ObjectID: "billing.code"}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("an undeclared type = %v, want refused", err)
	}
	who := cause.Cause{Kind: cause.KindAgent, ID: "watcher", Depth: 0}
	out, err := cwm.EventRaise(cause.WithCause(ctx, who), cwmapi.EventRaiseInput{Type: "code.changed", ObjectID: "billing.code",
		Payload: json.RawMessage(`{"paths": ["go.mod"]}`)})
	if err != nil {
		t.Fatal(err)
	}
	recs := history(t, ctx, svc, "billing.code", 2)
	time.Sleep(100 * time.Millisecond) // a record of the refused one would arrive by now
	recs = history(t, ctx, svc, "billing.code", 2)
	if got := actions(recs); len(got) != 2 || got[0] != api.ActionEvent || got[1] != api.ActionCreated {
		t.Fatalf("history = %v, want [event created]", got)
	}
	r := recs[0]
	if r.Event != "code.changed" || r.Note != `{"paths":["go.mod"]}` || r.EventID != out.EventID ||
		r.Cause == nil || *r.Cause != who || r.Module != "billing" || r.Type != "note" {
		t.Errorf("event record = %+v", r)
	}
}
