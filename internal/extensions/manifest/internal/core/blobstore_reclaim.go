// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"os"
	"strings"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// blobstore_reclaim.go — getting space back, by hash.
//
// Content is immutable and addressed by what it is, so nothing can be reclaimed
// by asking who owns it: several contributions may share one blob, and a URI
// that no longer exists says nothing about whether its content is still
// referenced elsewhere. The only sound question is "does the index still point
// at this hash", which is a question about the WHOLE index, not about any one
// entry.
//
// So: mark from the aggregated view, sweep by hash. The same shape every
// content-addressed store reaches for, for the same reason.

// LiveHashes returns every layer hash the entries still reference.
//
// Derived from the aggregated view rather than accumulated as entries come and
// go. A running tally would have to be correct across every path that adds or
// removes a contribution, and being wrong in the removing direction deletes
// content that is still in use — a failure that surfaces later, somewhere else,
// as content that cannot be found.
func LiveHashes(entries []contribapi.Contribution) map[string]bool {
	live := make(map[string]bool, len(entries))
	for _, c := range entries {
		for _, l := range c.Layers {
			if l.Hash != "" {
				live[l.Hash] = true
			}
		}
		if c.DataHash() != "" {
			live[c.DataHash()] = true
		}
	}
	return live
}

// Reclaim evicts everything in `store` that `entries` no longer reference, and
// returns how many addresses stopped resolving.
//
// Eviction and reclamation are separate on purpose (see BlobStore.Evict): a
// reader holding content open prevents its removal, so an address stops
// resolving at once and the bytes come back when the holder lets go. A caller
// that wants the space now calls Sweep after.
//
// Refuses to run on an EMPTY index. An index that is empty because it has not
// been built yet looks exactly like an index where nothing is referenced, and
// the two have opposite right answers — so the ambiguous case does nothing
// rather than deleting everything.
func Reclaim(ctx context.Context, entries []contribapi.Contribution, store BlobStore) (int, error) {
	if len(entries) == 0 {
		return 0, nil
	}
	lister, ok := store.(BlobLister)
	if !ok {
		return 0, fmt.Errorf("blob: %T cannot enumerate what it holds, so nothing can be reclaimed from it", store)
	}
	held, err := lister.Hashes()
	if err != nil {
		return 0, err
	}
	live := LiveHashes(entries)
	evicted := 0
	for _, hash := range held {
		if live[hash] {
			continue
		}
		if err := store.Evict(ctx, hash); err != nil {
			return evicted, err
		}
		evicted++
	}
	return evicted, nil
}

// BlobLister is implemented by a store that can say what it holds.
//
// Optional, because reclaiming needs it and reading does not — and the two are
// different operations with different costs. A filesystem store reads a
// directory; an object store pages through a listing. Requiring it of every
// backend would put a bulk operation in the contract every consumer implements.
type BlobLister interface {
	// Hashes returns every address the store currently resolves, tombstoned
	// ones excluded — content already evicted is not held, whether or not its
	// bytes have been reclaimed yet.
	Hashes() ([]string, error)
}

// Hashes implements BlobLister.
func (s *FileBlobStore) Hashes() ([]string, error) {
	entries, err := os.ReadDir(s.root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("blob: list %s: %w", s.root, err)
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		// Partial writes in flight are not content the store holds.
		if e.IsDir() || strings.Contains(name, ".tmp-") || strings.Contains(name, ".part-") {
			continue
		}
		if s.isEvicted(name) {
			continue
		}
		out = append(out, "sha256:"+name)
	}
	return out, nil
}
