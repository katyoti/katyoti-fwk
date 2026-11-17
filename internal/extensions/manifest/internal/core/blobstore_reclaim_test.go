// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	"context"
	"os"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

func ref(hash string, size int64) contribapi.LayerRef {
	return contribapi.LayerRef{Hash: hash, Size: size}
}

// Reclaiming is by hash, from the whole index. Several contributions may share
// one blob, so a contribution disappearing says nothing on its own about
// whether its content is still in use.
func TestReclaimKeepsContentAnotherEntryStillUses(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileBlobStore(root)

	shared, dropped := []byte("shared dataset"), []byte("nobody's content")
	sharedHash, droppedHash := contribapi.HashContent(shared), contribapi.HashContent(dropped)
	for h, b := range map[string][]byte{sharedHash: shared, droppedHash: dropped} {
		if err := store.Put(ctx, h, bytes.NewReader(b)); err != nil {
			t.Fatal(err)
		}
	}

	// `a` was deleted; `b` still points at the same content.
	entries := []contribapi.Contribution{{
		Provider: "p", Type: "reference", ID: "b",
		Layers: map[string]contribapi.LayerRef{contribapi.LayerSource: ref(sharedHash, int64(len(shared)))},
	}}

	n, err := Reclaim(ctx, entries, store)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("evicted %d, want 1", n)
	}
	if _, ok := store.Stat(ctx, sharedHash); !ok {
		t.Error("content another entry still references was evicted")
	}
	if _, ok := store.Stat(ctx, droppedHash); ok {
		t.Error("unreferenced content still resolves")
	}

	// Nothing held the content open, so the bytes went with the eviction and
	// Sweep has nothing left to do. Sweep exists for the other case — a reader
	// that has not let go — and running it here proves it is not needed to
	// reclaim space in the ordinary one.
	if got := store.Sweep(); got != 0 {
		t.Errorf("Sweep reclaimed %d; eviction should already have", got)
	}
	if left, _ := os.ReadDir(root); len(left) != 1 {
		t.Errorf("%d file(s) left on disk, want 1", len(left))
	}
}

// An index that is empty because it has not been built yet looks exactly like
// an index where nothing is referenced. The two have opposite right answers, so
// the ambiguous case does nothing rather than deleting everything.
func TestReclaimRefusesToActOnAnEmptyIndex(t *testing.T) {
	ctx := context.Background()
	store := NewFileBlobStore(t.TempDir())
	content := []byte("precious")
	hash := contribapi.HashContent(content)
	if err := store.Put(ctx, hash, bytes.NewReader(content)); err != nil {
		t.Fatal(err)
	}

	n, err := Reclaim(ctx, nil, store)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("an empty index evicted %d blob(s)", n)
	}
	if _, ok := store.Stat(ctx, hash); !ok {
		t.Error("an empty index deleted the store's content")
	}
}

// A body's hash is an address in the same space as a layer's, so marking has to
// count it — otherwise reclaiming deletes content the control plane is serving.
func TestReclaimMarksBodyHashesToo(t *testing.T) {
	ctx := context.Background()
	store := NewFileBlobStore(t.TempDir())
	body := []byte(`{"name":"thing"}`)
	hash := contribapi.HashContent(body)
	if err := store.Put(ctx, hash, bytes.NewReader(body)); err != nil {
		t.Fatal(err)
	}

	entries := []contribapi.Contribution{{Provider: "p", Type: "skill", ID: "thing", Layers: contribapi.DataDescriptor(hash, int64(len(body)))}}
	if _, err := Reclaim(ctx, entries, store); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Stat(ctx, hash); !ok {
		t.Error("a referenced body's content was evicted")
	}
}

// A store that cannot say what it holds cannot be reclaimed from, and says so
// rather than silently reporting that there was nothing to do.
func TestReclaimReportsAStoreThatCannotEnumerate(t *testing.T) {
	entries := []contribapi.Contribution{{Provider: "p", Type: "skill", ID: "x"}}
	if _, err := Reclaim(context.Background(), entries, opaqueStore{}); err == nil {
		t.Error("reclaiming from a non-enumerable store reported success")
	}
}

type opaqueStore struct{ BlobStore }
