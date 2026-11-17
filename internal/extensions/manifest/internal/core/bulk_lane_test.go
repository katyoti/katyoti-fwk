// SPDX-License-Identifier: Apache-2.0

package core_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/manifest/internal/core"
)

// bulk_lane_test.go — content larger than any message payload, end to end.
//
// The size below is deliberately past the control plane's 1 MiB limit: before
// the bulk plane this content could not be represented at all, let alone moved.

const bulkTestSize = 3 << 20 // 3 MiB — comfortably past the 1 MiB payload limit

// bulkContent returns incompressible bytes plus their content address.
// Incompressible on purpose: compressible filler would let a backend cheat the
// memory claim this test makes.
func bulkContent(t *testing.T, n int) ([]byte, string) {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return b, contribapi.HashContent(b)
}

// bulkCatalog wires a catalog whose one contribution declares a bulk-band
// source layer held in an object store.
func bulkCatalog(t *testing.T, content []byte, hash string) (*core.MemoryCatalog, *core.FileBlobStore) {
	t.Helper()
	blobs := core.NewFileBlobStore(t.TempDir())
	if err := blobs.Put(context.Background(), hash, bytes.NewReader(content)); err != nil {
		t.Fatalf("seed blob: %v", err)
	}
	cat := core.NewMemoryCatalog([]*mbapi.Manifest{{
		Provider: "ktw-docs",
		Contributions: []contribapi.Contribution{{
			Provider: "ktw-docs", Type: "reference", ID: "dataset.bin",
			Layers: map[string]contribapi.LayerRef{
				contribapi.LayerSource: {
					Hash: hash, Size: int64(len(content)), MediaType: "application/octet-stream",
				},
			},
		}},
	}}, nil)
	cat.SetContentLane(core.NewContentStore(t.TempDir()), func(context.Context, string, string) ([]byte, error) {
		t.Fatal("a bulk layer must never be fetched over the control plane")
		return nil, nil
	})
	cat.SetBulkLane(blobs)
	return cat, blobs
}

// TestBulkLayer_RoundTripsWithoutMemoryGrowth is the headline proof: content
// past the payload limit reaches disk intact, and peak heap does not grow with
// the content's size.
//
// The control-plane fetcher fails the test if called, so this also proves the
// bulk band never falls back to the transport that cannot carry it.
func TestBulkLayer_RoundTripsWithoutMemoryGrowth(t *testing.T) {
	content, hash := bulkContent(t, bulkTestSize)
	cat, _ := bulkCatalog(t, content, hash)
	ctx := context.Background()
	q := mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat}

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	l, err := mbapi.Open(ctx, q, "ktw-docs:/reference/dataset.bin", contribapi.LayerSource)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if l.Band() != contribapi.BandNameBulk {
		t.Fatalf("band = %q, want bulk", l.Band())
	}
	// Bytes must refuse — and refuse without transferring anything.
	if _, err := l.Bytes(); err == nil {
		t.Error("Bytes materialised a bulk layer")
	}

	l, err = mbapi.Open(ctx, q, "ktw-docs:/reference/dataset.bin", contribapi.LayerSource)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "dataset.bin")
	n, err := l.WriteFile(out)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	runtime.ReadMemStats(&after)
	grew := int64(after.TotalAlloc - before.TotalAlloc)

	if n != int64(len(content)) {
		t.Errorf("wrote %d bytes, want %d", n, len(content))
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Error("round-tripped content differs from the original")
	}
	// Streaming means the transfer costs a buffer, not the content. Half the
	// content size is a generous ceiling that still fails loudly if anyone
	// makes this path materialise.
	if grew > bulkTestSize/2 {
		t.Errorf("allocated %d bytes moving %d bytes of content — the bulk path is materialising, not streaming",
			grew, bulkTestSize)
	}
}

// TestBulkLayer_FetchContactsNoProducer asserts reading large content needs
// nothing from whoever published it: the object store holds the bytes, so the
// producing component may be stopped, redeployed or gone.
func TestBulkLayer_FetchContactsNoProducer(t *testing.T) {
	content, hash := bulkContent(t, bulkTestSize)
	cat, _ := bulkCatalog(t, content, hash)
	// A body fetcher that fails the test IS the assertion: the control plane is
	// the only route back to a producer, and it is never taken.
	l, err := mbapi.Open(context.Background(),
		mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat},
		"ktw-docs:/reference/dataset.bin", contribapi.LayerSource)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	head := make([]byte, 16)
	if _, err := io.ReadFull(l, head); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(head, content[:16]) {
		t.Error("first bytes differ")
	}
}

// TestBlobStore_RangeReadsResume asserts partial reads work from an arbitrary
// offset — what makes an interrupted large transfer continue rather than start
// over.
func TestBlobStore_RangeReads(t *testing.T) {
	content, hash := bulkContent(t, 64<<10)
	blobs := core.NewFileBlobStore(t.TempDir())
	ctx := context.Background()
	if err := blobs.Put(ctx, hash, bytes.NewReader(content)); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct{ offset, length int64 }{
		{0, 100},
		{1000, 500},
		{int64(len(content)) - 10, 10},
		{2048, -1}, // to the end
	} {
		t.Run(fmt.Sprintf("offset=%d/len=%d", tc.offset, tc.length), func(t *testing.T) {
			rc, err := blobs.Open(ctx, hash, tc.offset, tc.length)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = rc.Close() }()
			got, err := io.ReadAll(rc)
			if err != nil {
				t.Fatal(err)
			}
			want := content[tc.offset:]
			if tc.length > 0 {
				want = want[:tc.length]
			}
			if !bytes.Equal(got, want) {
				t.Errorf("range read returned %d bytes, want %d", len(got), len(want))
			}
		})
	}
}

// TestBlobStore_ResumeViaLocator asserts a fetch can continue from an offset
// through the locator path — the same code that will receive an https:// URL
// from an object store.
func TestBlobStore_ResumeViaLocator(t *testing.T) {
	content, hash := bulkContent(t, 128<<10)
	blobs := core.NewFileBlobStore(t.TempDir())
	ctx := context.Background()
	if err := blobs.Put(ctx, hash, bytes.NewReader(content)); err != nil {
		t.Fatal(err)
	}
	locator, err := blobs.Locate(ctx, hash)
	if err != nil {
		t.Fatal(err)
	}

	const stoppedAt = 40 << 10
	rc, err := core.FetchBlob(ctx, locator, stoppedAt)
	if err != nil {
		t.Fatalf("resume from %d: %v", stoppedAt, err)
	}
	defer func() { _ = rc.Close() }()
	rest, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(rest, content[stoppedAt:]) {
		t.Errorf("resumed read returned %d bytes, want %d", len(rest), len(content)-stoppedAt)
	}
}

// TestBlobStore_DedupAndHashVerification asserts hash IS identity: the same
// content stored twice costs one copy, and content that does not match its
// claimed address is refused rather than stored at a lie.
func TestBlobStore_DedupAndHashVerification(t *testing.T) {
	content, hash := bulkContent(t, 4096)
	blobs := core.NewFileBlobStore(t.TempDir())
	ctx := context.Background()

	if err := blobs.Put(ctx, hash, bytes.NewReader(content)); err != nil {
		t.Fatal(err)
	}
	// Same content again: dedup, not a second copy.
	if err := blobs.Put(ctx, hash, bytes.NewReader(content)); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(blobs.Root())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("store holds %d files for one content address, want 1", len(entries))
	}

	// Content that does not hash to its claimed address is refused, and leaves
	// nothing behind — storing it would put bytes at an address that lies
	// about them, which every consumer downstream would then trust.
	_, otherHash := bulkContent(t, 4096)
	if err := blobs.Put(ctx, otherHash, bytes.NewReader(content)); err == nil {
		t.Error("content was stored under an address that does not describe it")
	}
	if _, ok := blobs.Stat(ctx, otherHash); ok {
		t.Error("a refused Put left content behind")
	}
}

// TestBlobStore_EvictByHashWithReaderOpen asserts eviction is keyed by hash and
// survives a reader holding the content — on Windows an open file cannot simply
// be deleted, and eviction is housekeeping, not something a caller can act on.
func TestBlobStore_EvictByHashWithReaderOpen(t *testing.T) {
	content, hash := bulkContent(t, 8192)
	blobs := core.NewFileBlobStore(t.TempDir())
	ctx := context.Background()
	if err := blobs.Put(ctx, hash, bytes.NewReader(content)); err != nil {
		t.Fatal(err)
	}

	rc, err := blobs.Open(ctx, hash, 0, -1)
	if err != nil {
		t.Fatal(err)
	}
	if err := blobs.Evict(ctx, hash); err != nil {
		t.Errorf("Evict with a reader open returned %v; eviction must not fail a caller", err)
	}
	// The open reader keeps working: evicting content someone is reading must
	// not corrupt the read in progress.
	got, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatalf("reading an evicted-but-open blob: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Error("an in-flight read saw different content after eviction")
	}
	// And it is no longer addressable.
	if _, ok := blobs.Stat(ctx, hash); ok {
		t.Error("evicted content is still addressable")
	}
	blobs.Sweep()
}

// TestBulkLayer_NoObjectStoreIsReported asserts a bulk layer with no store
// wired says so, rather than silently trying the transport that cannot carry
// it and failing somewhere less informative.
func TestBulkLayer_NoObjectStoreIsReported(t *testing.T) {
	content, hash := bulkContent(t, bulkTestSize)
	cat := core.NewMemoryCatalog([]*mbapi.Manifest{{
		Provider: "ktw-docs",
		Contributions: []contribapi.Contribution{{
			Provider: "ktw-docs", Type: "reference", ID: "dataset.bin",
			Layers: map[string]contribapi.LayerRef{
				contribapi.LayerSource: {Hash: hash, Size: int64(len(content))},
			},
		}},
	}}, nil)

	_, err := mbapi.Open(context.Background(),
		mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat},
		"ktw-docs:/reference/dataset.bin", contribapi.LayerSource)
	if err == nil {
		t.Fatal("a bulk layer with no object store was opened")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("bulk band")) {
		t.Errorf("error does not say why: %v", err)
	}
}

// TestBulkLayer_ReadsFromWhereItWasPublished closes the loop c21 opened: a
// consumer whose own store is empty still reads a bulk layer, by falling
// through to the marketplace the content was published to.
//
// This is the whole shape of "install is cheap, reading streams". A pack that
// references a large dataset installs as a few hundred bytes; the consumer
// holds none of the content; and the first read gets it from where packaging
// put it, with no copy made on the way past.
func TestBulkLayer_ReadsFromWhereItWasPublished(t *testing.T) {
	ctx := context.Background()
	content, hash := bulkContent(t, bulkTestSize)

	// Published: packaging wrote the blob beside the catalog it published.
	published := core.NewFileBlobStore(t.TempDir())
	if err := published.Put(ctx, hash, bytes.NewReader(content)); err != nil {
		t.Fatal(err)
	}
	// The consumer: an empty local cache in front of it.
	cacheRoot := t.TempDir()
	consumer := core.NewLayeredBlobStore(core.NewFileBlobStore(cacheRoot), published)

	cat := core.NewMemoryCatalog([]*mbapi.Manifest{{
		Provider: "bigpack",
		Contributions: []contribapi.Contribution{{
			Provider: "bigpack", Type: "reference", ID: "dataset.bin",
			Layers: map[string]contribapi.LayerRef{
				contribapi.LayerSource: {
					Hash: hash, Size: int64(len(content)), MediaType: "application/octet-stream",
				},
			},
		}},
	}}, nil)
	cat.SetContentLane(core.NewContentStore(t.TempDir()), func(context.Context, string, string) ([]byte, error) {
		t.Fatal("a bulk layer must never be fetched over the control plane")
		return nil, nil
	})
	cat.SetBulkLane(consumer)

	q := mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat}
	l, err := mbapi.Open(ctx, q, "bigpack:/reference/dataset.bin", contribapi.LayerSource)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if l.Band() != contribapi.BandNameBulk {
		t.Fatalf("band = %q, want bulk", l.Band())
	}
	dest := filepath.Join(t.TempDir(), "dataset.bin")
	n, err := l.WriteFile(dest)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if n != int64(len(content)) {
		t.Fatalf("wrote %d bytes, want %d", n, len(content))
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Error("the content that arrived is not the content that was published")
	}
}
