// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// The consumer's store starts empty and stays empty until something is written
// into it. Installing a pack that references a large dataset must cost what
// installing a small one costs — so a read falls through to where the content
// was published rather than requiring a copy first.
func TestLayeredStoreReadsFromTheOriginWithoutCopying(t *testing.T) {
	ctx := context.Background()
	cacheRoot, originRoot := t.TempDir(), t.TempDir()

	content := bytes.Repeat([]byte("dataset."), 400_000) // 3.2 MB — bulk band
	hash := contribapi.HashContent(content)
	if !contribapi.BandOf(int64(len(content))).IsBulk() {
		t.Fatal("the fixture is not in the bulk band")
	}

	// Published: packaging wrote it where the catalog is published.
	origin := NewFileBlobStore(originRoot)
	if err := origin.Put(ctx, hash, bytes.NewReader(content)); err != nil {
		t.Fatal(err)
	}

	cache := NewFileBlobStore(cacheRoot)
	store := NewLayeredBlobStore(cache, origin)

	size, ok := store.Stat(ctx, hash)
	if !ok || size != int64(len(content)) {
		t.Fatalf("Stat = (%d, %v), want (%d, true)", size, ok, len(content))
	}

	rc, err := store.Open(ctx, hash, 0, -1)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("read %d bytes, want %d", len(got), len(content))
	}

	// Reading did NOT populate the cache. A layer in this band may be bigger
	// than the disk the cache sits on, so keeping a copy is a decision with
	// consequences and belongs to whoever knows why they are reading.
	entries, _ := os.ReadDir(cacheRoot)
	if len(entries) != 0 {
		t.Errorf("reading copied %d file(s) into the cache", len(entries))
	}
}

// A locator points at whichever layer holds the content, and resolving it is
// the same fetch either way — which is what lets the backend change without the
// consumer changing.
func TestLayeredStoreLocatesTheLayerThatHasIt(t *testing.T) {
	ctx := context.Background()
	cacheRoot, originRoot := t.TempDir(), t.TempDir()
	cache, origin := NewFileBlobStore(cacheRoot), NewFileBlobStore(originRoot)
	store := NewLayeredBlobStore(cache, origin)

	content := []byte("published once")
	hash := contribapi.HashContent(content)
	if err := origin.Put(ctx, hash, bytes.NewReader(content)); err != nil {
		t.Fatal(err)
	}

	loc, err := store.Locate(ctx, hash)
	if err != nil {
		t.Fatal(err)
	}
	rc, err := FetchBlob(ctx, loc, 0)
	if err != nil {
		t.Fatalf("the origin's locator did not resolve: %v", err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if !bytes.Equal(got, content) {
		t.Errorf("fetched %q, want %q", got, content)
	}

	// Once it IS local, the local copy wins — no round trip for content we hold.
	if err := cache.Put(ctx, hash, bytes.NewReader(content)); err != nil {
		t.Fatal(err)
	}
	loc, err = store.Locate(ctx, hash)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Base(cacheRoot); !bytes.Contains([]byte(loc), []byte(want)) {
		t.Errorf("locator %q does not point at the local store %q", loc, want)
	}
}

// Evicting reclaims space on THIS machine. An origin is where content was
// published; housekeeping here must not unpublish it for everyone.
func TestLayeredStoreEvictsLocallyOnly(t *testing.T) {
	ctx := context.Background()
	originRoot := t.TempDir()
	origin := NewFileBlobStore(originRoot)
	content := []byte("still published")
	hash := contribapi.HashContent(content)
	if err := origin.Put(ctx, hash, bytes.NewReader(content)); err != nil {
		t.Fatal(err)
	}

	store := NewLayeredBlobStore(NewFileBlobStore(t.TempDir()), origin)
	if err := store.Evict(ctx, hash); err != nil {
		t.Fatal(err)
	}
	if _, ok := origin.Stat(ctx, hash); !ok {
		t.Error("evicting locally removed the content from the origin")
	}
}

func TestLayeredStoreReportsContentNobodyHas(t *testing.T) {
	ctx := context.Background()
	store := NewLayeredBlobStore(NewFileBlobStore(t.TempDir()), NewFileBlobStore(t.TempDir()))
	if _, err := store.Locate(ctx, contribapi.HashContent([]byte("never published"))); err == nil {
		t.Error("located content no layer holds")
	}
}
