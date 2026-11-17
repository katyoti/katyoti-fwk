// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	journalapi "github.com/katyoti/katyoti-fwk/internal/extensions/journal/api"
	journalcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/journal/contrib"
	journallocal "github.com/katyoti/katyoti-fwk/internal/extensions/journal/local"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	schedcore "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/internal/core"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// A RUNTIME ROW FOLLOWS THE CONTENT IT NAMES: a move, a split
// or an archive of an object re-points or ends the rows naming it, idempotently;
// what the store missed, the tick catches up from the journal.

// followWorld is the scheduler's world with the in-process bus CWM's observers
// hear on, a note type to move, split and archive, and - journal - the journal.
func followWorld(t *testing.T, journal bool) (context.Context, cwmapi.CWMService, api.TodoService) {
	t.Helper()
	exts := []application.Extension{
		application.DeclareSimpleService[eventbus.EventBus](func(context.Context) (eventbus.EventBus, error) {
			return eventbus.NewMemoryEventBus(), nil
		}),
		cwmapp.Pack(fstest.MapFS{"object-types/note.yaml": {Data: []byte("name: note\n")}}),
		application.DeclareSimpleService[taskapi.TaskService](func(context.Context) (taskapi.TaskService, error) {
			return &fakeTasks{status: map[string]taskapi.RunStatus{}}, nil
		}),
	}
	if journal {
		exts = append(exts, journalcontrib.Pack(""), cwmapp.SystemModules(t), journallocal.Extension())
	}
	ctx, cwm, svc, _ := world(t, exts...)
	return ctx, cwm, svc
}

func note(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, module, id string) {
	t.Helper()
	o := cwmapi.Object{ID: id, Type: "note"}
	o.Name, o.Parent = id, module
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: o}); err != nil {
		t.Fatal(err)
	}
}

// eventually polls cond - the observer hears on its own goroutine.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("not within 5s: %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func get(t *testing.T, ctx context.Context, svc api.TodoService, id string) api.Todo {
	t.Helper()
	out, err := svc.Get(ctx, api.GetInput{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	return out.Todo
}

// A request moved to another module: the to-do's target and request, a
// blocked report's object wait and a module's wait on it name the new id; the
// move's event delivered again writes nothing.
func TestARowFollowsAMovedObject(t *testing.T) {
	ctx, cwm, svc := followWorld(t, false)
	note(t, ctx, cwm, "billing", "billing.rq1")
	td := put(t, ctx, svc, api.Todo{Module: "billing", Target: "billing.rq1", Request: "billing.rq1", Title: "implement"})
	blocked := put(t, ctx, svc, api.Todo{Module: "shipping", Title: "waits", Owner: "uncollected"}) // stays reported
	if _, err := svc.Pull(ctx, api.PullInput{ID: blocked.ID, By: alice}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Report(ctx, api.ReportInput{ID: blocked.ID, Result: api.Result{Status: api.StatusBlocked, Summary: "waits on rq1", By: alice},
		Wait: &api.HeldOn{Kind: api.HeldObject, Target: "billing.rq1", Reason: "its answer"}}); err != nil {
		t.Fatal(err)
	}
	waits, err := application.UseService[api.WaitService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	w, err := waits.Add(ctx, api.AddWaitInput{Module: "shipping", On: "billing.rq1", From: "billing", What: "the answer", By: person})
	if err != nil {
		t.Fatal(err)
	}

	mv, err := cwm.ObjectMove(ctx, cwmapi.ObjectMoveInput{ID: "billing.rq1", NewModule: "shipping", NewParent: "shipping", Reason: "belongs there"})
	if err != nil {
		t.Fatal(err)
	}
	to := mv.ID
	eventually(t, "the to-do re-pointed to "+to, func() bool {
		got := get(t, ctx, svc, td.ID)
		return got.Target == to && got.Request == to
	})
	eventually(t, "the blocked report's wait re-pointed", func() bool {
		got := get(t, ctx, svc, blocked.ID)
		return got.Held != nil && got.Held.Target == to
	})
	eventually(t, "the module's wait re-pointed", func() bool {
		ws, _ := waits.List(ctx, api.ListWaitsInput{Module: "shipping"})
		return len(ws.Waits) == 1 && ws.Waits[0].ID == w.ID && ws.Waits[0].On == to
	})

	before := get(t, ctx, svc, td.ID).Revision
	again, err := schedFollow(ctx, cwmapi.EventEnvelope{Topic: "object." + cwmapi.SubjectToken(to) + ".moved",
		Payload: mustJSON(t, cwmapi.ObjectMovedEventPayload{ObjectID: to, FromID: "billing.rq1", Module: "shipping", FromModule: "billing"})})
	if err != nil || len(again) != 0 {
		t.Fatalf("the move delivered again: %v %v; want nothing written", again, err)
	}
	if rev := get(t, ctx, svc, td.ID).Revision; rev != before {
		t.Errorf("revision %d after the replay, was %d", rev, before)
	}
}

// A split that drops its source ends the open rows naming it, a held one
// keeping its claim (its report still taken); a split keeping its source
// changes nothing; an archive ends the rows.
func TestARowEndsWithASplitOrArchivedObject(t *testing.T) {
	ctx, cwm, svc := followWorld(t, false)
	note(t, ctx, cwm, "billing", "billing.f1")
	note(t, ctx, cwm, "billing", "billing.f2")
	note(t, ctx, cwm, "billing", "billing.f3")
	open := put(t, ctx, svc, api.Todo{Module: "billing", Target: "billing.f1", Title: "verify"})
	heldTd := put(t, ctx, svc, api.Todo{Module: "billing", Target: "billing.f1", Title: "build"})
	if _, err := svc.Pull(ctx, api.PullInput{ID: heldTd.ID, By: alice}); err != nil {
		t.Fatal(err)
	}
	kept := put(t, ctx, svc, api.Todo{Module: "billing", Target: "billing.f2", Title: "verify"})
	archived := put(t, ctx, svc, api.Todo{Module: "billing", Target: "billing.f3", Title: "verify"})

	two := []cwmapi.ObjectSplitTarget{{Type: "note", Title: "part a"}, {Type: "note", Title: "part b"}}
	if _, err := cwm.ObjectSplit(ctx, cwmapi.ObjectSplitInput{ID: "billing.f1", Successors: two, Reason: "two wants", DeleteSource: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := cwm.ObjectSplit(ctx, cwmapi.ObjectSplitInput{ID: "billing.f2", Successors: two, Reason: "two wants"}); err != nil {
		t.Fatal(err)
	}
	if _, err := cwm.ObjectArchive(ctx, cwmapi.ObjectArchiveInput{ID: "billing.f3"}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the open row of the split source ended", func() bool {
		got := get(t, ctx, svc, open.ID)
		return got.Archived != "" && strings.Contains(got.Note, "split into")
	})
	eventually(t, "the row of the archived object ended", func() bool {
		got := get(t, ctx, svc, archived.ID)
		return got.Archived != "" && strings.Contains(got.Note, "archived")
	})
	if got := get(t, ctx, svc, heldTd.ID); got.Archived != "" || got.State != api.StateClaimed {
		t.Errorf("the held row: %+v; want it kept, claimed", got)
	}
	if _, err := svc.Report(ctx, api.ReportInput{ID: heldTd.ID, Result: api.Result{Status: api.StatusSuccess, Summary: "built", By: alice}}); err != nil {
		t.Errorf("the held row's report: %v; want it taken", err)
	}
	if got := get(t, ctx, svc, kept.ID); got.Archived != "" || got.Target != "billing.f2" {
		t.Errorf("a split keeping its source: %+v; want the row unchanged", got)
	}
}

// What the store missed - a to-do still naming an id that moved -, the tick
// follows from the journal; a name the journal has no record of is flagged,
// the row kept.
func TestTheTickCatchesUpWhatTheStoreMissed(t *testing.T) {
	ctx, cwm, svc := followWorld(t, true)
	note(t, ctx, cwm, "billing", "billing.rq2")
	mv, err := cwm.ObjectMove(ctx, cwmapi.ObjectMoveInput{ID: "billing.rq2", NewModule: "shipping", NewParent: "shipping", Reason: "belongs there"})
	if err != nil {
		t.Fatal(err)
	}
	journal := journalOf(t, ctx)
	eventually(t, "the journal records the move", func() bool {
		got, _ := journal.Query(ctx, journalQuery("billing.rq2"))
		return len(got.Records) > 0
	})
	// Put after the move: the observer heard nothing about these rows.
	missed := put(t, ctx, svc, api.Todo{Module: "billing", Target: "billing.rq2", Request: "billing.rq2", Title: "implement"})
	lost := put(t, ctx, svc, api.Todo{Module: "billing", Target: "billing.never-was", Title: "implement"})

	rep, err := svc.Tick(ctx, api.TickInput{})
	if err != nil {
		t.Fatal(err)
	}
	if got := get(t, ctx, svc, missed.ID); got.Target != mv.ID || got.Request != mv.ID {
		t.Errorf("after the tick: %+v (report %+v); want it re-pointed to %s", got, rep, mv.ID)
	}
	if got := get(t, ctx, svc, lost.ID); got.Target != "billing.never-was" || got.Archived != "" {
		t.Errorf("a name with no record: %+v; want the row kept", got)
	}
	if len(rep.Flagged) != 1 || !strings.Contains(rep.Flagged[0], "billing.never-was") {
		t.Errorf("flagged %v; want billing.never-was, once", rep.Flagged)
	}
	if again, _ := svc.Tick(ctx, api.TickInput{}); len(again.Flagged)+len(again.Followed) != 0 {
		t.Errorf("a second tick: %+v; want nothing followed, nothing flagged again", again)
	}
}

func schedFollow(ctx context.Context, ev cwmapi.EventEnvelope) ([]string, error) {
	return schedcore.New().Follow(ctx, ev)
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func journalOf(t *testing.T, ctx context.Context) journalapi.JournalService {
	t.Helper()
	j, err := application.UseService[journalapi.JournalService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func journalQuery(id string) journalapi.QueryInput {
	return journalapi.QueryInput{ObjectID: id, Actions: []string{journalapi.ActionMoved}}
}
