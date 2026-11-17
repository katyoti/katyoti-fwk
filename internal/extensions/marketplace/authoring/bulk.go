// SPDX-License-Identifier: Apache-2.0

package authoring

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
)

// bulk.go — getting large content out of the pack and into the store.
//
// A pack is bounded by the NUMBER of contributions it carries, not by their
// content size. That is the same rule the index follows, applied one level out:
// a 40 GB dataset makes a pack three lines longer instead of 40 GB bigger.
//
// Packaging is where that happens, and the only place it happens. A binary that
// only publishes content must not carry upload code or store credentials — the
// cost of a capability belongs with whoever exercises it, and a publish-only
// sidecar does not exercise distribution. Content is immutable and
// content-addressed, so it can be written once, at build time, and never again.

// BlobsDir is where a marketplace keeps its content, beside the catalog it
// already publishes. The store IS the marketplace folder: it is already
// hash-addressed, already signed, already served — so bulk layers travel the
// distribution chain that exists rather than a second one built beside it.
const BlobsDir = "blobs"

// HoistedLayer is one file moved out of a pack and into the store.
type HoistedLayer struct {
	// Path is the file's location within the pack, slash-separated. What the
	// recorded manifest is keyed by, and what packing must skip.
	Path string
	Ref  contribapi.LayerRef
}

// HoistBulkLayers writes every bulk-band file in `folder` to the store under
// `blobRoot`, records their descriptors in the pack, and returns what it moved.
//
// The files are left on disk. Removing an author's content would be a
// destructive side effect of building — the pack is what excludes them (see
// BulkSkipper), and the author's folder stays what it was.
//
// Nothing is read whole at any point: each file is streamed twice, once to
// compute its address and once to store it. Two passes rather than one because
// the store verifies content against a claimed address, and an address cannot
// be claimed before it is known.
func HoistBulkLayers(ctx context.Context, folder, blobRoot string) ([]HoistedLayer, error) {
	store := manifestlocal.NewFileBlobStore(blobRoot)

	var moved []HoistedLayer
	err := filepath.Walk(folder, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return err
		}
		if !contribapi.BandOf(fi.Size()).IsBulk() {
			return nil
		}
		rel, rerr := filepath.Rel(folder, p)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)

		f, oerr := os.Open(p)
		if oerr != nil {
			return fmt.Errorf("open %s: %w", rel, oerr)
		}
		hash, size, herr := contribapi.HashReader(f)
		_ = f.Close()
		if herr != nil {
			return fmt.Errorf("hash %s: %w", rel, herr)
		}

		// Already stored is the common case across versions: two releases
		// sharing an unchanged dataset share one blob, so republishing
		// transfers nothing. Put is idempotent, but checking first means the
		// file is not even re-read.
		if _, present := store.Stat(ctx, hash); !present {
			f, oerr = os.Open(p)
			if oerr != nil {
				return fmt.Errorf("reopen %s: %w", rel, oerr)
			}
			perr := store.Put(ctx, hash, f)
			_ = f.Close()
			if perr != nil {
				return fmt.Errorf("store %s: %w", rel, perr)
			}
		}

		moved = append(moved, HoistedLayer{
			Path: rel,
			Ref: contribapi.LayerRef{
				Hash: hash, Size: size, MediaType: mediaTypeOf(rel),
			},
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(moved, func(i, j int) bool { return moved[i].Path < moved[j].Path })
	recorded := make(map[string]contribapi.LayerRef, len(moved))
	for _, m := range moved {
		recorded[m.Path] = m.Ref
	}
	if err := contribapi.WriteLayerManifest(folder, recorded); err != nil {
		return nil, err
	}
	return moved, nil
}

// BulkSkipper reports which paths packing must leave out.
//
// Returned as a predicate rather than applied by HoistBulkLayers, because
// excluding content is the ARCHIVE's decision and belongs where the archive is
// built. A caller that wants a self-contained bundle, with no network at
// install time, hoists and then packs everything anyway.
func BulkSkipper(moved []HoistedLayer) func(rel string) bool {
	if len(moved) == 0 {
		return nil
	}
	skip := make(map[string]bool, len(moved))
	for _, m := range moved {
		skip[m.Path] = true
	}
	return func(rel string) bool { return skip[filepath.ToSlash(rel)] }
}

// mediaTypeOf is the media type for a pack file, from its extension.
//
// Deliberately coarse: a bulk layer is content a consumer streams to disk or
// hands to something else, so the media type is a hint on a descriptor rather
// than a dispatch key. Anything unrecognised is an octet stream, which is the
// honest answer for arbitrary bytes.
func mediaTypeOf(rel string) string {
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".md", ".markdown":
		return "text/markdown"
	case ".json":
		return "application/json"
	case ".yaml", ".yml":
		return "application/yaml"
	case ".txt", ".csv":
		return "text/plain"
	case ".pdf":
		return "application/pdf"
	case ".zip":
		return "application/zip"
	case ".gz", ".tgz":
		return "application/gzip"
	default:
		return "application/octet-stream"
	}
}
