// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"io"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// bulk_lane.go — serving a layer's content at any size.
//
// The catalog owns transport, so the band decision lives here rather than in
// every consumer. A caller asks for a layer and gets a reader; whether those
// bytes came from the index, from one bounded control-plane fetch, or from an
// object store over a freshly-minted locator is not its concern, and is not
// something it can get wrong.

// SetBulkLane attaches the object store that serves large layers.
//
// Optional. Without it, a bulk-band layer reports that its content is not
// reachable rather than silently falling back to the control plane — which
// would push content past the payload limit and fail somewhere less
// informative.
func (c *MemoryCatalog) SetBulkLane(blobs BlobStore) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.blobs = blobs
}

// OpenLayer streams one layer's content, choosing transport from its size.
//
// Satisfies mbapi.LayerOpener, which is how the generic read surface reaches a
// catalog's transport without knowing anything about it.
//
// The returned reader is the caller's to close. For the bulk band it is a live
// handle on the object store: nothing has been read, so the caller decides
// whether to stream it to disk, read a prefix, or abandon it — and abandoning
// it costs nothing.
func (c *MemoryCatalog) OpenLayer(ctx context.Context, uri, layer string) (io.ReadCloser, contribapi.LayerRef, error) {
	entry, found := c.ContributionAt(uri)
	if !found {
		return nil, contribapi.LayerRef{}, notInCatalog(uri)
	}
	if layer == "" {
		// The KIND names its default layer; this catalog holds every
		// declaration, so the answer is one lookup away and never a constant.
		layer = c.kinds().DefaultLayer(entry.Type)
	}
	c.mu.RLock()
	store, layers, fetch, blobs := c.content, c.layers, c.fetch, c.blobs
	c.mu.RUnlock()

	// The band decision and its typed errors are contribapi.OpenLayerFrom's;
	// this view contributes its lanes. An undeclared layer is refused before
	// any lane runs, for every layer name — data and source included (e14).
	lanes := contribapi.LayerLanes{
		// A layer the view does not carry: bytes already held by hash, else
		// the layer lane by name (any layer but data), else the body lane
		// (the data layer only — a lane never answers another lane's
		// question). OpenLayerFrom verifies what comes back against the
		// address before anything is served.
		Fetch: func(ref contribapi.LayerRef) ([]byte, string, error) {
			if store != nil {
				if b, ok := store.Get(ref.Hash); ok {
					return b, "", nil
				}
			}
			if layer != contribapi.LayerData {
				if layers == nil {
					return nil, "", contribapi.ErrLayerNotCarried{URI: uri, Layer: layer}
				}
				body, got, err := layers(ctx, uri, layer)
				if err != nil {
					return nil, "", fmt.Errorf("open %s layer %q: %w", uri, layer, err)
				}
				// Put verifies the address itself: a mismatch is not
				// stored, and OpenLayerFrom refuses it typed.
				if store != nil && len(body) > 0 {
					_ = store.Put(ref.Hash, body)
				}
				return body, got.MediaType, nil
			}
			if fetch == nil {
				return nil, "", contribapi.ErrLayerNotCarried{URI: uri, Layer: layer}
			}
			body, err := fetchVerified(ctx, fetch, store, ref.Hash, uri)
			return body, "", err
		},
	}
	if blobs != nil {
		lanes.Bulk = func(ref contribapi.LayerRef) (io.ReadCloser, error) { return c.openBulk(ctx, ref) }
	}
	return contribapi.OpenLayerFrom(entry, layer, lanes)
}

// openBulk resolves a large layer through the object store.
//
// The locator is minted HERE, at fetch time, and thrown away after. It is never
// written back into the index: an address can be short-lived or scoped, and an
// index holding one would be an index that goes stale on its own.
//
// Nothing in this path contacts the producing component. The object store holds
// the content, so a large layer is readable whether or not whoever published it
// is running.
func (c *MemoryCatalog) openBulk(ctx context.Context, ref contribapi.LayerRef) (io.ReadCloser, error) {
	c.mu.RLock()
	blobs := c.blobs
	c.mu.RUnlock()
	if blobs == nil {
		return nil, fmt.Errorf("layer %s is %d bytes (bulk band) and no object store is wired: content is not reachable",
			ref.Hash, ref.Size)
	}
	locator, err := blobs.Locate(ctx, ref.Hash)
	if err != nil {
		return nil, fmt.Errorf("locate bulk layer %s: %w", ref.Hash, err)
	}
	rc, err := FetchBlob(ctx, locator, 0)
	if err != nil {
		return nil, fmt.Errorf("fetch bulk layer %s: %w", ref.Hash, err)
	}
	return rc, nil
}

// fetchVerified is the content lane's miss path: one bounded fetch by hash,
// then the address check, then the store. The producer hashed exactly the
// bytes it carries (contribapi.HashContent over what SetData received) and
// every hop moves those bytes verbatim — LayerRef.Inline and GetBodyOutput.
// Content are []byte, base64 over JSON, and the hub's tier-1 file keeps them
// the same way — so the fetched bytes must hash to the index's address. A
// body that does not is refused (contribapi.ErrContentMismatch) and never
// stored: a content-addressed store keyed by an address its content does not
// have would serve the wrong bytes to every later reader.
func fetchVerified(ctx context.Context, fetch BodyFetcher, store *ContentStore, hash, uri string) ([]byte, error) {
	body, err := fetch(ctx, hash, uri)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", uri, err)
	}
	if err := contribapi.VerifyContent(uri, contribapi.LayerData, hash, body); err != nil {
		return nil, err
	}
	if store != nil {
		// Best-effort cache write — a racy store write must not fail the
		// read; the address is already verified, so Put's own check holds.
		_ = store.Put(hash, body)
	}
	return body, nil
}

var _ mbapi.LayerOpener = (*MemoryCatalog)(nil)

// kinds builds the kind registry from every declaration this catalog holds.
func (c *MemoryCatalog) kinds() contribapi.Kinds {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var all []contribapi.Contribution
	for _, name := range c.members {
		if m := c.by[name]; m != nil {
			all = append(all, m.Contributions...)
		}
	}
	return contribapi.KindsFrom(all)
}
