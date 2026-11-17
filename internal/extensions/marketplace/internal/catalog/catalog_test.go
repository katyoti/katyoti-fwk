// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/signing"
)

// TestFetch_PinnedTrust exercises the install-time provenance gate: a pinned
// marketplace only trusts a catalog whose `.minisig` verifies against the
// pinned key; tampering, the wrong key, or a missing key/signature all refuse.
// A `trust: none` marketplace skips verification entirely.
func TestFetch_PinnedTrust(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir()) // isolate the catalog cache
	ctx := context.Background()
	dir := t.TempDir()

	pk, sk, err := signing.Generate()
	if err != nil {
		t.Fatal(err)
	}
	index := []byte("apiVersion: 1\nmarketplace: {name: team}\nitems:\n  - {name: x, kind: sidecar}\n")
	idx := filepath.Join(dir, "katyoti-marketplace.yaml")
	if err := os.WriteFile(idx, index, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(idx+".minisig", []byte(signing.Sign(index, sk, "v1")), 0o644); err != nil {
		t.Fatal(err)
	}
	url := "file://" + filepath.ToSlash(idx)
	pinned := func(pubKey string) mktapi.Marketplace {
		return mktapi.Marketplace{Name: "team", URL: url, Enabled: true, Trust: mktapi.TrustPinned, PubKey: pubKey}
	}

	// 1. valid pinned signature → trusted.
	cat, err := Fetch(ctx, pinned(signing.PublicKeyLine(pk)))
	if err != nil {
		t.Fatalf("valid pinned catalog should fetch: %v", err)
	}
	if len(cat.Items) != 1 || cat.Items[0].Name != "x" {
		t.Fatalf("unexpected catalog: %+v", cat.Items)
	}

	// 2. pinned but no key → refuse (fail closed).
	if _, err := Fetch(ctx, pinned("")); err == nil {
		t.Fatal("pinned with no key must refuse")
	}

	// 3. wrong key → refuse.
	other, _, _ := signing.Generate()
	if _, err := Fetch(ctx, pinned(signing.PublicKeyLine(other))); err == nil {
		t.Fatal("pinned with the wrong key must refuse")
	}

	// 4. tampered index → refuse (signature no longer matches).
	if err := os.WriteFile(idx, append(append([]byte(nil), index...), '!'), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Fetch(ctx, pinned(signing.PublicKeyLine(pk))); err == nil {
		t.Fatal("tampered index must refuse")
	}
	_ = os.WriteFile(idx, index, 0o644) // restore

	// 5. trust: none → verification skipped even with the signature removed.
	_ = os.Remove(idx + ".minisig")
	if _, err := Fetch(ctx, mktapi.Marketplace{Name: "local", URL: url, Enabled: true, Trust: mktapi.TrustNone}); err != nil {
		t.Fatalf("trust=none should skip verification: %v", err)
	}
}
