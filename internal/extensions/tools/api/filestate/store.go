// SPDX-License-Identifier: Apache-2.0

// Package filestate provides a session-scoped store of file read/write
// snapshots used by the file_read, file_write, and file_edit tools to
// implement deduplication and staleness checks.
//
// Concept: each successful read of a file records an Entry containing the
// content read, the file's modification time at read time, and the offset/
// limit window that was actually read. A subsequent read of the same path
// with the same window can short-circuit if the on-disk mtime has not
// advanced since the recorded snapshot — the agent already has that content
// in its turn history, so re-reading would just burn tokens.
//
// Entries originating from file_write and file_edit set Offset/Limit to nil
// (the "undefined" sentinel) to mark the entry as a write/edit snapshot
// rather than a read snapshot. file_read MUST NOT dedup against such
// entries — see spec FR-FR-10 in docs/agent_tools/tool_file_read_spec.md.
//
// The store is registered as a session-scoped service via
// application.DeclareScopedSimpleService(application.SessionScope, ...) so
// each interactive session gets its own store and the store is closed when
// the session ends.
package filestate

import (
	"sync"
	"time"
)

// Entry is a single recorded snapshot of a file at the moment a tool last
// touched it.
//
// Offset and Limit are *int (not int) so callers can distinguish "no offset
// recorded" (nil) from "offset zero" (pointer to 0). file_read writes a
// non-nil Offset/Limit; file_write and file_edit leave them nil.
type Entry struct {
	// Content is the file content as observed by the recording tool. For
	// read entries, it is the cat-n-formatted content returned to the LLM.
	// For write/edit entries, it is the post-mutation content.
	Content string

	// Timestamp is the file's modification time at the moment the tool
	// recorded the entry. Used by dedup to detect on-disk changes since
	// the recording.
	Timestamp time.Time

	// Offset is the start line of the read window (0-based). nil means the
	// entry was created by a write/edit operation, not a read.
	Offset *int

	// Limit is the maximum number of lines requested by the read. nil means
	// the entry was created by a write/edit operation, not a read.
	Limit *int

	// IsPartialView is true when the read covered a strict sub-range of the
	// file rather than the full content. Reserved for future dedup tuning;
	// MVP dedup matches strictly on (path, offset, limit, mtime) regardless
	// of this flag.
	IsPartialView bool
}

// IsReadEntry reports whether the entry was created by a read operation
// (Offset and Limit set) rather than a write/edit operation (both nil).
// Dedup gating uses this to satisfy spec FR-FR-10.
func (e Entry) IsReadEntry() bool {
	return e.Offset != nil && e.Limit != nil
}

// Store is the per-session file-state cache consumed by the file tools.
//
// Implementations MUST be safe for concurrent use: multiple agents within a
// single session may touch the store from different goroutines.
type Store interface {
	// Record stores (or overwrites) the entry for absPath. The store may
	// evict an older unrelated entry to enforce its capacity.
	Record(absPath string, entry Entry)

	// Lookup returns the recorded entry for absPath, or (zero, false) if
	// no entry exists.
	Lookup(absPath string) (Entry, bool)

	// Close releases any resources held by the store. After Close, calls to
	// Record and Lookup behave as if the store is empty. Close MUST be safe
	// to call multiple times.
	Close() error
}

// memoryStore is the default in-memory Store implementation.
//
// It uses a bounded map with FIFO eviction by insertion order — when the
// store reaches capacity, the oldest insertion is dropped to make room for
// the new one. This is intentionally simpler than LRU: under typical agent
// workloads (a session reads at most a few hundred files), the eviction
// policy rarely fires, and FIFO is half the bookkeeping of LRU.
type memoryStore struct {
	mu       sync.Mutex
	capacity int
	entries  map[string]Entry
	order    []string // insertion order, used for FIFO eviction
}

// DefaultCapacity is the maximum number of entries a memoryStore retains
// before evicting the oldest. Chosen to comfortably fit any realistic
// agent session without runaway memory growth.
const DefaultCapacity = 256

// NewMemoryStore returns a Store backed by an in-memory bounded map. A
// capacity <= 0 is replaced by DefaultCapacity.
func NewMemoryStore(capacity int) Store {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	return &memoryStore{
		capacity: capacity,
		entries:  make(map[string]Entry, capacity),
		order:    make([]string, 0, capacity),
	}
}

func (s *memoryStore) Record(absPath string, entry Entry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.entries[absPath]; exists {
		// Overwrite in place; preserve insertion order.
		s.entries[absPath] = entry
		return
	}

	if len(s.entries) >= s.capacity {
		// Evict the oldest insertion.
		oldest := s.order[0]
		s.order = s.order[1:]
		delete(s.entries, oldest)
	}

	s.entries[absPath] = entry
	s.order = append(s.order, absPath)
}

func (s *memoryStore) Lookup(absPath string) (Entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.entries[absPath]
	return entry, ok
}

func (s *memoryStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries = make(map[string]Entry)
	s.order = s.order[:0]
	return nil
}
