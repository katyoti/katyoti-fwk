// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
)

// THE COST OF A LISTING, counted.
//
// These tests count ROUND TRIPS, because that is what the four seconds were.
// Reasoning about whether a path is "probably fine" is what let a listing pay
// one full index fetch per entry for as long as it did.

// countingSender records every call by method.
type countingSender struct {
	mu      sync.Mutex
	calls   map[string]int
	index   []contribapi.Contribution
	bodies  map[string][]byte
	perBody func(uri string)
}

func (s *countingSender) Call(_ context.Context, path string, params any, _ ...ktsapi.CallOption) (json.RawMessage, error) {
	s.mu.Lock()
	s.calls[path]++
	s.mu.Unlock()
	switch path {
	case aggregator + "ListContributions":
		return json.Marshal(mbapi.CatalogListContributionsOutput{Contributions: s.index})
	case aggregator + "GetBody":
		in, ok := params.(mbapi.GetBodyInput)
		if !ok {
			return nil, fmt.Errorf("unexpected params %T", params)
		}
		if s.perBody != nil {
			s.perBody(in.URI)
		}
		return json.Marshal(mbapi.GetBodyOutput{Content: s.bodies[in.URI]})
	}
	return nil, fmt.Errorf("unexpected call %q", path)
}

func (s *countingSender) Notify(context.Context, string, any, ...ktsapi.CallOption) error { return nil }

func (s *countingSender) count(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[aggregator+path]
}

// fixture builds n body-free task entries, the shape the aggregate carries.
func fixture(t *testing.T, n int) *countingSender {
	t.Helper()
	s := &countingSender{calls: map[string]int{}, bodies: map[string][]byte{}}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("task-%02d", i)
		body, _ := json.Marshal(map[string]any{"name": id, "type": "agent"})
		c := contribapi.Contribution{
			Provider: "p", Type: "task", ID: id,
			Layers: contribapi.DataDescriptor(contribapi.HashContent(body), int64(len(body))),
		}
		s.index = append(s.index, c)
		s.bodies[c.ResolvedURI()] = body
	}
	return s
}

// TestSnapshotReadsTheIndexOnceHoweverManyBodies — the defect, measured. A
// bare Catalog locates each entry by re-reading the whole index, so N bodies
// cost N index fetches.
func TestSnapshotReadsTheIndexOnceHoweverManyBodies(t *testing.T) {
	ctx := context.Background()
	const n = 63 // the task gallery's real size

	bare := fixture(t, n)
	cat := New(bare)
	idx, err := cat.Index(ctx)
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	for _, e := range idx {
		if _, err := cat.GetBody(ctx, e.ResolvedURI()); err != nil {
			t.Fatalf("GetBody: %v", err)
		}
	}
	bareIndex := bare.count("ListContributions")
	if bareIndex < n {
		t.Fatalf("the bare reader made %d index fetches for %d bodies — the defect this measures is gone, update the test", bareIndex, n)
	}

	snap := fixture(t, n)
	s, err := TakeSnapshot(ctx, snap)
	if err != nil {
		t.Fatalf("TakeSnapshot: %v", err)
	}
	for _, e := range s.index {
		if _, err := s.GetBody(ctx, e.ResolvedURI()); err != nil {
			t.Fatalf("snapshot GetBody: %v", err)
		}
	}
	if got := snap.count("ListContributions"); got != 1 {
		t.Errorf("the snapshot made %d index fetches, want exactly 1", got)
	}
	if got := snap.count("GetBody"); got != n {
		t.Errorf("bodies fetched %d times, want %d — one each", got, n)
	}
	t.Logf("index fetches: bare %d, snapshot %d (for %d bodies)", bareIndex, snap.count("ListContributions"), n)
}

// TestSnapshotAsksForOneBodyOnce — within one piece of work the same question
// is not asked twice.
func TestSnapshotAsksForOneBodyOnce(t *testing.T) {
	ctx := context.Background()
	s := fixture(t, 3)
	snap, err := TakeSnapshot(ctx, s)
	if err != nil {
		t.Fatalf("TakeSnapshot: %v", err)
	}
	uri := snap.index[0].ResolvedURI()
	for i := 0; i < 5; i++ {
		if _, err := snap.GetBody(ctx, uri); err != nil {
			t.Fatalf("GetBody: %v", err)
		}
	}
	if got := s.count("GetBody"); got != 1 {
		t.Errorf("one body asked for five times cost %d calls, want 1", got)
	}
}

// TestSnapshotServesTheIndexWithoutCalling — Index and the by-type filter are
// what the snapshot IS; asking must not reach the bus.
func TestSnapshotServesTheIndexWithoutCalling(t *testing.T) {
	ctx := context.Background()
	s := fixture(t, 5)
	snap, err := TakeSnapshot(ctx, s)
	if err != nil {
		t.Fatalf("TakeSnapshot: %v", err)
	}
	before := s.count("ListContributions")
	for i := 0; i < 4; i++ {
		if _, err := snap.Index(ctx); err != nil {
			t.Fatalf("Index: %v", err)
		}
		if _, err := snap.GetContributionsByType(ctx, "task"); err != nil {
			t.Fatalf("GetContributionsByType: %v", err)
		}
		if _, err := snap.List(ctx); err != nil {
			t.Fatalf("List: %v", err)
		}
	}
	if got := s.count("ListContributions"); got != before {
		t.Errorf("reading the held index cost %d more calls", got-before)
	}
}

// TestSnapshotRefusesAnEntryItDoesNotHold — a snapshot is a moment, and an
// entry outside it is absent rather than silently fetched, which would make
// the moment a lie.
func TestSnapshotRefusesAnEntryItDoesNotHold(t *testing.T) {
	ctx := context.Background()
	s := fixture(t, 2)
	snap, err := TakeSnapshot(ctx, s)
	if err != nil {
		t.Fatalf("TakeSnapshot: %v", err)
	}
	if _, err := snap.GetBody(ctx, "p:/task/not-in-the-snapshot"); err == nil {
		t.Error("an entry the snapshot does not hold must not resolve")
	}
}
