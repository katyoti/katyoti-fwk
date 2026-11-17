// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/authoring"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/index"
)

// A SIGNED CANDIDATE REACHES CANARY ONLY, ITS PROMOTION STABLE
// - through Fetch, as every list/install/update reads a marketplace: a
// candidate marked canary and then promoted, the catalog signed again each
// time, still verifies against the pinned key; a stable installation sees the
// official version only, a canary one the candidate too.
func TestASignedCandidateReachesCanaryOnlyAndItsPromotionStable(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir()) // isolate the catalog cache
	ctx := context.Background()
	dir := t.TempDir()
	svc := authoring.New()
	pubFile, secFile, _, err := svc.Keygen("test key")
	if err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(t.TempDir(), "team.key")
	if err := os.WriteFile(key, []byte(secFile), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := index.MarshalCatalog(mktapi.Catalog{APIVersion: 1, Marketplace: mktapi.CatalogMeta{Name: "team"}, Items: []mktapi.Item{
		{Name: "ktw-agents", Kind: mktapi.KindSidecar, Versions: []mktapi.ItemVersion{{Version: "2026.10-rc1"}, {Version: "2026.09"}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "katyoti-marketplace.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	url := "file://" + filepath.ToSlash(filepath.Join(dir, "katyoti-marketplace.yaml"))
	following := func(channel string) []string {
		t.Helper()
		cat, err := Fetch(ctx, mktapi.Marketplace{Name: "team", URL: url, Enabled: true, Trust: mktapi.TrustPinned, PubKey: pubFile, Channel: channel})
		if err != nil {
			t.Fatalf("%s: %v", channel, err)
		}
		var vs []string
		for _, it := range cat.Items {
			for _, v := range it.Versions {
				vs = append(vs, v.Version)
			}
		}
		return vs
	}

	if _, err := svc.MarkChannel(dir, "2026.10-rc1", mktapi.ChannelCanary, key); err != nil {
		t.Fatal(err)
	}
	if got := following(""); len(got) != 1 || got[0] != "2026.09" {
		t.Errorf("stable sees %v after the candidate; want 2026.09 only", got)
	}
	if got := following(mktapi.ChannelCanary); len(got) != 2 || got[0] != "2026.10-rc1" {
		t.Errorf("canary sees %v; want the candidate first, then 2026.09", got)
	}
	if _, err := svc.Promote(dir, "2026.10-rc1", "2026.10", key); err != nil {
		t.Fatal(err)
	}
	if got := following(""); len(got) != 2 || got[0] != "2026.10" {
		t.Errorf("stable sees %v after the promotion; want 2026.10 first", got)
	}
}
