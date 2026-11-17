// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"io"
)

// blobstore_layered.go — a local store in front of the places content comes from.
//
// Content is written ONCE, at packaging, into the marketplace it is published
// to. A consumer holds none of it until something reads it — installing a pack
// that references a 40 GB dataset must cost what installing a 500-byte archive
// costs, or the bulk band has bought nothing.
//
// So the consumer's store is a CACHE with origins behind it, and a miss falls
// through rather than failing. That is the same shape as the control plane's
// content lane one band down (local store, then one bounded fetch), which is
// the point: a consumer's experience of "where did these bytes come from" is
// the same question at every size.

// LayeredBlobStore serves from a local store, falling back to read-only
// origins in order.
//
// Deliberately does NOT copy content into the cache on the way past. A layer in
// this band may be larger than the disk the cache sits on, so caching it is a
// decision with consequences and belongs to whoever knows why they are reading
// it — `StreamToFile` is how a consumer keeps content it wants to keep. The
// store's job is to say where the bytes are, not to decide that a copy is
// wanted.
type LayeredBlobStore struct {
	// Cache is the local, writable store. Writes go here and nowhere else:
	// an origin is somewhere content was published to, not somewhere a
	// consumer pushes.
	Cache BlobStore

	// Origins are consulted in order on a miss. A marketplace's blobs
	// directory is one; anything satisfying the contract is another.
	Origins []BlobStore
}

// NewLayeredBlobStore fronts `cache` with `origins`.
func NewLayeredBlobStore(cache BlobStore, origins ...BlobStore) *LayeredBlobStore {
	return &LayeredBlobStore{Cache: cache, Origins: origins}
}

// Put stores content locally. Origins are read-only.
func (s *LayeredBlobStore) Put(ctx context.Context, hash string, r io.Reader) error {
	if s.Cache == nil {
		return fmt.Errorf("blob: no local store to write %s into", hash)
	}
	return s.Cache.Put(ctx, hash, r)
}

// Stat reports the first layer that holds this hash.
func (s *LayeredBlobStore) Stat(ctx context.Context, hash string) (int64, bool) {
	if s.Cache != nil {
		if size, ok := s.Cache.Stat(ctx, hash); ok {
			return size, true
		}
	}
	for _, o := range s.Origins {
		if size, ok := o.Stat(ctx, hash); ok {
			return size, true
		}
	}
	return 0, false
}

// Locate mints an address from whichever layer holds the content.
//
// The address may point at an origin, and that is the intended outcome rather
// than a degraded one: the consumer streams from where the content was
// published, which is what "install is cheap, reading streams" means.
func (s *LayeredBlobStore) Locate(ctx context.Context, hash string) (string, error) {
	if s.Cache != nil {
		if _, ok := s.Cache.Stat(ctx, hash); ok {
			return s.Cache.Locate(ctx, hash)
		}
	}
	for _, o := range s.Origins {
		if _, ok := o.Stat(ctx, hash); ok {
			return o.Locate(ctx, hash)
		}
	}
	return "", fmt.Errorf("%w: %s", ErrBlobNotFound, hash)
}

// Open reads a byte range from whichever layer holds the content.
func (s *LayeredBlobStore) Open(ctx context.Context, hash string, offset, length int64) (io.ReadCloser, error) {
	if s.Cache != nil {
		if _, ok := s.Cache.Stat(ctx, hash); ok {
			return s.Cache.Open(ctx, hash, offset, length)
		}
	}
	for _, o := range s.Origins {
		if _, ok := o.Stat(ctx, hash); ok {
			return o.Open(ctx, hash, offset, length)
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrBlobNotFound, hash)
}

// Evict removes content from the LOCAL store only.
//
// An origin is somewhere content was published; reclaiming space on this
// machine must not reach back and unpublish it for everyone. Eviction is
// housekeeping on a cache, and only the cache is ours to keep house in.
func (s *LayeredBlobStore) Evict(ctx context.Context, hash string) error {
	if s.Cache == nil {
		return nil
	}
	return s.Cache.Evict(ctx, hash)
}

var _ BlobStore = (*LayeredBlobStore)(nil)
