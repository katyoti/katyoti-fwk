// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	crudapi "github.com/katyoti/katyoti-fwk/internal/extensions/crud/api"
)

// fixedClock returns a clock that always returns the same instant —
// makes Save-then-Load assertions deterministic.
func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// newStore is the shared MemoryStore constructor for the tests below.
// Uses NopEventBus by default — tests that exercise Watch swap in a
// MemoryEventBus to capture published events.
func newStore(t *testing.T) *MemoryStore {
	t.Helper()
	s := NewMemoryStore("test-conn", eventbus.NopEventBus{})
	s.nowFn = fixedClock(time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC))
	return s
}

func mustRaw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestMemoryStore_SaveAndLoad_RoundTrips(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	body := mustRaw(t, map[string]any{"title": "Dune", "author": "Herbert"})
	out, err := s.Save(ctx, crudapi.SaveInput{URI: "Book/b1", Body: body})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if out.Record.ID != "b1" || out.Record.Type != "Book" {
		t.Fatalf("Save returned (%q,%q), want (Book,b1)", out.Record.Type, out.Record.ID)
	}
	if out.Record.Version != 1 {
		t.Fatalf("Save version = %d, want 1", out.Record.Version)
	}

	loaded, err := s.Load(ctx, crudapi.LoadInput{URI: "Book/b1"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if string(loaded.Record.Body) != string(body) {
		t.Fatalf("Load body = %s, want %s", string(loaded.Record.Body), string(body))
	}
}

func TestMemoryStore_Save_GeneratesIDWhenURIHasNoIDSegment(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	// URI without trailing /id segment → server-assigns ID per
	// CapabilitiesOutput.IDAssignment="hybrid".
	out, err := s.Save(ctx, crudapi.SaveInput{URI: "Note", Body: mustRaw(t, map[string]any{"text": "hi"})})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if out.Record.ID == "" {
		t.Fatal("Save with no ID segment should generate one")
	}
}

func TestMemoryStore_SaveSameID_IncrementsVersion(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	_, _ = s.Save(ctx, crudapi.SaveInput{URI: "T/x", Body: mustRaw(t, map[string]any{"n": 1})})
	out, err := s.Save(ctx, crudapi.SaveInput{URI: "T/x", Body: mustRaw(t, map[string]any{"n": 2})})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if out.Record.Version != 2 {
		t.Fatalf("Version = %d, want 2", out.Record.Version)
	}
}

func TestMemoryStore_Load_MissingReturnsErrNotFound(t *testing.T) {
	_, err := newStore(t).Load(context.Background(), crudapi.LoadInput{URI: "Book/ghost"})
	if !errors.Is(err, crudapi.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound chain", err)
	}
}

func TestMemoryStore_Delete_RemovesAndReportsNotFoundAfter(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	_, _ = s.Save(ctx, crudapi.SaveInput{URI: "T/x", Body: mustRaw(t, map[string]any{"v": 1})})
	out, err := s.Delete(ctx, crudapi.DeleteInput{URI: "T/x"})
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !out.OK {
		t.Fatal("Delete.OK = false")
	}
	_, err = s.Load(ctx, crudapi.LoadInput{URI: "T/x"})
	if !errors.Is(err, crudapi.ErrNotFound) {
		t.Fatalf("post-delete Load err = %v, want ErrNotFound chain", err)
	}
}

func TestMemoryStore_List_FiltersAndPaginates(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	for i, title := range []string{"Dune", "Foundation", "Hyperion", "Neuromancer", "Snow Crash"} {
		_, err := s.Save(ctx, crudapi.SaveInput{
			URI:  crudapi.URI("Book/" + string(rune('a'+i))),
			Body: mustRaw(t, map[string]any{"title": title, "year": 1965 + i*5}),
		})
		if err != nil {
			t.Fatalf("seed Save: %v", err)
		}
	}

	// Equality filter — collection URI is "Book" (no /id).
	out, err := s.List(ctx, crudapi.ListInput{
		URI: "Book",
		Filters: map[string]crudapi.FilterValue{
			"title": {Op: "eq", Value: mustRaw(t, "Hyperion")},
		},
	})
	if err != nil {
		t.Fatalf("List eq: %v", err)
	}
	if len(out.Records) != 1 || string(out.Records[0].ID) != "c" {
		t.Fatalf("List eq Hyperion: got %d records (ids: %v)", len(out.Records), recordIDs(out.Records))
	}

	// gte filter + order_by
	out, err = s.List(ctx, crudapi.ListInput{
		URI: "Book",
		Filters: map[string]crudapi.FilterValue{
			"year": {Op: "gte", Value: mustRaw(t, 1980)},
		},
		OrderBy: []crudapi.OrderBy{{Field: "year", Desc: false}},
	})
	if err != nil {
		t.Fatalf("List gte: %v", err)
	}
	// Seeded years: 1965, 1970, 1975, 1980, 1985 — gte 1980 = 1980, 1985.
	if out.Pagination.Total != 2 {
		t.Fatalf("Total = %d, want 2", out.Pagination.Total)
	}

	// Pagination: limit 2, offset 1
	out, err = s.List(ctx, crudapi.ListInput{URI: "Book", Limit: 2, Offset: 1})
	if err != nil {
		t.Fatalf("List paged: %v", err)
	}
	if len(out.Records) != 2 {
		t.Fatalf("Paged len = %d, want 2 (ids: %v)", len(out.Records), recordIDs(out.Records))
	}
	if !out.Pagination.HasMore {
		t.Fatal("HasMore = false, want true (2 records seen, 5 total)")
	}
}

func TestMemoryStore_List_InAndContains(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	for _, name := range []string{"alpha", "beta", "gamma"} {
		_, _ = s.Save(ctx, crudapi.SaveInput{URI: crudapi.URI("T/" + name), Body: mustRaw(t, map[string]any{"name": name})})
	}

	out, err := s.List(ctx, crudapi.ListInput{
		URI: "T",
		Filters: map[string]crudapi.FilterValue{
			"name": {Op: "in", Value: mustRaw(t, []string{"alpha", "gamma"})},
		},
	})
	if err != nil {
		t.Fatalf("List in: %v", err)
	}
	if len(out.Records) != 2 {
		t.Fatalf("in: got %d, want 2 (%v)", len(out.Records), recordIDs(out.Records))
	}

	out, err = s.List(ctx, crudapi.ListInput{
		URI: "T",
		Filters: map[string]crudapi.FilterValue{
			"name": {Op: "contains", Value: mustRaw(t, "et")},
		},
	})
	if err != nil {
		t.Fatalf("List contains: %v", err)
	}
	if len(out.Records) != 1 || out.Records[0].ID != "beta" {
		t.Fatalf("contains: got %v, want [beta]", recordIDs(out.Records))
	}
}

func TestMemoryStore_List_RejectsUnsupportedOp(t *testing.T) {
	_, err := newStore(t).List(context.Background(), crudapi.ListInput{
		URI: "T",
		Filters: map[string]crudapi.FilterValue{
			"f": {Op: "regex", Value: mustRaw(t, "x")},
		},
	})
	if !errors.Is(err, crudapi.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput chain", err)
	}
}

// TestMemoryStore_List_RejectsRecordURI exercises the method-based
// disambiguation contract: List takes a collection URI; passing a
// record URI (`<type>/<id>`) is an error per cwm-v3 §2.4.
func TestMemoryStore_List_RejectsRecordURI(t *testing.T) {
	_, err := newStore(t).List(context.Background(), crudapi.ListInput{URI: "T/x"})
	if !errors.Is(err, crudapi.ErrInvalidInput) {
		t.Fatalf("List(record-URI): err = %v, want ErrInvalidInput chain", err)
	}
}

// TestMemoryStore_Load_RejectsCollectionURI exercises the inverse:
// Load takes a record URI; passing a collection URI (no /id) errors.
func TestMemoryStore_Load_RejectsCollectionURI(t *testing.T) {
	_, err := newStore(t).Load(context.Background(), crudapi.LoadInput{URI: "T"})
	if !errors.Is(err, crudapi.ErrInvalidInput) {
		t.Fatalf("Load(collection-URI): err = %v, want ErrInvalidInput chain", err)
	}
}

func TestMemoryStore_Watch_PublishesOnSave(t *testing.T) {
	bus := eventbus.NewMemoryEventBus()
	s := NewMemoryStore("test-conn", bus)
	s.nowFn = fixedClock(time.Now().UTC())

	wo, err := s.Watch(context.Background(), crudapi.WatchInput{URI: "Book"})
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if wo.Subject == "" {
		t.Fatal("Watch returned empty subject")
	}

	var mu sync.Mutex
	var received []crudapi.Event
	unsub := bus.Subscribe(wo.Subject, func(_ string, payload any) {
		evt, ok := payload.(crudapi.Event)
		if !ok {
			t.Errorf("payload is %T, want crudapi.Event", payload)
			return
		}
		mu.Lock()
		received = append(received, evt)
		mu.Unlock()
	})
	defer unsub()

	_, _ = s.Save(context.Background(), crudapi.SaveInput{
		URI:  "Book/b1",
		Body: mustRaw(t, map[string]any{"title": "Dune"}),
	})
	_, _ = s.Delete(context.Background(), crudapi.DeleteInput{URI: "Book/b1"})

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Fatalf("got %d events, want 2", len(received))
	}
	if received[0].Kind != crudapi.WatchKindCreated {
		t.Fatalf("ev0.Kind = %q, want created", received[0].Kind)
	}
	if received[1].Kind != crudapi.WatchKindDeleted {
		t.Fatalf("ev1.Kind = %q, want deleted", received[1].Kind)
	}
}

func TestMemoryStore_Capabilities_Sane(t *testing.T) {
	caps, err := newStore(t).Capabilities(context.Background())
	if err != nil {
		t.Fatalf("Capabilities: %v", err)
	}
	if !caps.Writable {
		t.Error("Writable = false, want true")
	}
	if !caps.Watch.Supported {
		t.Error("Watch.Supported = false, want true")
	}
	if caps.Pagination.DefaultLimit == 0 {
		t.Error("Pagination.DefaultLimit = 0 — caller couldn't size a default page")
	}
	if len(caps.Filters.Ops) == 0 {
		t.Error("Filters.Ops empty — no filter ops advertised")
	}
	// cwm-v3 §6: connection must self-describe its URI shapes.
	if len(caps.URIPatterns) == 0 {
		t.Error("URIPatterns empty — memory store should declare {type} + {type}/{id}")
	}
}

// recordIDs is a debugging helper for table-style assertions.
func recordIDs(recs []crudapi.Record) []string {
	ids := make([]string, len(recs))
	for i, r := range recs {
		ids[i] = r.ID
	}
	return ids
}
