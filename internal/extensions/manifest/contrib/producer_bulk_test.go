// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/api/providertest"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
)

// producer_bulk_test.go — the PRODUCER side of the bulk band.
//
// bulk_lane_test.go (manifest/internal/core) proves the consumer streams a
// bulk layer without holding it. This file proves the producer never held it
// in the first place: enumerating a folder that carries a bulk-band file
// materialises none of that file, opening ANOTHER entry touches none of it,
// and a mutable source remembers a bulk file's address by (path, size, mtime)
// rather than re-streaming it on every enumeration.

const producerBulkSize = 1 << 20 // 1 MiB — past BandControl, so bulk

// countingLoader wraps a filesystem loader and counts what reaches the
// content: bytes MATERIALISED through ReadFile and bytes STREAMED through
// OpenFile, per path, plus every enumeration of the source (List or Walk).
// The Streamer and ImmutableSource answers are the wrapped loader's own.
type countingLoader struct {
	*loader.FSLoader
	mu       sync.Mutex
	read     map[string]int64
	streamed map[string]int64
	walks    int
}

func newCountingLoader(root string) *countingLoader {
	return &countingLoader{FSLoader: loader.NewFSLoader(root), read: map[string]int64{}, streamed: map[string]int64{}}
}

func (c *countingLoader) ReadFile(name string) ([]byte, error) {
	b, err := c.FSLoader.ReadFile(name)
	c.mu.Lock()
	c.read[name] += int64(len(b))
	c.mu.Unlock()
	return b, err
}

func (c *countingLoader) List(pattern string) []string {
	c.mu.Lock()
	c.walks++
	c.mu.Unlock()
	return c.FSLoader.List(pattern)
}

func (c *countingLoader) Walk() []string {
	c.mu.Lock()
	c.walks++
	c.mu.Unlock()
	return c.FSLoader.Walk()
}

func (c *countingLoader) OpenFile(name string) (io.ReadCloser, error) {
	rc, err := c.FSLoader.OpenFile(name)
	if err != nil {
		return nil, err
	}
	return &countingReader{ReadCloser: rc, add: func(n int) {
		c.mu.Lock()
		c.streamed[name] += int64(n)
		c.mu.Unlock()
	}}, nil
}

func (c *countingLoader) snapshot(name string) (read, streamed int64, walks int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.read[name], c.streamed[name], c.walks
}

type countingReader struct {
	io.ReadCloser
	add func(int)
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	r.add(n)
	return n, err
}

func randomBytes(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return b
}

func writeFile(t *testing.T, root, rel string, data []byte) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func entryByID(entries []contribapi.Contribution, id string) (contribapi.Contribution, bool) {
	for _, c := range entries {
		if c.ID == id {
			return c, true
		}
	}
	return contribapi.Contribution{}, false
}

// TestProducer_BulkFileIsAddressedNotHeld: a mutable (directory) source
// carrying a 1 MiB file. One Enumerate materialises none of it and streams
// it exactly once, for its address; opening ANOTHER entry touches none of
// it; a second Enumerate re-streams none of it (the address is remembered
// by what the file was streamed at); a changed file is re-addressed.
func TestProducer_BulkFileIsAddressedNotHeld(t *testing.T) {
	root := t.TempDir()
	big := randomBytes(t, producerBulkSize)
	writeFile(t, root, "refs/big.bin", big)
	writeFile(t, root, "refs/small.md", []byte("# small\n"))
	cl := newCountingLoader(root)
	p := &EmbedFSResourceProvider{
		ProviderName: "ktw-test", ResourceType: "reference", Loader: cl, Root: "refs",
		KeepFolderStructure: true, KeepExtension: true,
	}
	ctx := context.Background()
	if p.Identity().Mutable != true {
		t.Fatal("a directory source is mutable; this test is about the mutable path")
	}

	entries, err := p.Enumerate(ctx)
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	bigEntry, ok := entryByID(entries, "big.bin")
	if !ok {
		t.Fatal("big.bin not enumerated")
	}
	src := bigEntry.Layers[contribapi.LayerSource]
	if !src.IsBulk() || src.Hash != contribapi.HashContent(big) || src.Inline != nil || src.Size != producerBulkSize {
		t.Fatalf("big.bin source layer = %+v, want bulk, addressed, not carried", src)
	}
	read, streamed, walks := cl.snapshot("refs/big.bin")
	t.Logf("after one Enumerate: bulk bytes materialised=%d streamed=%d walks=%d", read, streamed, walks)
	if read != 0 {
		t.Errorf("one Enumerate materialised %d bytes of the bulk file through ReadFile, want 0", read)
	}
	if streamed != producerBulkSize {
		t.Errorf("one Enumerate streamed %d bytes of the bulk file, want exactly its size once (its address)", streamed)
	}

	// Opening ANOTHER entry reads that entry, and nothing of the bulk file.
	rc, ref, err := p.Open(ctx, "small.md", contribapi.LayerSource)
	if err != nil {
		t.Fatalf("Open small.md: %v", err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(got) != "# small\n" || ref.Hash != contribapi.HashContent(got) {
		t.Errorf("Open small.md = %q ref %+v", got, ref)
	}
	read2, streamed2, walks2 := cl.snapshot("refs/big.bin")
	t.Logf("after one Open of ANOTHER entry: bulk bytes materialised=%d streamed=%d walks=%d", read2, streamed2, walks2)
	if read2 != 0 || streamed2 != streamed {
		t.Errorf("Open of another entry touched the bulk file: materialised %d, streamed %d (was %d)", read2, streamed2, streamed)
	}
	if walks2-walks > 1 {
		t.Errorf("Open of one entry enumerated the source %d times, want at most one stat-only walk", walks2-walks)
	}

	// A second Enumerate: the address is remembered by (path, size, mtime).
	if _, err := p.Enumerate(ctx); err != nil {
		t.Fatal(err)
	}
	read3, streamed3, _ := cl.snapshot("refs/big.bin")
	if read3 != 0 || streamed3 != streamed {
		t.Errorf("second Enumerate re-read the bulk file: materialised %d, streamed %d (was %d)", read3, streamed3, streamed)
	}

	// The file changes (size too, so the stamp differs whatever the clock's
	// resolution): the next Enumerate addresses the new content.
	big2 := randomBytes(t, producerBulkSize+1)
	writeFile(t, root, "refs/big.bin", big2)
	entries, err = p.Enumerate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	bigEntry, _ = entryByID(entries, "big.bin")
	if h := bigEntry.Layers[contribapi.LayerSource].Hash; h != contribapi.HashContent(big2) {
		t.Errorf("after a change the address is %s, want the new content's %s", h, contribapi.HashContent(big2))
	}
	read4, streamed4, _ := cl.snapshot("refs/big.bin")
	if read4 != 0 || streamed4 != streamed+producerBulkSize+1 {
		t.Errorf("re-addressing a changed bulk file: materialised %d, streamed %d, want 0 and %d", read4, streamed4, streamed+producerBulkSize+1)
	}

	providertest.Run(t, p)
}

// walkCounter counts enumerations of any loader and forwards its
// immutability, so an immutable embedded source stays immutable when wrapped.
type walkCounter struct {
	loader.Loader
	walks int
}

func (w *walkCounter) List(pattern string) []string { w.walks++; return w.Loader.List(pattern) }
func (w *walkCounter) Walk() []string               { w.walks++; return w.Loader.Walk() }
func (w *walkCounter) Immutable() bool              { return loader.IsImmutable(w.Loader) }

// TestProducer_ImmutableOpenIndexesTheMemo: an immutable source is walked
// once per process; every Open — hit or miss — answers from that one
// enumeration.
func TestProducer_ImmutableOpenIndexesTheMemo(t *testing.T) {
	w := &walkCounter{Loader: loader.NewEmbedFSLoader(makeFS(), ".").MarkImmutable()}
	p := &EmbedFSResourceProvider{
		ProviderName: "ktw-test", ResourceType: "skill", Loader: w, Root: "embed/skills",
		FileExtensions: []string{"md"},
	}
	ctx := context.Background()
	if _, err := p.Enumerate(ctx); err != nil {
		t.Fatal(err)
	}
	if p.Identity().Mutable {
		t.Fatal("the wrapped embedded source must stay immutable")
	}
	walks := w.walks
	if walks != 1 {
		t.Errorf("one Enumerate walked the source %d times, want 1", walks)
	}
	for _, id := range []string{"doc-create", "diagram", "category-nested", "doc-create"} {
		rc, _, err := p.Open(ctx, id, contribapi.LayerSource)
		if err != nil {
			t.Fatalf("Open %s: %v", id, err)
		}
		_ = rc.Close()
	}
	if _, _, err := p.Open(ctx, "no-such-skill", contribapi.LayerSource); !errors.As(err, new(contribapi.ErrContributionNotFound)) {
		t.Errorf("unknown id: %v, want ErrContributionNotFound", err)
	}
	if w.walks != walks {
		t.Errorf("five Opens walked the source %d more times, want 0: the memoised enumeration is the index", w.walks-walks)
	}
	providertest.Run(t, p)
}

// TestProducer_WalksAnyDepth: a file twelve folders down is published. The
// walk used to stop at eight and the file vanished with no Drop.
func TestProducer_WalksAnyDepth(t *testing.T) {
	deepDir := strings.Repeat("d/", 12)
	fsys := fstest.MapFS{
		"embed/references/top.md":                 {Data: []byte("top")},
		"embed/references/" + deepDir + "leaf.md": {Data: []byte("leaf")},
	}
	p := &EmbedFSResourceProvider{
		ProviderName: "ktw-test", ResourceType: "reference", Loader: loader.NewEmbedFSLoader(fsys, "."),
		Root: "embed/references", KeepFolderStructure: true,
	}
	entries, err := p.Enumerate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, c := range entries {
		ids = append(ids, c.ID)
	}
	if _, ok := entryByID(entries, deepDir+"leaf"); !ok {
		t.Errorf("a file at depth 13 under the root is not enumerated; got %v", ids)
	}
	if _, ok := entryByID(entries, "top"); !ok {
		t.Errorf("top.md not enumerated; got %v", ids)
	}
	if len(entries) != 2 {
		t.Errorf("entries = %d, want 2: %v", len(entries), ids)
	}
}
