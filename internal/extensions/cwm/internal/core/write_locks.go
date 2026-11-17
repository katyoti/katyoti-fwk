// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"math/rand"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/atomicio"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// WRITES ARE SERIALISED. One process holds the store (the lockfile), so an
// in-process lock per object is enough: it covers only the short section that
// compares a revision and writes the file and the index — never a hook, never
// a nested verb — so it cannot deadlock. A read-modify-write that runs hooks
// (ObjectUpdate's callers) carries the REVISION it read instead and is
// refused with ErrConflict when the object moved on; CWM's own such verbs
// read again and redo their change (retryOnConflict).

// writeLocks holds one mutex per key: an object id, or "create:<module>" for
// the minting of ids.
type writeLocks struct {
	m sync.Map
	// reserved are ids a create in flight has claimed: minted or checked free,
	// not yet written. Minting skips them and a second create of one is
	// refused, without holding a lock across the create's hooks.
	reserved sync.Map
}

// lock takes the key's mutex and returns its release.
func (w *writeLocks) lock(key string) func() {
	v, _ := w.m.LoadOrStore(key, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// holdsStore refuses a write once this process lost the store's lock to
// another: the other one writes the store now, and a write
// from here would overwrite its work.
func (s *Service) holdsStore() error {
	if s.Storage == nil {
		return nil
	}
	return s.Storage.CheckLease()
}

// fileRevision is the version of an object file: the hash of its bytes.
func fileRevision(path string) (string, error) {
	raw, err := atomicio.ReadAtomic(path)
	if err != nil {
		return "", err
	}
	return hashBytes(raw), nil
}

// readObjectFileRev reads an object and the revision of the very bytes it was
// decoded from: two reads could straddle a write and pair an old object with
// a new revision.
func readObjectFileRev(path string) (api.Object, string, error) {
	info, obj, rev, ok := cachedObjectFile(path)
	if ok {
		return obj, rev, nil
	}
	raw, err := atomicio.ReadAtomic(path)
	if err != nil {
		return api.Object{}, "", err
	}
	obj, err = decodeObjectFile(path, raw)
	if err != nil {
		return api.Object{}, "", err
	}
	rev = hashBytes(raw)
	keepObjectFile(path, info, obj, rev)
	return obj, rev, nil
}

// updateFresh reads the stored object, applies change and writes it back on
// the revision read, again from a fresh read when someone wrote in between: a
// verb of CWM that edits another object never writes over a newer version.
// change returns false when there is nothing to write.
func (s *Service) updateFresh(ctx stdctx.Context, id string, change func(*api.Object) bool) error {
	_, err := retryOnConflict(func() (struct{}, error) {
		obj, module, rev, err := s.fullObjectRev(ctx, id)
		if err != nil {
			return struct{}{}, err
		}
		if !change(&obj) {
			return struct{}{}, nil
		}
		_, err = s.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: module, Object: obj, Revision: rev})
		return struct{}{}, err
	})
	return err
}

// conflictRetries bounds how often a verb of CWM redoes its read-modify-write
// after losing a race: a conflict means someone else's write landed, so the
// next attempt usually succeeds; under many writers on one object the
// attempts back off with jitter so they stop colliding. The bound keeps a
// hot object from spinning for ever.
const conflictRetries = 64

// retryOnConflict runs fn again while it fails with ErrConflict, waiting a
// little longer (with jitter) after each loss.
func retryOnConflict[T any](fn func() (T, error)) (T, error) {
	var out T
	var err error
	for i := 0; i < conflictRetries; i++ {
		out, err = fn()
		if !errors.Is(err, api.ErrConflict) {
			return out, err
		}
		wait := time.Duration(i+1) * 500 * time.Microsecond
		if wait > 20*time.Millisecond {
			wait = 20 * time.Millisecond
		}
		time.Sleep(time.Duration(rand.Int63n(int64(wait)) + 1))
	}
	return out, err
}
