// SPDX-License-Identifier: Apache-2.0

package authoring

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/index"
)

// Init makes a marketplace with its key, and never replaces an existing key.
func TestInitMarketplaceKeepsItsKey(t *testing.T) {
	dir := t.TempDir()
	r, err := New().InitMarketplace(filepath.Join(dir, "market"), "acme", filepath.Join(dir, "keys"), "acme")
	if err != nil {
		t.Fatal(err)
	}
	if !r.KeyCreated || r.PubKeyLine == "" {
		t.Fatalf("first init = %+v, want a key created", r)
	}
	for _, f := range []string{"market/katyoti-marketplace.yaml", "market/katyoti-marketplace.yaml.minisig", "market/acme.key.pub"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("init should write %s: %v", f, err)
		}
	}
	again, err := New().InitMarketplace(filepath.Join(dir, "market"), "acme", filepath.Join(dir, "keys"), "acme")
	if err != nil {
		t.Fatal(err)
	}
	if again.KeyCreated || again.PubKeyLine != r.PubKeyLine {
		t.Fatalf("a second init replaced the key: %+v", again)
	}
}

// The infra channel: every platform's control-plane binaries with signed
// SHA256SUMS, the pointer naming the version, the previous one and the key,
// the installers; a published version is refused again; a version the pointer
// no longer reaches is pruned.
func TestBuildInfraChannel(t *testing.T) {
	dir := t.TempDir()
	market := filepath.Join(dir, "market")
	keys := filepath.Join(dir, "keys")
	if _, err := New().InitMarketplace(market, "acme", keys, "acme"); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(keys, "acme.key")
	dist := filepath.Join(dir, "dist")
	for _, p := range []string{"windows-amd64", "linux-amd64"} {
		_ = os.MkdirAll(filepath.Join(dist, p), 0o755)
		_ = os.WriteFile(filepath.Join(dist, p, "katyoti"), []byte("k-"+p), 0o755)
		_ = os.WriteFile(filepath.Join(dist, p, "ktw-hub"), []byte("h-"+p), 0o755)
		_ = os.WriteFile(filepath.Join(dist, p, "not-infra"), []byte("x"), 0o755)
	}
	_ = os.MkdirAll(filepath.Join(dist, "notes"), 0o755)

	for _, v := range []string{"v1", "v2", "v3"} {
		r, err := New().BuildInfraChannel(dist, market, v, secret, "archi@"+v)
		if err != nil {
			t.Fatalf("%s: %v", v, err)
		}
		if len(r.Platforms) != 2 {
			t.Fatalf("%s platforms = %v", v, r.Platforms)
		}
	}
	ch := filepath.Join(market, "infra")
	pointer, _ := os.ReadFile(filepath.Join(ch, "channel.yaml"))
	for _, want := range []string{"version: v3", "previous: v2", "key: acme.key.pub", "solution: archi@v3"} {
		if !strings.Contains(string(pointer), want) {
			t.Fatalf("channel.yaml lacks %q:\n%s", want, pointer)
		}
	}
	sums, _ := os.ReadFile(filepath.Join(ch, "v3", "linux-amd64", "SHA256SUMS"))
	if !strings.Contains(string(sums), "  katyoti") || !strings.Contains(string(sums), "  ktw-hub") || strings.Contains(string(sums), "not-infra") {
		t.Fatalf("SHA256SUMS should list the infra binaries only:\n%s", sums)
	}
	for _, f := range []string{"channel.yaml.minisig", "v3/linux-amd64/SHA256SUMS.minisig", "install.ps1", "install.sh", "install-on-windows.cmd", "install-on-mac.command", "acme.key.pub"} {
		if _, err := os.Stat(filepath.Join(ch, f)); err != nil {
			t.Errorf("the channel should hold %s: %v", f, err)
		}
	}
	if _, err := os.Stat(filepath.Join(ch, "v1")); err == nil {
		t.Error("v1 is reached by nothing and should be pruned")
	}
	if _, err := New().BuildInfraChannel(dist, market, "v4", secret, "no-version"); err == nil {
		t.Error("a solution without a version should be refused")
	}
	if _, err := os.Stat(filepath.Join(ch, "v4")); err == nil {
		t.Error("a refused publish left v4 behind")
	}
	if _, err := New().BuildInfraChannel(dist, market, "v3", secret, ""); err == nil {
		t.Error("publishing v3 again should be refused")
	}
}

// A CANDIDATE CUT NEVER MOVES THE POINTER STABLE INSTALLATIONS FOLLOW
// : a canary cut publishes its control plane beside the
// stable one and leaves infra/channel.yaml on the stable version; promoting
// the candidate points the channel at the candidate's own control plane -
// nothing rebuilt - with the promoted solution at its official version.
func TestACanaryCutLeavesTheInfraPointerOnStableUntilPromoted(t *testing.T) {
	dir := t.TempDir()
	market, keys := filepath.Join(dir, "market"), filepath.Join(dir, "keys")
	s := New()
	if _, err := s.InitMarketplace(market, "acme", keys, "acme"); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(keys, "acme.key")
	dist := filepath.Join(dir, "dist")
	_ = os.MkdirAll(filepath.Join(dist, "windows-amd64"), 0o755)
	_ = os.WriteFile(filepath.Join(dist, "windows-amd64", "katyoti.exe"), []byte("k"), 0o755)
	pointer := func() infraPointer {
		var p infraPointer
		b, _ := os.ReadFile(filepath.Join(market, "infra", "channel.yaml"))
		if err := config.DecodeBytes(b, marshal.YAML, &p); err != nil {
			t.Fatal(err)
		}
		return p
	}

	if _, err := s.BuildInfraChannel(dist, market, "2026.09", secret, "archifactory@2026.09"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BuildInfraVersion(dist, market, "2026.10-rc2", secret, "archifactory@2026.10-rc2"); err != nil {
		t.Fatal(err)
	}
	if p := pointer(); p.Version != "2026.09" || p.Solution != "archifactory@2026.09" {
		t.Fatalf("after the canary cut the pointer moved: %+v", p)
	}
	if _, err := os.Stat(filepath.Join(market, "infra", "2026.10-rc2", "windows-amd64", "katyoti.exe")); err != nil {
		t.Fatalf("the candidate's control plane is not published: %v", err)
	}

	cat, err := loadCatalogFile(filepath.Join(market, "katyoti-marketplace.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	cat.Items = append(cat.Items, mktapi.Item{Name: "archifactory", Kind: mktapi.KindSolution,
		Versions: []mktapi.ItemVersion{{Version: "2026.10-rc2", Channel: mktapi.ChannelCanary}, {Version: "2026.09"}}})
	data, err := index.MarshalCatalog(cat)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(market, "katyoti-marketplace.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Promote(market, "2026.10-rc2", "2026.10", secret); err != nil {
		t.Fatal(err)
	}
	if p := pointer(); p.Version != "2026.10-rc2" || p.Previous != "2026.09" || p.Solution != "archifactory@2026.10" {
		t.Fatalf("after the promotion the pointer = %+v, want the candidate's control plane, 2026.09 kept for roll back, the solution at 2026.10", p)
	}
	if _, err := os.Stat(filepath.Join(market, "infra", "channel.yaml.minisig")); err != nil {
		t.Fatalf("the pointer is not signed: %v", err)
	}
}
