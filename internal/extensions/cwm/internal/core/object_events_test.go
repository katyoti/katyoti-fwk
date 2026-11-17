// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/events"
)

// OBJECT EVENTS say WHICH object: the subject carries a token (a hash), so the
// payload carries the id and the module — what the journal records.

type captured struct {
	mu   sync.Mutex
	seen []any
}

func (c *captured) on(_ string, payload any) {
	c.mu.Lock()
	c.seen = append(c.seen, payload)
	c.mu.Unlock()
}

func (c *captured) all() []any {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]any(nil), c.seen...)
}

func eventsFixture(t *testing.T) (*Service, *eventbus.MemoryEventBus) {
	t.Helper()
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir()}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	bus := eventbus.NewMemoryEventBus()
	svc.Events = events.New("t", "u", "w", bus, nil)
	return svc, bus
}

func treeObject(id, parent string) api.Object {
	o := api.Object{ID: id, Type: "note"}
	o.Name = id
	o.Parent = parent
	return o
}

// TestObjectEventsCarryTheObjectIDAndModule: create, update and delete publish
// ONE object event each, naming the object, its module and its type.
func TestObjectEventsCarryTheObjectIDAndModule(t *testing.T) {
	svc, bus := eventsFixture(t)
	ctx := stdctx.Background()
	o := treeObject("billing.spec", "")
	tok := api.SubjectToken(o.ID)
	var c captured
	for _, sub := range []string{
		api.ObjectCreatedSubject("t", "u", "w", tok),
		api.ObjectUpdatedSubject("t", "u", "w", tok),
		api.ObjectDeletedSubject("t", "u", "w", tok),
	} {
		_ = bus.Subscribe(sub, c.on)
	}
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: o}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "billing", Object: o}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, err := svc.ObjectDelete(ctx, api.ObjectDeleteInput{Module: "billing", ID: o.ID}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	got := c.all()
	if len(got) != 3 {
		t.Fatalf("events = %d, want one for each of create, update, delete", len(got))
	}
	for i, p := range got {
		v, ok := p.(events.ObjectPayload)
		if !ok {
			t.Fatalf("unexpected payload %T", p)
		}
		if want := []string{"created", "updated", "deleted"}[i]; v.ObjectID != o.ID || v.Module != "billing" || v.Type != "note" || v.Action != want {
			t.Errorf("event %d = %+v, want %s of billing.spec (note) in billing", i, v, want)
		}
	}
}

// TestAMoveIsOneMovedEvent: whatever a move takes underneath, subscribers get
// ONE moved event naming both ids, both parents and the reason.
func TestAMoveIsOneMovedEvent(t *testing.T) {
	svc, bus := eventsFixture(t)
	ctx := stdctx.Background()
	for _, o := range []api.Object{treeObject("billing.a", ""), treeObject("billing.b", ""), treeObject("billing.x", "billing.a")} {
		if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}

	// Within a module: re-parent.
	var within captured
	_ = bus.Subscribe(api.ObjectMovedSubject("t", "u", "w", api.SubjectToken("billing.x")), within.on)
	if _, err := svc.ObjectMove(ctx, api.ObjectMoveInput{ID: "billing.x", NewParent: "billing.b", Reason: "belongs under b"}); err != nil {
		t.Fatalf("move: %v", err)
	}
	got := within.all()
	if len(got) != 1 {
		t.Fatalf("moved events = %d, want exactly one", len(got))
	}
	m := got[0].(events.ObjectMovedPayload)
	if m.ObjectID != "billing.x" || m.FromID != "billing.x" || m.FromParent != "billing.a" || m.Parent != "billing.b" ||
		m.Reason != "belongs under b" || m.Module != "billing" || m.FromModule != "billing" {
		t.Fatalf("moved = %+v", m)
	}

	// Across modules: a new id — the event is on the NEW id's token.
	var across captured
	_ = bus.Subscribe(api.ObjectMovedSubject("t", "u", "w", api.SubjectToken("ledger.x")), across.on)
	if _, err := svc.ObjectMove(ctx, api.ObjectMoveInput{ID: "billing.x", NewModule: "ledger", Reason: "ledger owns it"}); err != nil {
		t.Fatalf("move across: %v", err)
	}
	got = across.all()
	if len(got) != 1 {
		t.Fatalf("moved events across modules = %d, want exactly one", len(got))
	}
	m = got[0].(events.ObjectMovedPayload)
	if m.ObjectID != "ledger.x" || m.FromID != "billing.x" || m.Module != "ledger" || m.FromModule != "billing" {
		t.Fatalf("moved across = %+v", m)
	}
}

// TestASplitIsOneSplitEventAndItsDeleteIsMarked: a split publishes ONE split
// event on the source's token naming the successors and the reason; when the
// source goes, its delete is marked operation split (the split event reports it).
func TestASplitIsOneSplitEventAndItsDeleteIsMarked(t *testing.T) {
	svc, bus := eventsFixture(t)
	ctx := stdctx.Background()
	for _, o := range []api.Object{treeObject("billing.a", ""), treeObject("billing.c1", "billing.a"), treeObject("billing.c2", "billing.a")} {
		if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}
	var split, deleted captured
	_ = bus.Subscribe(api.ObjectSplitSubject("t", "u", "w", api.SubjectToken("billing.a")), split.on)
	_ = bus.Subscribe(api.ObjectDeletedSubject("t", "u", "w", api.SubjectToken("billing.a")), deleted.on)
	out, err := svc.ObjectSplit(ctx, api.ObjectSplitInput{ID: "billing.a", Reason: "two concerns", DeleteSource: true,
		Successors: []api.ObjectSplitTarget{
			{Type: "note", Title: "Left", Children: []string{"billing.c1"}},
			{Type: "note", Title: "Right", Children: []string{"billing.c2"}},
		}})
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	got := split.all()
	if len(got) != 1 {
		t.Fatalf("split events = %d, want exactly one", len(got))
	}
	p := got[0].(events.ObjectSplitPayload)
	if p.ObjectID != "billing.a" || p.Module != "billing" || p.Reason != "two concerns" || p.SourceKept ||
		len(p.Successors) != 2 || p.Successors[0] != out.Successors[0] {
		t.Fatalf("split = %+v (out %+v)", p, out)
	}
	dels := deleted.all()
	if len(dels) != 1 || dels[0].(events.ObjectPayload).Operation != api.OperationSplit {
		t.Fatalf("the source's delete = %+v, want one marked operation split", dels)
	}
}

// TestAWriteStampsItsEventMeta: every write's event says which event it is,
// when, the revision it left and — when the write's context carries one — its
// cause; a write with no cause carries none; a delete carries the revision it
// removed.
func TestAWriteStampsItsEventMeta(t *testing.T) {
	svc, bus := eventsFixture(t)
	who := cause.Cause{Kind: cause.KindPerson, ID: "ada", Depth: 0}
	caused := cause.WithCause(stdctx.Background(), who)
	o := treeObject("billing.meta", "")
	tok := api.SubjectToken(o.ID)
	var c captured
	for _, sub := range []string{
		api.ObjectCreatedSubject("t", "u", "w", tok),
		api.ObjectUpdatedSubject("t", "u", "w", tok),
		api.ObjectDeletedSubject("t", "u", "w", tok),
	} {
		_ = bus.Subscribe(sub, c.on)
	}
	before := time.Now().UTC().Add(-time.Second)
	created, err := svc.ObjectCreate(caused, api.ObjectCreateInput{Module: "billing", Object: o})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	upd, err := svc.ObjectUpdate(stdctx.Background(), api.ObjectUpdateInput{Module: "billing", Object: o})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, err := svc.ObjectDelete(caused, api.ObjectDeleteInput{Module: "billing", ID: o.ID}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	got := c.all()
	if len(got) != 3 {
		t.Fatalf("events = %d, want 3", len(got))
	}
	cr, up, del := got[0].(events.ObjectPayload), got[1].(events.ObjectPayload), got[2].(events.ObjectPayload)
	for i, p := range []events.ObjectPayload{cr, up, del} {
		if _, err := uuid.Parse(p.EventID); err != nil || p.At.Before(before) || p.Revision == "" {
			t.Errorf("event %d meta = %+v: want a uuid event id, a time at the write and a revision", i, p.EventMeta)
		}
	}
	if cr.EventID == up.EventID || up.EventID == del.EventID {
		t.Error("two events share an id")
	}
	if cr.Cause == nil || *cr.Cause != who || cr.Revision != created.Revision {
		t.Errorf("created = %+v, want cause %+v and revision %s", cr.EventMeta, who, created.Revision)
	}
	if up.Cause != nil || up.Revision != upd.Revision {
		t.Errorf("the uncaused update = %+v, want no cause and revision %s", up.EventMeta, upd.Revision)
	}
	if del.Cause == nil || *del.Cause != who || del.Revision != upd.Revision {
		t.Errorf("deleted = %+v, want cause %+v and the revision removed %s", del.EventMeta, who, upd.Revision)
	}
}
