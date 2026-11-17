// SPDX-License-Identifier: Apache-2.0

package filestate

import (
	"strconv"
	"sync"
	"testing"
	"time"
)

// intPtr is a small helper because writing &n for a literal is annoying.
func intPtr(n int) *int { return &n }

func TestEntryIsReadEntry(t *testing.T) {
	cases := []struct {
		name string
		e    Entry
		want bool
	}{
		{"both nil (write/edit)", Entry{}, false},
		{"offset only", Entry{Offset: intPtr(0)}, false},
		{"limit only", Entry{Limit: intPtr(10)}, false},
		{"both set (read)", Entry{Offset: intPtr(0), Limit: intPtr(10)}, true},
		{"both zero (read of empty file)", Entry{Offset: intPtr(0), Limit: intPtr(0)}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.e.IsReadEntry(); got != c.want {
				t.Errorf("IsReadEntry() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestMemoryStore_RecordAndLookup(t *testing.T) {
	s := NewMemoryStore(0) // 0 → DefaultCapacity

	now := time.Now()
	s.Record("/a/b.txt", Entry{
		Content:   "hello",
		Timestamp: now,
		Offset:    intPtr(0),
		Limit:     intPtr(2000),
	})

	got, ok := s.Lookup("/a/b.txt")
	if !ok {
		t.Fatal("Lookup miss after Record")
	}
	if got.Content != "hello" {
		t.Errorf("Content = %q, want %q", got.Content, "hello")
	}
	if !got.Timestamp.Equal(now) {
		t.Errorf("Timestamp = %v, want %v", got.Timestamp, now)
	}
	if got.Offset == nil || *got.Offset != 0 {
		t.Errorf("Offset = %v, want *0", got.Offset)
	}
	if got.Limit == nil || *got.Limit != 2000 {
		t.Errorf("Limit = %v, want *2000", got.Limit)
	}
	if !got.IsReadEntry() {
		t.Error("expected IsReadEntry true")
	}
}

func TestMemoryStore_LookupMiss(t *testing.T) {
	s := NewMemoryStore(0)
	_, ok := s.Lookup("/nonexistent")
	if ok {
		t.Error("Lookup of unknown path returned ok")
	}
}

func TestMemoryStore_RecordOverwrite(t *testing.T) {
	s := NewMemoryStore(0)
	t1 := time.Now()
	t2 := t1.Add(time.Second)

	s.Record("/a", Entry{Content: "first", Timestamp: t1, Offset: intPtr(0), Limit: intPtr(100)})
	s.Record("/a", Entry{Content: "second", Timestamp: t2, Offset: intPtr(10), Limit: intPtr(50)})

	got, ok := s.Lookup("/a")
	if !ok {
		t.Fatal("Lookup miss")
	}
	if got.Content != "second" {
		t.Errorf("Content = %q, want %q (overwrite failed)", got.Content, "second")
	}
	if !got.Timestamp.Equal(t2) {
		t.Errorf("Timestamp = %v, want %v", got.Timestamp, t2)
	}
	if *got.Offset != 10 || *got.Limit != 50 {
		t.Errorf("Offset/Limit = %d/%d, want 10/50", *got.Offset, *got.Limit)
	}
}

func TestMemoryStore_OverwriteDoesNotEvict(t *testing.T) {
	// Capacity 2: insert A, B, then overwrite A. The overwrite must NOT
	// evict B — overwrite is not a new insertion. Then insert C: this is
	// the new insertion that evicts the oldest. By insertion order, A was
	// inserted first; B second; A is still "first" because overwrite
	// preserves position. So C should evict A.
	s := NewMemoryStore(2)
	s.Record("/a", Entry{Content: "a1", Offset: intPtr(0), Limit: intPtr(1)})
	s.Record("/b", Entry{Content: "b1", Offset: intPtr(0), Limit: intPtr(1)})
	s.Record("/a", Entry{Content: "a2", Offset: intPtr(0), Limit: intPtr(1)})

	// Both still present after overwrite.
	if _, ok := s.Lookup("/a"); !ok {
		t.Error("/a evicted by overwrite, should still be present")
	}
	if _, ok := s.Lookup("/b"); !ok {
		t.Error("/b evicted by overwrite of /a, should still be present")
	}

	// Now insert /c — this triggers FIFO eviction of the oldest insertion.
	s.Record("/c", Entry{Content: "c1", Offset: intPtr(0), Limit: intPtr(1)})

	if _, ok := s.Lookup("/a"); ok {
		t.Error("/a should have been evicted by /c (oldest insertion)")
	}
	if _, ok := s.Lookup("/b"); !ok {
		t.Error("/b should still be present")
	}
	if _, ok := s.Lookup("/c"); !ok {
		t.Error("/c should be present (just inserted)")
	}
}

func TestMemoryStore_Eviction(t *testing.T) {
	// Capacity 3, insert 5 entries → first two evicted in FIFO order.
	s := NewMemoryStore(3)
	for i := 0; i < 5; i++ {
		s.Record("/p"+strconv.Itoa(i), Entry{Content: strconv.Itoa(i), Offset: intPtr(0), Limit: intPtr(1)})
	}

	// p0 and p1 should be evicted; p2/p3/p4 retained.
	for _, evicted := range []string{"/p0", "/p1"} {
		if _, ok := s.Lookup(evicted); ok {
			t.Errorf("%s should have been evicted", evicted)
		}
	}
	for _, retained := range []string{"/p2", "/p3", "/p4"} {
		if _, ok := s.Lookup(retained); !ok {
			t.Errorf("%s should still be present", retained)
		}
	}
}

func TestMemoryStore_DefaultCapacity(t *testing.T) {
	s := NewMemoryStore(-1) // negative → DefaultCapacity
	ms := s.(*memoryStore)
	if ms.capacity != DefaultCapacity {
		t.Errorf("capacity = %d, want DefaultCapacity (%d)", ms.capacity, DefaultCapacity)
	}
}

func TestMemoryStore_Close(t *testing.T) {
	s := NewMemoryStore(10)
	s.Record("/a", Entry{Content: "a", Offset: intPtr(0), Limit: intPtr(1)})
	s.Record("/b", Entry{Content: "b", Offset: intPtr(0), Limit: intPtr(1)})

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// After Close, lookups miss.
	if _, ok := s.Lookup("/a"); ok {
		t.Error("Lookup hit after Close")
	}

	// Close is idempotent.
	if err := s.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}

	// Record still works after Close (the contract is "behave as if empty",
	// not "panic"). This makes the close-on-shutdown path race-tolerant.
	s.Record("/c", Entry{Content: "c", Offset: intPtr(0), Limit: intPtr(1)})
	if _, ok := s.Lookup("/c"); !ok {
		t.Error("Record after Close did not take effect")
	}
}

func TestMemoryStore_PartialViewFlag(t *testing.T) {
	s := NewMemoryStore(0)
	s.Record("/a", Entry{
		Content:       "lines 10-15",
		Offset:        intPtr(10),
		Limit:         intPtr(5),
		IsPartialView: true,
	})
	got, _ := s.Lookup("/a")
	if !got.IsPartialView {
		t.Error("IsPartialView lost")
	}
	if !got.IsReadEntry() {
		t.Error("partial view should still be a read entry")
	}
}

func TestMemoryStore_WriteEntryNotARead(t *testing.T) {
	// Simulate a file_write recording: Offset and Limit are nil.
	s := NewMemoryStore(0)
	s.Record("/a", Entry{Content: "post-write content"})
	got, _ := s.Lookup("/a")
	if got.IsReadEntry() {
		t.Error("entry with nil Offset/Limit should NOT be a read entry (FR-FR-10)")
	}
}

func TestMemoryStore_ConcurrentRecordAndLookup(t *testing.T) {
	// Smoke test for the mutex: many goroutines hammering the same store.
	// We just want it to finish without a -race trip and without panicking.
	s := NewMemoryStore(100)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			path := "/p" + strconv.Itoa(id%10)
			for j := 0; j < 100; j++ {
				s.Record(path, Entry{
					Content: strconv.Itoa(j),
					Offset:  intPtr(0),
					Limit:   intPtr(1),
				})
				_, _ = s.Lookup(path)
			}
		}(i)
	}
	wg.Wait()
}
