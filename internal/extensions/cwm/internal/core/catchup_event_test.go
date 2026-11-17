// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/events"
)

// openPublishing starts a service on the project - a restart - with an
// in-process bus, and loads its index: the catch-up publishes as in
// production. It answers the grouped events the open published.
func (r *catchUpRepo) openPublishing() (*Service, *captured) {
	r.t.Helper()
	svc, err := New(stdctx.Background(), &CWMServiceConfig{WorkspaceRoot: r.ws, ProjectRoot: r.root})
	if err != nil {
		r.t.Fatalf("New: %v", err)
	}
	r.t.Cleanup(func() { _ = svc.Close() })
	bus := eventbus.NewMemoryEventBus()
	svc.Events = events.New("t", "u", "w", bus, nil)
	var c captured
	_ = bus.Subscribe(api.ProjectCaughtUpSubject("t", "u", "w"), c.on)
	if err := svc.index().ensureLoaded(); err != nil {
		r.t.Fatal(err)
	}
	return svc, &c
}

// caughtUp is the one grouped event published, failing on any other count.
func caughtUp(t *testing.T, c *captured) api.ObjectsCaughtUpEventPayload {
	t.Helper()
	got := c.all()
	if len(got) != 1 {
		t.Fatalf("the catch-up published %d grouped events, want exactly one: %+v", len(got), got)
	}
	p, ok := got[0].(events.ObjectsCaughtUpPayload)
	if !ok {
		t.Fatalf("the grouped event's payload is a %T", got[0])
	}
	if p.EventID == "" || p.At.IsZero() || p.By.Kind != api.ActorOutside {
		t.Fatalf("the grouped event has no id, no time or an actor not outside: %+v", p)
	}
	return p
}

// TestACommittedHandEditPublishesOneGroupedEvent pins grouped
// event: two objects edited by hand and committed outside CWM are caught up at
// the next open as ONE event naming both, its cause the commits between the
// HEAD CWM saw last and this one, its actor outside - the commit's author
// with its sha.
func TestACommittedHandEditPublishesOneGroupedEvent(t *testing.T) {
	r := newCatchUpRepo(t)
	paths := r.seeded("cwm.a", "cwm.b", "cwm.c")
	r.open() // the cursor at the seed's commit
	from := strings.TrimSpace(r.git("rev-parse", "HEAD"))
	r.handEdit(paths["cwm.a"], "name: cwm.a", "name: edited by hand")
	r.handEdit(paths["cwm.b"], "name: cwm.b", "name: edited too")
	r.commit("edited outside")
	sha := strings.TrimSpace(r.git("rev-parse", "HEAD"))

	_, c := r.openPublishing()
	p := caughtUp(t, c)
	if !slices.Equal(p.Modified, []string{"cwm.a", "cwm.b"}) || len(p.Added)+len(p.Removed) != 0 {
		t.Fatalf("the event names added %v, modified %v, removed %v; want [cwm.a cwm.b] modified", p.Added, p.Modified, p.Removed)
	}
	if p.Cause != (api.CaughtUpCause{Kind: api.CaughtUpCommits, From: from, To: sha}) {
		t.Fatalf("cause %+v, want the commits %s..%s", p.Cause, from, sha)
	}
	if p.By.ID != "t" || p.Sha != sha {
		t.Fatalf("actor %+v sha %q, want outside(t, %s)", p.By, p.Sha, sha)
	}

	if _, again := r.openPublishing(); len(again.all()) != 0 {
		t.Fatalf("a restart announced the commit again: %+v", again.all())
	}
}

// TestAnUncommittedEditIsCaughtUpWithoutASha pins the working tree's side: an
// edit not committed yet is announced by an outside actor with no sha, its
// cause the working tree.
func TestAnUncommittedEditIsCaughtUpWithoutASha(t *testing.T) {
	r := newCatchUpRepo(t)
	paths := r.seeded("cwm.a", "cwm.b")
	head := strings.TrimSpace(r.git("rev-parse", "HEAD"))
	r.handEdit(paths["cwm.b"], "name: cwm.b", "name: not committed")

	_, c := r.openPublishing()
	p := caughtUp(t, c)
	if !slices.Equal(p.Modified, []string{"cwm.b"}) {
		t.Fatalf("modified %v, want [cwm.b]", p.Modified)
	}
	if p.Sha != "" || p.Cause.Kind != api.CaughtUpWorkingTree || p.Cause.To != head || p.By.ID == "" {
		t.Fatalf("an uncommitted edit: actor %+v sha %q cause %+v; want outside with no sha, the working tree at %s", p.By, p.Sha, p.Cause, head)
	}
}

// bulkWrite writes n objects cwm.o0000… named name with plain writes, as a
// bulk copy or a pull makes them.
func (r *catchUpRepo) bulkWrite(n int, name string) {
	r.t.Helper()
	dir := filepath.Join(r.root, api.DefaultModulesDir, "cwm", "objects")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		r.t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("cwm.o%04d", i)
		o := obj(id, "feature", "")
		o.Name = name
		path := filepath.Join(dir, objectFileName("cwm", id))
		b, err := encodeObjectFile(path, o)
		if err == nil {
			err = os.WriteFile(path, b, 0o644)
		}
		if err != nil {
			r.t.Fatal(err)
		}
	}
}

// openAll restarts the service on the project with an in-process bus and
// captures EVERY event the open published, grouped and per object.
func (r *catchUpRepo) openAll() (*Service, *captured) {
	r.t.Helper()
	svc, err := New(stdctx.Background(), &CWMServiceConfig{WorkspaceRoot: r.ws, ProjectRoot: r.root})
	if err != nil {
		r.t.Fatalf("New: %v", err)
	}
	r.t.Cleanup(func() { _ = svc.Close() })
	bus := eventbus.NewMemoryEventBus()
	svc.Events = events.New("t", "u", "w", bus, nil)
	var c captured
	bus.SubscribeAny(c.on)
	if err := svc.index().ensureLoaded(); err != nil {
		r.t.Fatal(err)
	}
	return svc, &c
}

// split sorts what was captured: the grouped events and the object events.
func split(t *testing.T, c *captured) (grouped []api.ObjectsCaughtUpEventPayload, objects []api.ObjectEventPayload) {
	t.Helper()
	for _, p := range c.all() {
		switch v := p.(type) {
		case api.ObjectsCaughtUpEventPayload:
			grouped = append(grouped, v)
		case api.ObjectEventPayload:
			objects = append(objects, v)
		default:
			t.Fatalf("an unexpected event payload %T", p)
		}
	}
	return grouped, objects
}

// TestACatchUpAtTheThresholdPublishesEachObjectsEvent pins
// threshold from below: 50 objects moved outside the verbs get, beside the
// one grouped event, one object event each - updated, operation caught-up,
// the revision their file has now - so reactions run as for a verb write;
// and the catch-up writes no file of the project.
func TestACatchUpAtTheThresholdPublishesEachObjectsEvent(t *testing.T) {
	r := newCatchUpRepo(t)
	r.bulkWrite(api.CatchUpPerObjectMax, "before")
	r.open() // the baseline
	r.bulkWrite(api.CatchUpPerObjectMax, "after")
	before := treeDigest(t, r.root)

	svc, c := r.openAll()
	grouped, objects := split(t, c)
	if len(grouped) != 1 || grouped[0].Held || grouped[0].Count != api.CatchUpPerObjectMax {
		t.Fatalf("want one grouped event, not held, count %d: %+v", api.CatchUpPerObjectMax, grouped)
	}
	if len(objects) != api.CatchUpPerObjectMax {
		t.Fatalf("%d objects caught up published %d object events, want one each", api.CatchUpPerObjectMax, len(objects))
	}
	for _, p := range objects {
		if p.Action != "updated" || p.Operation != api.OperationCaughtUp || p.Module != "cwm" || p.Type != "feature" || p.EventID == "" {
			t.Fatalf("an object event is not a caught-up update of a cwm feature: %+v", p)
		}
		if p.Revision != svc.index().byID[p.ObjectID].Hash {
			t.Fatalf("%s's event carries revision %q, its file has %q", p.ObjectID, p.Revision, svc.index().byID[p.ObjectID].Hash)
		}
	}
	if after := treeDigest(t, r.root); after != before {
		t.Fatal("a catch-up changed the project's files")
	}
	if _, err := svc.CatchUpRelease(stdctx.Background(), api.CatchUpReleaseInput{EventID: grouped[0].EventID}); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("a catch-up under the threshold held nothing to release, got %v", err)
	}
}

// TestACatchUpAboveTheThresholdHoldsItsObjectEvents pins the threshold from
// above: 51 objects moved get ONE grouped event, held, with the count, and no
// object event until CatchUpRelease is called with its id - then 51, once,
// even after a restart in between; the project's files stay byte-identical.
func TestACatchUpAboveTheThresholdHoldsItsObjectEvents(t *testing.T) {
	n := api.CatchUpPerObjectMax + 1
	r := newCatchUpRepo(t)
	r.bulkWrite(n, "before")
	r.open()
	r.bulkWrite(n, "after")
	before := treeDigest(t, r.root)

	_, c := r.openAll()
	grouped, objects := split(t, c)
	if len(grouped) != 1 || !grouped[0].Held || grouped[0].Count != n {
		t.Fatalf("want one grouped event, held, count %d: %+v", n, grouped)
	}
	if len(objects) != 0 {
		t.Fatalf("a catch-up above the threshold published %d object events before its release", len(objects))
	}
	if after := treeDigest(t, r.root); after != before {
		t.Fatal("a catch-up changed the project's files")
	}

	// The card is answered after a restart: the held events survive it.
	svc, c2 := r.openAll()
	if g, o := split(t, c2); len(g)+len(o) != 0 {
		t.Fatalf("a quiet restart published %d grouped and %d object events", len(g), len(o))
	}
	out, err := svc.CatchUpRelease(stdctx.Background(), api.CatchUpReleaseInput{EventID: grouped[0].EventID})
	if err != nil || out.Published != n {
		t.Fatalf("release: %+v, %v; want %d published", out, err, n)
	}
	_, objects = split(t, c2)
	if len(objects) != n {
		t.Fatalf("the release published %d object events, want %d", len(objects), n)
	}
	for _, p := range objects {
		if p.Action != "updated" || p.Operation != api.OperationCaughtUp {
			t.Fatalf("a released event is not a caught-up update: %+v", p)
		}
	}
	if _, err := svc.CatchUpRelease(stdctx.Background(), api.CatchUpReleaseInput{EventID: grouped[0].EventID}); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("a second release of the same catch-up: %v, want not found", err)
	}
	if after := treeDigest(t, r.root); after != before {
		t.Fatal("a release changed the project's files")
	}
}

// TestAVerbsOwnWritesAreNotCaughtUpAtTheNextOpen pins that the cursor follows
// the verbs: objects created, updated and deleted through CWM after the open,
// with no scan since, are no outside change - the next open publishes no
// grouped event and no object event for them.
func TestAVerbsOwnWritesAreNotCaughtUpAtTheNextOpen(t *testing.T) {
	r := newCatchUpRepo(t)
	r.seeded("cwm.a", "cwm.b")
	svc, _ := r.open()
	ctx := stdctx.Background()
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "cwm", Object: obj("cwm.c", "feature", "")}); err != nil {
		t.Fatal(err)
	}
	a := obj("cwm.a", "feature", "")
	a.Name = "written by a verb"
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "cwm", Object: a}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ObjectDelete(ctx, api.ObjectDeleteInput{Module: "cwm", ID: "cwm.b"}); err != nil {
		t.Fatal(err)
	}
	_ = svc.Close()

	if _, c := r.openAll(); len(c.all()) != 0 {
		grouped, objects := split(t, c)
		t.Fatalf("the next open caught up the verbs' own writes: %+v, %d object events", grouped, len(objects))
	}
}
