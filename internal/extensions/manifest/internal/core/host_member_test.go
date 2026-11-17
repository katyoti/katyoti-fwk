// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// A member may publish under another provider name than the one the universe
// declares it by — a ktw-host adopting its folder's name.
// The hub acquires a connection by MEMBER name, so the host of each of its URIs
// is the member: routing a layer read by the provider asked for a connection
// no member has, and every non-inline layer of such a member was unreachable.
func TestHostOf_IsTheMemberThatCarriesTheURI_NotItsProvider(t *testing.T) {
	cache := NewCache(t.TempDir())
	m := makeManifest("acme", "1.0.0", "sha256:acme") // the manifest's provider
	entry := contribapi.Contribution{Provider: "acme", Type: "skill", ID: "greet"}
	entry.SetData([]byte(`{"name":"greet"}`))
	m.Contributions = []contribapi.Contribution{entry}

	// Saved, and declared, under its member name.
	exe := testExePath(cache, "member-a")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte{0}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cache.Save("member-a", m, exe, SourceDigests{}); err != nil {
		t.Fatal(err)
	}
	cat, err := NewCatalog(cache, MembersOf(cache, "member-a"))
	if err != nil {
		t.Fatal(err)
	}

	uri := contribapi.ResourceURI{Provider: "acme", Type: "skill", ID: "greet"}
	if got := cat.HostOf(uri); got != "member-a" {
		t.Fatalf("HostOf(%s) = %q, want the member that carries it (member-a), the name the hub connects by", uri, got)
	}
	if body, err := cat.GetBody(context.Background(), uri.String()); err != nil || string(body) != `{"name":"greet"}` {
		t.Fatalf("GetBody(%s) = %q, %v", uri, body, err)
	}
	// An unknown entry of a provider a member publishes is not found — not
	// "not in the catalog", which says no member publishes the provider.
	if _, err := cat.GetBody(context.Background(), "acme:/skill/none"); err == nil || errors.Is(err, mbapi.ErrSidecarNotInCatalog) {
		t.Fatalf("an unknown entry of a published provider: err = %v, want not found, not \"not in the catalog\"", err)
	}
	if _, err := cat.GetBody(context.Background(), "nobody:/skill/greet"); !errors.Is(err, mbapi.ErrSidecarNotInCatalog) {
		t.Fatalf("a provider no member publishes: err = %v, want not in the catalog", err)
	}
}
