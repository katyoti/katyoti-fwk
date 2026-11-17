// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// --- ST1: BodyHash stability + emptiness ---

func TestHashBody_StableAndEmpty(t *testing.T) {
	if got := HashBody(nil); got != "" {
		t.Errorf("HashBody(nil) = %q, want empty", got)
	}
	if got := HashBody([]byte{}); got != "" {
		t.Errorf("HashBody(empty) = %q, want empty", got)
	}
	body := []byte(`{"name":"x","instructions":"do the thing"}`)
	want := "sha256:" + hexSum(body)
	if got := HashBody(body); got != want {
		t.Errorf("HashBody = %q, want %q", got, want)
	}
	// Stable across calls.
	first, second := HashBody(body), HashBody(body)
	if first != second {
		t.Error("HashBody not deterministic")
	}
}

func hexSum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func TestBuilder_SetsBodyHash(t *testing.T) {
	body := []byte(`{"name":"alpha"}`)
	b := NewBuilder("ktw-x", "test")
	m, err := b.BuildFromProviders(context.Background(), []contribapi.ContributionProvider{
		staticProvider{provider: "ktw-x", typ: "skill", entries: []contribapi.Contribution{
			{Provider: "ktw-x", Type: "skill", ID: "alpha", Layers: contribapi.InlineData(body)},
			{Provider: "ktw-x", Type: "skill", ID: "empty"}, // no body
		}},
		staticProvider{provider: "ktw-x", typ: "contribution-type", entries: []contribapi.Contribution{
			{Provider: "ktw-x", Type: "contribution-type", ID: "skill",
				Layers: contribapi.InlineData(json.RawMessage(`{"name":"skill","schema":"ktw-x:/data-type/skill"}`))},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]contribapi.Contribution{}
	for _, c := range m.Contributions {
		byID[c.ID] = c
	}
	if got := byID["alpha"].DataHash(); got != "sha256:"+hexSum(body) {
		t.Errorf("alpha BodyHash = %q", got)
	}
	if got := byID["empty"].DataHash(); got != "" {
		t.Errorf("empty BodyHash = %q, want empty", got)
	}
}

// --- ST2: content store: dedup, hit/miss, hash verification ---

func TestContentStore_PutGetDedup(t *testing.T) {
	store := NewContentStore(t.TempDir())
	bodyA := []byte(`{"k":"a"}`)
	bodyB := []byte(`{"k":"b"}`)
	hashA := HashBody(bodyA)
	hashB := HashBody(bodyB)

	if _, ok := store.Get(hashA); ok {
		t.Error("expected miss before Put")
	}
	if err := store.Put(hashA, bodyA); err != nil {
		t.Fatal(err)
	}
	got, ok := store.Get(hashA)
	if !ok || string(got) != string(bodyA) {
		t.Errorf("Get(hashA) = %q, %v", got, ok)
	}
	// Idempotent re-put of same hash is a no-op (dedup).
	if err := store.Put(hashA, bodyA); err != nil {
		t.Errorf("re-put same body: %v", err)
	}
	if err := store.Put(hashB, bodyB); err != nil {
		t.Fatal(err)
	}
	if !store.Has(hashA) || !store.Has(hashB) {
		t.Error("expected both present")
	}
	// Cross-provider dedup: identical body from a different logical
	// source addresses to the same file (one Put, one file).
	if err := store.Put(hashA, bodyA); err != nil {
		t.Fatal(err)
	}
}

func TestContentStore_PutRejectsHashMismatch(t *testing.T) {
	store := NewContentStore(t.TempDir())
	if err := store.Put("sha256:deadbeef", []byte("real content")); err == nil {
		t.Error("expected hash-mismatch error")
	}
}

// --- ST3 + cache-hit/miss: GetBody lazy resolution ---

func TestMemoryCatalog_GetBody_ContentStoreHitNoFetch(t *testing.T) {
	body := []byte(`{"name":"sk","instructions":"L1"}`)
	hash := HashBody(body)

	// Body-free index entry (Body=nil, BodyHash set).
	idx := &mbapi.Manifest{Provider: "ktw-x", Contributions: []contribapi.Contribution{
		{Provider: "ktw-x", Type: "skill", ID: "sk", Layers: contribapi.DataDescriptor(hash, 0)},
	}}
	cat := NewMemoryCatalog([]*mbapi.Manifest{idx}, nil)

	store := NewContentStore(t.TempDir())
	if err := store.Put(hash, body); err != nil {
		t.Fatal(err)
	}
	var fetches int32
	cat.SetContentLane(store, func(_ context.Context, _, _ string) ([]byte, error) {
		atomic.AddInt32(&fetches, 1)
		return nil, fmt.Errorf("should not fetch on a store hit")
	})

	got, err := cat.GetBody(context.Background(), "ktw-x:/skill/sk")
	if err != nil || string(got) != string(body) {
		t.Fatalf("GetBody = %q, %v", got, err)
	}
	if n := atomic.LoadInt32(&fetches); n != 0 {
		t.Errorf("fetch count = %d, want 0 (content-store hit)", n)
	}
}

func TestMemoryCatalog_GetBody_StoreMissFetchesAndStores(t *testing.T) {
	body := []byte(`{"name":"sk","instructions":"L1"}`)
	hash := HashBody(body)
	idx := &mbapi.Manifest{Provider: "ktw-x", Contributions: []contribapi.Contribution{
		{Provider: "ktw-x", Type: "skill", ID: "sk", Layers: contribapi.DataDescriptor(hash, 0)},
	}}
	cat := NewMemoryCatalog([]*mbapi.Manifest{idx}, nil)

	store := NewContentStore(t.TempDir())
	var fetches int32
	cat.SetContentLane(store, func(_ context.Context, gotHash, gotURI string) ([]byte, error) {
		atomic.AddInt32(&fetches, 1)
		if gotHash != hash {
			t.Errorf("fetch hash = %q, want %q", gotHash, hash)
		}
		if gotURI != "ktw-x:/skill/sk" {
			t.Errorf("fetch uri = %q", gotURI)
		}
		return body, nil
	})

	// First call → miss → fetch + store.
	got, err := cat.GetBody(context.Background(), "ktw-x:/skill/sk")
	if err != nil || string(got) != string(body) {
		t.Fatalf("GetBody#1 = %q, %v", got, err)
	}
	if n := atomic.LoadInt32(&fetches); n != 1 {
		t.Fatalf("fetch count after miss = %d, want 1", n)
	}
	if !store.Has(hash) {
		t.Error("expected body stored after fetch")
	}
	// Second call → now a store hit → no further fetch.
	if _, err := cat.GetBody(context.Background(), "ktw-x:/skill/sk"); err != nil {
		t.Fatal(err)
	}
	if n := atomic.LoadInt32(&fetches); n != 1 {
		t.Errorf("fetch count after second call = %d, want 1 (cached)", n)
	}
}

// The content lane's miss path verifies the address before it stores or
// serves (e25, N8). The producer hashed exactly the bytes it carries, and
// every hop moves them verbatim (LayerRef.Inline and GetBodyOutput.Content
// are []byte), so a body that does not hash to the index's address is a
// corrupted or wrong answer: refused typed, never stored — a content-addressed
// store keyed by an address its bytes do not have serves the wrong bytes to
// every later reader. The body that does match is stored and served from the
// store thereafter.
func TestMemoryCatalog_GetBody_RefusesABodyThatDoesNotMatchItsAddress(t *testing.T) {
	good := []byte(`{"name":"sk","instructions":"L1"}`)
	indexHash := HashBody(good)
	var reindented []byte
	{
		var v any
		_ = json.Unmarshal(good, &v)
		reindented, _ = json.MarshalIndent(v, "", "  ")
	}

	idx := &mbapi.Manifest{Provider: "ktw-x", Contributions: []contribapi.Contribution{
		{Provider: "ktw-x", Type: "skill", ID: "sk", Layers: contribapi.DataDescriptor(indexHash, int64(len(good)))},
	}}
	cat := NewMemoryCatalog([]*mbapi.Manifest{idx}, nil)
	store := NewContentStore(t.TempDir())
	var fetches int32
	serve := reindented // a corrupted answer first: same JSON value, other bytes
	cat.SetContentLane(store, func(context.Context, string, string) ([]byte, error) {
		atomic.AddInt32(&fetches, 1)
		return serve, nil
	})

	_, err := cat.GetBody(context.Background(), "ktw-x:/skill/sk")
	var mismatch contribapi.ErrContentMismatch
	if !errors.As(err, &mismatch) || mismatch.Want != indexHash {
		t.Fatalf("corrupted body: err = %v, want contribapi.ErrContentMismatch against %s", err, indexHash)
	}
	if store.Has(indexHash) {
		t.Fatal("a body that does not match its address was stored under it")
	}
	// Through the layer lane too: the same bytes, the same refusal.
	if _, _, err := cat.OpenLayer(context.Background(), "ktw-x:/skill/sk", contribapi.LayerData); !errors.As(err, &mismatch) {
		t.Fatalf("OpenLayer(data) on a corrupted body: err = %v, want contribapi.ErrContentMismatch", err)
	}

	serve = good
	got, err := cat.GetBody(context.Background(), "ktw-x:/skill/sk")
	if err != nil || string(got) != string(good) {
		t.Fatalf("GetBody with the right bytes = %q, %v", got, err)
	}
	if !store.Has(indexHash) {
		t.Fatal("a verified body was not stored under its address")
	}
	n := atomic.LoadInt32(&fetches)
	if _, err := cat.GetBody(context.Background(), "ktw-x:/skill/sk"); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&fetches) != n {
		t.Error("a stored body was fetched again")
	}
}

func TestMemoryCatalog_GetBody_InlineBodyServedDirectly(t *testing.T) {
	body := []byte(`{"name":"local"}`)
	// Full-body manifest (the local self-manifest case).
	m := &mbapi.Manifest{Provider: "local", Contributions: []contribapi.Contribution{
		{Provider: "local", Type: "skill", ID: "x", Layers: contribapi.InlineData(body)},
	}}
	cat := NewMemoryCatalog([]*mbapi.Manifest{m}, nil)
	// No content lane wired — inline body must still resolve.
	got, err := cat.GetBody(context.Background(), "local:/skill/x")
	if err != nil || string(got) != string(body) {
		t.Fatalf("GetBody(inline) = %q, %v", got, err)
	}
}

func TestMemoryCatalog_GetBody_EmptyBodyContribution(t *testing.T) {
	m := &mbapi.Manifest{Provider: "ktw-x", Contributions: []contribapi.Contribution{
		{Provider: "ktw-x", Type: "config", ID: "c"}, // no body, no hash
	}}
	cat := NewMemoryCatalog([]*mbapi.Manifest{m}, nil)
	got, err := cat.GetBody(context.Background(), "ktw-x:/config/c")
	if err != nil {
		t.Fatalf("GetBody(empty) err = %v", err)
	}
	if got != nil {
		t.Errorf("GetBody(empty) = %q, want nil", got)
	}
}

// --- QueryTyped convergence (skills/tasks/personas path) ---

func TestQueryTyped_ResolvesBodyViaContentLane(t *testing.T) {
	type skillBody struct {
		Name         string `json:"name"`
		Instructions string `json:"instructions"`
	}
	body := mustJSONBytes(t, skillBody{Name: "doc-create", Instructions: "the L1 instructions"})
	hash := HashBody(body)
	idx := &mbapi.Manifest{Provider: "ktw-docs", Contributions: []contribapi.Contribution{
		{Provider: "ktw-docs", Type: "skill", ID: "doc-create", Layers: contribapi.DataDescriptor(hash, 0)},
	}}
	cat := NewMemoryCatalog([]*mbapi.Manifest{idx}, nil)
	store := NewContentStore(t.TempDir())
	_ = store.Put(hash, body)
	cat.SetContentLane(store, func(context.Context, string, string) ([]byte, error) {
		return nil, fmt.Errorf("unexpected fetch")
	})

	decoded, err := mbapi.QueryTyped[skillBody](context.Background(),
		mbapi.Q{Reach: mbapi.ReachUniverse, Kind: "skill", Catalog: cat})
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 1 {
		t.Fatalf("decoded len = %d", len(decoded))
	}
	if decoded[0].Provider != "ktw-docs" || decoded[0].Body.Instructions != "the L1 instructions" {
		t.Errorf("decoded = %+v", decoded[0])
	}
}

// --- ST6(a): NATS-safety probe ---

// catalogListResourcesNATSLimit mirrors the embedded NATS MaxPayload
// default (1 MiB) that fetchManifestsFromHub's paged body-free index
// must stay under.
const catalogListResourcesNATSLimit = 1 << 20

func TestTwoPlane_NATSSafety_LargePackIndexStaysSmall(t *testing.T) {
	// Synthetic large pack: 2000 contributions, each with a multi-KB
	// body. The FULL manifest (bodies inline) blows past 1 MiB; the
	// body-free INDEX page must stay well under it.
	const n = 2000
	bigBody := make([]byte, 4096)
	for i := range bigBody {
		bigBody[i] = 'x'
	}
	entries := make([]contribapi.Contribution, 0, n)
	for i := 0; i < n; i++ {
		body := fmt.Appendf(nil, `{"i":%d,"pad":%q}`, i, string(bigBody))
		entries = append(entries, contribapi.Contribution{
			Provider: "ktw-big",
			Type:     "skill",
			ID:       fmt.Sprintf("skill-%04d", i),
			Layers:   contribapi.InlineData(body),
		})
	}
	full := &mbapi.Manifest{Provider: "ktw-big", Contributions: entries}

	// Sanity: the full manifest is a NATS size bomb.
	fullJSON, _ := json.Marshal(full)
	if len(fullJSON) < catalogListResourcesNATSLimit {
		t.Fatalf("test pack too small to be meaningful: full manifest %d bytes", len(fullJSON))
	}

	// Body-free index page (what the boot path / GetManifest ships):
	// strip bodies, keep BodyHash. One page of 500 entries must fit.
	const pageSize = 500
	page := make([]contribapi.Contribution, 0, pageSize)
	for _, e := range entries[:pageSize] {
		e.DropInline()
		page = append(page, e)
	}
	pageJSON, _ := json.Marshal(mbapi.CatalogListResourcesOutput{
		Contributions: page,
		Pagination:    mbapi.Pagination{Total: n, PageSize: pageSize, HasMore: true, NextCursor: "500"},
	})
	if len(pageJSON) >= catalogListResourcesNATSLimit {
		t.Errorf("body-free index page = %d bytes, must be < NATS limit %d", len(pageJSON), catalogListResourcesNATSLimit)
	}
	t.Logf("full manifest=%d bytes (over limit), body-free index page(500)=%d bytes (under %d)",
		len(fullJSON), len(pageJSON), catalogListResourcesNATSLimit)

	// And the WHOLE set, fully stripped, in one shot: still must page —
	// assert all-stripped fits comfortably (no body) to prove the index
	// plane scales.
	allStripped := make([]contribapi.Contribution, 0, n)
	for _, e := range entries {
		e.DropInline()
		allStripped = append(allStripped, e)
	}
	allJSON, _ := json.Marshal(allStripped)
	t.Logf("all %d body-free entries = %d bytes", n, len(allJSON))
	// Each body-free entry must be tiny; verify the per-entry budget.
	perEntry := len(allJSON) / n
	if perEntry > 256 {
		t.Errorf("body-free per-entry size = %d bytes, expected small (URI + hash only)", perEntry)
	}
}

// --- ST3 + ST5: hub-side CatalogImpl.GetBody reads its own full-body
// manifest; the consumer GetManifest path is body-free (covered via the
// strip semantics the bus layer uses). ---

func TestCatalogImpl_GetBody_ReadsLoadedManifest(t *testing.T) {
	dir := t.TempDir()
	cache := NewCache(dir)
	body := []byte(`{"name":"doc-create","instructions":"L1"}`)
	docs := makeManifest("ktw-docs", "1.0.0", "")
	docs.Contributions = []contribapi.Contribution{
		{Provider: "ktw-docs", Type: "skill", ID: "doc-create", Layers: contribapi.InlineData(body)},
	}
	if err := SeedCacheSave(t, cache, docs); err != nil {
		t.Fatalf("Save: %v", err)
	}
	c, err := NewCatalog(cache, MembersOf(cache, "ktw-docs"))
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	got, err := c.GetBody(context.Background(), "ktw-docs:/skill/doc-create")
	if err != nil {
		t.Fatalf("GetBody err = %v", err)
	}
	// The cache re-indents JSON on round-trip; compare structurally.
	var gotMap, wantMap map[string]any
	_ = json.Unmarshal(got, &gotMap)
	_ = json.Unmarshal(body, &wantMap)
	if fmt.Sprint(gotMap) != fmt.Sprint(wantMap) {
		t.Fatalf("GetBody = %v, want %v", gotMap, wantMap)
	}
	if _, err := c.GetBody(context.Background(), "ktw-docs:/skill/missing"); err == nil {
		t.Error("GetBody(missing) should error")
	}
}

func mustJSONBytes(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// staticProvider is a minimal ContributionProvider returning a fixed
// entry set — for builder tests.
type staticProvider struct {
	provider string
	typ      string
	entries  []contribapi.Contribution
}

func (p staticProvider) Provider() string { return p.provider }
func (p staticProvider) Type() string     { return p.typ }
func (p staticProvider) Enumerate(context.Context) ([]contribapi.Contribution, error) {
	return p.entries, nil
}
func (p staticProvider) Identity() contribapi.Identity { return contribapi.Identity{} }
func (p staticProvider) Open(ctx context.Context, id, layer string) (io.ReadCloser, contribapi.LayerRef, error) {
	return contribapi.OpenEnumerated(ctx, p, id, layer)
}
