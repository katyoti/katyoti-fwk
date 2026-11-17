// SPDX-License-Identifier: Apache-2.0

package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// ContentStore is the content-addressed local cache for contribution
// bodies — the CONTENT plane of the two-plane cache. One file per
// sha256, named by the hash; identical bodies from different providers
// dedup to a single file.
//
// Layout under <Root>:
//
//	<Root>/<hex-sha256>
//
// where <Root> is typically `<CacheDir>/contributions/`. The store is
// safe to `rm -rf` — it is a pure cache; any miss is repaired by a
// per-hash bus fetch (see Catalog.GetBody). Writes are atomic
// (sibling temp + rename) so a crash mid-write never leaves a partial
// file at the canonical name.
//
// Concurrency: an in-process write lock serialises Put for the same
// process; cross-process writes are made safe by the temp+rename
// pattern (the rename is atomic for the destination name).
type ContentStore struct {
	root string

	mu sync.Mutex // serialises Put within this process

	// The resident layer. A content-addressed store cannot serve a wrong
	// entry from memory: the hash IS the content's identity, so a hit is
	// correct by construction and no invalidation is needed — an edit
	// changes the bytes, which changes the hash, which is a different key.
	//
	// Bounded by total bytes, FIFO. Eviction can only cost a disk read, so
	// the policy needs to be cheap, not clever.
	memMu    sync.RWMutex
	mem      map[string][]byte
	memOrder []string
	memBytes int

	hits, misses int64 // observability; read via Stats
}

const (
	// memCapBytes is how much decoded content the resident layer holds.
	// Sized for a universe's worth of skills and tasks, which is what an
	// agent spawn reads.
	memCapBytes = 32 << 20

	// memMaxEntryBytes keeps one large body from evicting the whole working
	// set. It is the control band's ceiling, not a number of its own:
	// anything above it does not travel this lane at all — it goes to the
	// object store — so holding it resident here would cache content this
	// store never serves.
	memMaxEntryBytes = contribapi.BandControl
)

// NewContentStore constructs a store rooted at the given directory.
// Callers pass `<CacheDir>/contributions`. The directory is created on
// first Put; Get tolerates absence (clean miss).
func NewContentStore(root string) *ContentStore {
	return &ContentStore{root: root}
}

// Root returns the absolute path the store is rooted at.
func (s *ContentStore) Root() string { return s.root }

// hashFileName strips the "sha256:" algorithm prefix (if present) so the
// on-disk file name is the bare hex digest. Returns "" for an empty or
// malformed hash (caller treats as "no content address").
func hashFileName(hash string) string {
	h := strings.TrimSpace(hash)
	if h == "" {
		return ""
	}
	if i := strings.IndexByte(h, ':'); i >= 0 {
		h = h[i+1:]
	}
	// Reject path-traversal / separators — the file name must be a bare
	// hex digest.
	if h == "" || strings.ContainsAny(h, "/\\.") {
		return ""
	}
	return h
}

// Get returns the cached body for the given hash and true when present,
// or (nil, false) on a clean miss. A read error other than not-exist is
// treated as a miss (the body is re-fetchable) — the store never fails
// a read; misses are repaired upstream.
func (s *ContentStore) Get(hash string) ([]byte, bool) {
	name := hashFileName(hash)
	if name == "" {
		return nil, false
	}
	if b, ok := s.memGet(name); ok {
		return b, true
	}
	b, err := os.ReadFile(filepath.Join(s.root, name))
	if err != nil {
		atomic.AddInt64(&s.misses, 1)
		return nil, false
	}
	s.memPut(name, b)
	return b, true
}

// memGet reads the resident layer.
func (s *ContentStore) memGet(name string) ([]byte, bool) {
	s.memMu.RLock()
	b, ok := s.mem[name]
	s.memMu.RUnlock()
	if ok {
		atomic.AddInt64(&s.hits, 1)
	}
	return b, ok
}

// memPut admits a body to the resident layer, evicting oldest-first until it
// fits. Bodies the caller may mutate are not a concern: everything here comes
// from a fresh ReadFile or a fetch, and callers treat bodies as read-only.
func (s *ContentStore) memPut(name string, b []byte) {
	if len(b) == 0 || len(b) > memMaxEntryBytes {
		return
	}
	s.memMu.Lock()
	defer s.memMu.Unlock()
	if s.mem == nil {
		s.mem = make(map[string][]byte)
	}
	if _, exists := s.mem[name]; exists {
		return
	}
	for s.memBytes+len(b) > memCapBytes && len(s.memOrder) > 0 {
		oldest := s.memOrder[0]
		s.memOrder = s.memOrder[1:]
		s.memBytes -= len(s.mem[oldest])
		delete(s.mem, oldest)
	}
	s.mem[name] = b
	s.memOrder = append(s.memOrder, name)
	s.memBytes += len(b)
}

// Stats reports resident-layer hits and disk misses since construction. Used
// by tests and the before/after measurement; not part of any wire surface.
func (s *ContentStore) Stats() (hits, misses int64) {
	return atomic.LoadInt64(&s.hits), atomic.LoadInt64(&s.misses)
}

// Put stores body under the given hash. Idempotent: a body already
// present (same hash) is left untouched (dedup). Verifies the supplied
// hash matches the body's actual sha256 — a mismatch is an error (the
// caller passed a wrong address, which would poison the store).
//
// An empty body with an empty hash is a no-op (nothing to store).
func (s *ContentStore) Put(hash string, body []byte) error {
	name := hashFileName(hash)
	if name == "" {
		if len(body) == 0 {
			return nil // nothing to store
		}
		return fmt.Errorf("content store: empty/invalid hash for non-empty body")
	}
	// Verify the address matches the content (no poisoning).
	sum := sha256.Sum256(body)
	want := hex.EncodeToString(sum[:])
	if name != want {
		return fmt.Errorf("content store: hash mismatch (claimed %q, actual sha256:%s)", hash, want)
	}
	return s.put(name, body)
}

// put writes body to the canonical file named `name` (a bare hex
// digest) via temp+rename. Caller has already resolved/verified the
// name.
func (s *ContentStore) put(name string, body []byte) error {
	finalPath := filepath.Join(s.root, name)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Already present → dedup, nothing to do.
	if _, err := os.Stat(finalPath); err == nil {
		return nil
	}

	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return fmt.Errorf("content store: mkdir %s: %w", s.root, err)
	}

	tmp, err := os.CreateTemp(s.root, name+".tmp-*")
	if err != nil {
		return fmt.Errorf("content store: create temp: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("content store: write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("content store: close temp: %w", err)
	}
	s.memPut(name, body)
	if err := os.Rename(tmpName, finalPath); err != nil {
		// A concurrent writer may have won the race and created the
		// canonical file — that's fine, the content is identical
		// (same hash). Treat an existing destination as success.
		if _, statErr := os.Stat(finalPath); statErr == nil {
			_ = os.Remove(tmpName)
			return nil
		}
		_ = os.Remove(tmpName)
		return fmt.Errorf("content store: rename %s → %s: %w", tmpName, finalPath, err)
	}
	return nil
}

// Has reports whether the hash is present without reading the body.
func (s *ContentStore) Has(hash string) bool {
	name := hashFileName(hash)
	if name == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(s.root, name))
	return err == nil
}
