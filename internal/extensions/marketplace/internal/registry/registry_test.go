// SPDX-License-Identifier: Apache-2.0

package registry

import (
	"os"
	"path/filepath"
	"testing"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func byName(list []mktapi.Marketplace) map[string]mktapi.Marketplace {
	m := map[string]mktapi.Marketplace{}
	for _, e := range list {
		m[e.Name] = e
	}
	return m
}

// TestSeedOverlayMergeAndCRUD exercises the effective-list merge (seed ∪ user)
// and the CRUD paths, incl. disabling/removing a read-only seed entry via the
// overlay.
func TestSeedOverlayMergeAndCRUD(t *testing.T) {
	dir := t.TempDir()
	seed := filepath.Join(dir, "seed.yaml")
	overlay := filepath.Join(dir, "overlay.yaml")
	write(t, seed, `version: 1
marketplaces:
  - name: official
    url: https://seed.example/official.yaml
    enabled: true
  - name: extra
    url: https://seed.example/extra.yaml
    enabled: true
`)
	r := forPaths(seed, overlay)

	// Effective list from the seed only (no overlay yet).
	list, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 from seed, got %d", len(list))
	}
	if list[0].Source != "seed" {
		t.Fatalf("want seed source, got %q", list[0].Source)
	}

	// Add a user marketplace → written to the overlay.
	if err := r.Add("team", "https://user.example/team.yaml", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(overlay); err != nil {
		t.Fatalf("overlay not written: %v", err)
	}
	list, _ = r.List()
	if len(list) != 3 {
		t.Fatalf("want 3 after add, got %d", len(list))
	}
	if byName(list)["team"].Source != "user" {
		t.Fatal("team should have user source")
	}

	// Disable a SEED entry via the overlay; the seed URL must be preserved.
	if err := r.SetEnabled("official", false); err != nil {
		t.Fatal(err)
	}
	m := byName(mustList(t, r))
	if m["official"].Enabled {
		t.Fatal("official should be disabled")
	}
	if m["official"].URL != "https://seed.example/official.yaml" {
		t.Fatalf("disable-only override lost the seed URL: %q", m["official"].URL)
	}

	// Remove a seed entry → recorded as a disabled override (seed is read-only).
	if err := r.Remove("extra"); err != nil {
		t.Fatal(err)
	}
	if byName(mustList(t, r))["extra"].Enabled {
		t.Fatal("removed seed entry should be disabled")
	}

	// Remove a user entry → gone entirely.
	if err := r.Remove("team"); err != nil {
		t.Fatal(err)
	}
	if _, ok := byName(mustList(t, r))["team"]; ok {
		t.Fatal("team should be gone")
	}

	// Unknown name → error.
	if err := r.Remove("nope"); err == nil {
		t.Fatal("removing an unknown marketplace should error")
	}
}

func mustList(t *testing.T, r *Registry) []mktapi.Marketplace {
	t.Helper()
	list, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	return list
}

// TestPinnedKeyPersistsAndInherits: Add with a pinned key surfaces via List;
// a later URL-only update keeps the pinned key/trust (empty values inherit).
func TestPinnedKeyPersistsAndInherits(t *testing.T) {
	dir := t.TempDir()
	r := forPaths(filepath.Join(dir, "seed.yaml"), filepath.Join(dir, "overlay.yaml"))

	if err := r.Add("team", "https://team.example/mkt.yaml", "PINNEDKEYB64", "pinned"); err != nil {
		t.Fatal(err)
	}
	got := byName(mustList(t, r))["team"]
	if got.Trust != "pinned" || got.PubKey != "PINNEDKEYB64" {
		t.Fatalf("pinned trust not surfaced: trust=%q pubKey=%q", got.Trust, got.PubKey)
	}

	// URL-only update (empty key/trust) must NOT drop the pin.
	if err := r.Add("team", "https://team.example/mkt-v2.yaml", "", ""); err != nil {
		t.Fatal(err)
	}
	got = byName(mustList(t, r))["team"]
	if got.URL != "https://team.example/mkt-v2.yaml" {
		t.Fatalf("url not updated: %q", got.URL)
	}
	if got.Trust != "pinned" || got.PubKey != "PINNEDKEYB64" {
		t.Fatalf("pin dropped on url-only update: trust=%q pubKey=%q", got.Trust, got.PubKey)
	}
}

// AN INSTALLATION CHOOSES ITS CHANNEL - a marketplace is
// followed on stable by default; SetChannel puts it on canary and back; a seed
// marketplace takes the choice through the overlay; an unknown channel or
// marketplace is refused.
func TestAnInstallationChoosesItsChannel(t *testing.T) {
	dir := t.TempDir()
	r := forPaths(filepath.Join(dir, "seed.yaml"), filepath.Join(dir, "overlay.yaml"))
	if err := r.Add("archifactory", "https://af.example/mkt.yaml", "", ""); err != nil {
		t.Fatal(err)
	}
	if got := byName(mustList(t, r))["archifactory"].Channel; got != "" {
		t.Fatalf("a new subscription follows %q; want stable (empty)", got)
	}
	if err := r.SetChannel("archifactory", mktapi.ChannelCanary); err != nil {
		t.Fatal(err)
	}
	if got := byName(mustList(t, r))["archifactory"]; got.Channel != mktapi.ChannelCanary || got.URL != "https://af.example/mkt.yaml" {
		t.Errorf("after SetChannel canary: %+v; want canary, its url kept", got)
	}
	if err := r.Add("archifactory", "https://af.example/mkt-v2.yaml", "", ""); err != nil {
		t.Fatal(err)
	}
	if got := byName(mustList(t, r))["archifactory"].Channel; got != mktapi.ChannelCanary {
		t.Errorf("a url update dropped the channel: %q", got)
	}
	if err := r.SetChannel("archifactory", mktapi.ChannelStable); err != nil {
		t.Fatal(err)
	}
	if got := byName(mustList(t, r))["archifactory"].Channel; got != "" {
		t.Errorf("back on stable: %q; want stable (empty)", got)
	}
	if err := r.SetChannel("archifactory", "beta"); err == nil {
		t.Error("an unknown channel was accepted")
	}
	if err := r.SetChannel("nowhere", mktapi.ChannelCanary); err == nil {
		t.Error("a channel was set on an unknown marketplace")
	}
}
