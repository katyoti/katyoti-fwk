// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
)

// wideFixture: n small skills across one provider, the manifest hashed with
// `hash`, in a memory view the service reads.
func wideFixture(n int, hash string) (*catalogServiceImpl, *manifestlocal.MemoryCatalog) {
	var cs []contribapi.Contribution
	for i := 0; i < n; i++ {
		c := contribapi.Contribution{Provider: "ktw-docs", Type: "skill", ID: fmt.Sprintf("s%04d", i)}
		c.SetData(json.RawMessage(`{"name":"s"}`))
		cs = append(cs, c)
	}
	cat := manifestlocal.NewMemoryCatalog([]*mbapi.Manifest{{Provider: "ktw-docs", Hash: hash, Contributions: cs}}, nil)
	return &catalogServiceImpl{cat: cat}, cat
}

// --- M5: every reply is one page ---

// Limit == 0 used to mean "everything in one reply" — the one path whose
// size grew with the universe. Now it is the default page, and the envelope
// says there is more.
func TestListContributions_LimitZeroIsOnePageNotTheUniverse(t *testing.T) {
	ctx := context.Background()
	s, _ := wideFixture(1200, "sha256:one")
	first, err := s.ListContributions(ctx, mbapi.CatalogListContributionsInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Contributions) != catalogListDefaultLimit || !first.Pagination.HasMore || first.Pagination.NextCursor == "" || first.Pagination.Total != 1200 {
		t.Fatalf("Limit 0 answered %d entries, pagination %+v; want %d and a next cursor", len(first.Contributions), first.Pagination, catalogListDefaultLimit)
	}
	// Following the cursor drains the universe in three replies.
	got, calls, cursor := 0, 0, ""
	for {
		out, err := s.ListContributions(ctx, mbapi.CatalogListContributionsInput{Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		calls++
		got += len(out.Contributions)
		if !out.Pagination.HasMore {
			break
		}
		cursor = out.Pagination.NextCursor
	}
	if got != 1200 || calls != 3 {
		t.Fatalf("drained %d entries in %d calls, want 1200 in 3", got, calls)
	}
	// The compatibility method is bounded the same way.
	compat, err := s.ListResources(ctx, mbapi.CatalogListResourcesInput{})
	if err != nil || len(compat.Contributions) != catalogListDefaultLimit || !compat.Pagination.HasMore {
		t.Fatalf("ListResources unpaged answered %d entries (%v), want one page", len(compat.Contributions), err)
	}
}

// GetManifest with SlotsOnly answers the callable lane and nothing else: the
// consumer that already holds the paged index must not receive every entry a
// second time, unpaged.
func TestGetManifest_SlotsOnlyCarriesNoContributions(t *testing.T) {
	ctx := context.Background()
	s, _ := wideFixture(3, "sha256:m")
	full, err := s.GetManifest(ctx, mbapi.GetManifestInput{Provider: "ktw-docs"})
	if err != nil || full.Manifest == nil || len(full.Manifest.Contributions) != 3 {
		t.Fatalf("full manifest: %d entries, %v", len(full.Manifest.Contributions), err)
	}
	slots, err := s.GetManifest(ctx, mbapi.GetManifestInput{Provider: "ktw-docs", SlotsOnly: true})
	if err != nil || slots.Manifest == nil {
		t.Fatal(err)
	}
	if len(slots.Manifest.Contributions) != 0 || slots.Manifest.Hash != "sha256:m" || slots.Manifest.Provider != "ktw-docs" {
		t.Fatalf("slots-only manifest = %d entries, hash %q, provider %q; want none, sha256:m, ktw-docs", len(slots.Manifest.Contributions), slots.Manifest.Hash, slots.Manifest.Provider)
	}
	// The catalog's own copy is untouched (the reply was a copy).
	again, _ := s.GetManifest(ctx, mbapi.GetManifestInput{Provider: "ktw-docs"})
	if len(again.Manifest.Contributions) != 3 {
		t.Fatal("SlotsOnly emptied the catalog's own manifest")
	}
}

// --- L4: a cursor survives a reload by restarting, not by skipping ---

func TestListContributions_CursorRestartsWhenTheIndexChanged(t *testing.T) {
	ctx := context.Background()
	s, cat := wideFixture(5, "sha256:v1")
	page1, err := s.ListContributions(ctx, mbapi.CatalogListContributionsInput{Limit: 2})
	if err != nil || len(page1.Contributions) != 2 || !page1.Pagination.HasMore {
		t.Fatalf("page 1: %+v %v", page1.Pagination, err)
	}
	cursor := page1.Pagination.NextCursor
	if !strings.Contains(cursor, cursorSep) {
		t.Fatalf("cursor %q carries no index digest", cursor)
	}

	// Unchanged index: the cursor continues where it left.
	page2, err := s.ListContributions(ctx, mbapi.CatalogListContributionsInput{Limit: 2, Cursor: cursor})
	if err != nil || page2.Pagination.Offset != 2 || page2.Contributions[0].ID != "s0002" {
		t.Fatalf("page 2 on the same index: offset %d first %q (%v), want 2 / s0002", page2.Pagination.Offset, page2.Contributions[0].ID, err)
	}

	// The index reloads between two pages (a member re-probed, two entries
	// gone): the old cursor's offsets count entries that no longer exist,
	// so the page restarts at 0 instead of skipping or repeating.
	_, fresh := wideFixture(3, "sha256:v2")
	freshList, _ := fresh.List(ctx)
	cat.Replace(freshList, nil)
	restarted, err := s.ListContributions(ctx, mbapi.CatalogListContributionsInput{Limit: 2, Cursor: cursor})
	if err != nil || restarted.Pagination.Offset != 0 || restarted.Contributions[0].ID != "s0000" || restarted.Pagination.Total != 3 {
		t.Fatalf("page after a reload: offset %d first %q total %d (%v); want a restart at 0 over the 3 new entries",
			restarted.Pagination.Offset, restarted.Contributions[0].ID, restarted.Pagination.Total, err)
	}

	// A bare-offset cursor from an older client is honoured as before.
	legacy, err := s.ListContributions(ctx, mbapi.CatalogListContributionsInput{Limit: 2, Cursor: "2"})
	if err != nil || legacy.Pagination.Offset != 2 {
		t.Fatalf("legacy cursor: offset %d (%v), want 2", legacy.Pagination.Offset, err)
	}
	if _, err := s.ListContributions(ctx, mbapi.CatalogListContributionsInput{Cursor: "x@y"}); err == nil {
		t.Fatal("a malformed cursor was accepted")
	}
}

// --- M6: the hub verifies the hash it is given ---

func TestGetBody_RefusesAHashTheBodyDoesNotMatch(t *testing.T) {
	ctx := context.Background()
	s, _ := wideFixture(1, "sha256:h")
	uri := "ktw-docs:/skill/s0000"
	body := json.RawMessage(`{"name":"s"}`)
	right, err := s.GetBody(ctx, mbapi.GetBodyInput{URI: uri, Hash: contribapi.HashContent(body)})
	if err != nil || string(right.Content) != string(body) {
		t.Fatalf("right hash: %s %v", right.Content, err)
	}
	_, err = s.GetBody(ctx, mbapi.GetBodyInput{URI: uri, Hash: "sha256:0000"})
	if err == nil || !strings.Contains(err.Error(), "does not match its address") {
		t.Fatalf("wrong hash: err = %v, want a content mismatch", err)
	}
}

// --- L7: an absent member binary is stamped, its entries still served ---

func TestListNames_StampsAMemberWhoseBinaryIsAbsent(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	cache := manifestlocal.NewCache(filepath.Join(dir, "cache"))
	var members []manifestlocal.Member
	for _, name := range []string{"ktw-here", "ktw-gone"} {
		exe := filepath.Join(dir, "bin", name+".exe")
		_ = os.MkdirAll(filepath.Dir(exe), 0o755)
		if err := os.WriteFile(exe, []byte{0}, 0o644); err != nil {
			t.Fatal(err)
		}
		m := &mbapi.Manifest{Provider: name, Version: "1", Hash: "sha256:" + name,
			Contributions: []contribapi.Contribution{{Provider: name, Type: "skill", ID: "demo"}}}
		if err := cache.Save(name, m, exe, manifestlocal.SourceDigests{}); err != nil {
			t.Fatal(err)
		}
		members = append(members, manifestlocal.Member{Name: name, Binary: exe})
	}
	if err := os.Remove(members[1].Binary); err != nil {
		t.Fatal(err)
	}
	cat, err := manifestlocal.NewCatalog(cache, members)
	if err != nil {
		t.Fatal(err)
	}
	s := &catalogServiceImpl{cat: cat}
	out, err := s.ListNames(ctx, mbapi.ListNamesInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Names) != 2 || len(out.Unavailable) != 1 || out.Unavailable[0] != "ktw-gone" {
		t.Fatalf("names %v unavailable %v; want both names and ktw-gone stamped", out.Names, out.Unavailable)
	}
	// Its entries are still served from the record.
	list, err := s.ListContributions(ctx, mbapi.CatalogListContributionsInput{Filter: contribapi.ListFilter{Providers: []string{"ktw-gone"}}})
	if err != nil || len(list.Contributions) != 1 {
		t.Fatalf("the absent member's entries: %d (%v), want 1 from the record", len(list.Contributions), err)
	}
	// A memory view declares no binaries and stamps nothing.
	mem, _ := wideFixture(1, "sha256:m")
	memOut, _ := mem.ListNames(ctx, mbapi.ListNamesInput{})
	if memOut.Unavailable != nil {
		t.Fatalf("memory view stamped %v", memOut.Unavailable)
	}
}

// --- M4 at the wire: one URI two members carry lists once ---

func TestListContributions_OneURICarriedTwiceListsOnce(t *testing.T) {
	shared := contribapi.Contribution{Provider: "shared-pack", Type: "recipe", ID: "twice"}
	cat := manifestlocal.NewMemoryCatalog([]*mbapi.Manifest{
		{Provider: "ktw-b", Hash: "sha256:b", Contributions: []contribapi.Contribution{shared}},
		{Provider: "ktw-a", Hash: "sha256:a", Contributions: []contribapi.Contribution{shared}},
	}, nil)
	s := &catalogServiceImpl{cat: cat}
	out, err := s.ListContributions(context.Background(), mbapi.CatalogListContributionsInput{})
	if err != nil || len(out.Contributions) != 1 || out.Pagination.Total != 1 {
		t.Fatalf("listed %d entries (total %d, %v) for one URI carried by two members, want 1", len(out.Contributions), out.Pagination.Total, err)
	}
	uris, _ := s.ListURIs(context.Background(), mbapi.CatalogListURIsInput{})
	if len(uris.URIs) != 1 {
		t.Fatalf("ListURIs listed %d, want 1", len(uris.URIs))
	}
}
