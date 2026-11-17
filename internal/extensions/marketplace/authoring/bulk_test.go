// SPDX-License-Identifier: Apache-2.0

package authoring

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore/frozenhost"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

func bulkContent(n int) []byte { return bytes.Repeat([]byte("dataset."), n/8+1)[:n] }

func packWithALargeFile(t *testing.T, size int) (folder string, content []byte) {
	t.Helper()
	folder = t.TempDir()
	content = bulkContent(size)
	if err := os.MkdirAll(filepath.Join(folder, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	for path, data := range map[string][]byte{
		"ktw-host.yaml":          []byte("name: bigpack\n"),
		"skills/usage.md":        []byte("---\nname: usage\ndescription: How to.\n---\n\nBody.\n"),
		"references/dataset.bin": content,
	} {
		full := filepath.Join(folder, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return folder, content
}

// A pack is bounded by the NUMBER of contributions it carries, not their
// content size. That is the whole claim of the bulk band on the producer side,
// and it is only true if the archive actually leaves the content out.
func TestPackStaysSmallWhenItsContentIsLarge(t *testing.T) {
	folder, content := packWithALargeFile(t, 3<<20) // 3 MiB
	blobs := t.TempDir()

	moved, err := HoistBulkLayers(context.Background(), folder, blobs)
	if err != nil {
		t.Fatal(err)
	}
	if len(moved) != 1 || moved[0].Path != "references/dataset.bin" {
		t.Fatalf("hoisted %v, want the dataset", moved)
	}
	if want := contribapi.HashContent(content); moved[0].Ref.Hash != want {
		t.Errorf("hash %s, want %s", moved[0].Ref.Hash, want)
	}
	if moved[0].Ref.Size != int64(len(content)) {
		t.Errorf("size %d, want %d", moved[0].Ref.Size, len(content))
	}

	archive, err := frozenhost.TarGzDirExcept(folder, BulkSkipper(moved))
	if err != nil {
		t.Fatal(err)
	}
	// The archive holds two small files and a recorded manifest. Anything near
	// the content's size means the exclusion did not happen.
	if len(archive) > 4096 {
		t.Errorf("archive is %d bytes for a pack holding %d bytes of content", len(archive), len(content))
	}

	// The content is in the store, addressed by its hash.
	name := moved[0].Ref.Hash[len("sha256:"):]
	stored, err := os.ReadFile(filepath.Join(blobs, name))
	if err != nil {
		t.Fatalf("content is not in the store: %v", err)
	}
	if !bytes.Equal(stored, content) {
		t.Error("stored content does not match what was hoisted")
	}

	// And the pack records what it no longer carries — the only remaining
	// statement of what that layer is.
	m, err := contribapi.ReadLayerManifest(folder)
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Layers["references/dataset.bin"]; got.Hash != moved[0].Ref.Hash || got.Size != moved[0].Ref.Size {
		t.Errorf("recorded %+v, want %+v", got, moved[0].Ref)
	}
}

// Two versions sharing unchanged content share one blob. Republishing a large
// file that did not change must transfer nothing.
func TestRepublishingUnchangedContentStoresItOnce(t *testing.T) {
	folder, _ := packWithALargeFile(t, 3<<20)
	blobs := t.TempDir()

	first, err := HoistBulkLayers(context.Background(), folder, blobs)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadDir(blobs)

	// A second version: something small changed, the dataset did not.
	if err := os.WriteFile(filepath.Join(folder, "skills", "usage.md"),
		[]byte("---\nname: usage\ndescription: How to, v2.\n---\n\nBody.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := HoistBulkLayers(context.Background(), folder, blobs)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadDir(blobs)

	if len(after) != len(before) {
		t.Errorf("republishing added %d blob(s); unchanged content should add none", len(after)-len(before))
	}
	if first[0].Ref.Hash != second[0].Ref.Hash || first[0].Ref.Size != second[0].Ref.Size {
		t.Errorf("unchanged content got a different descriptor: %+v then %+v", first[0].Ref, second[0].Ref)
	}
}

// Content that grows across the boundary is routed differently with nothing
// declared, flagged or migrated — which is the point of deriving transport from
// size rather than taking a producer's word for it.
func TestGrowingAcrossTheBoundaryNeedsNoMigration(t *testing.T) {
	folder := t.TempDir()
	if err := os.MkdirAll(filepath.Join(folder, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(folder, "references", "notes.txt")
	blobs := t.TempDir()

	// v1: comfortably inside the control band.
	if err := os.WriteFile(file, bulkContent(1024), 0o644); err != nil {
		t.Fatal(err)
	}
	moved, err := HoistBulkLayers(context.Background(), folder, blobs)
	if err != nil {
		t.Fatal(err)
	}
	if len(moved) != 0 {
		t.Fatalf("a %d-byte file was hoisted", 1024)
	}
	if _, err := os.Stat(filepath.Join(folder, contribapi.LayerManifestFile)); !os.IsNotExist(err) {
		t.Error("a pack with no bulk content carries a layer manifest")
	}

	// v2: the same file, past the boundary. Nothing was declared or converted.
	if err := os.WriteFile(file, bulkContent(3<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	moved, err = HoistBulkLayers(context.Background(), folder, blobs)
	if err != nil {
		t.Fatal(err)
	}
	if len(moved) != 1 {
		t.Fatalf("the grown file was not routed to the store: %v", moved)
	}
	if !moved[0].Ref.IsBulk() {
		t.Error("the grown file was not recognised as bulk")
	}
}
