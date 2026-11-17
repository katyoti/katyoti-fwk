// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/authoring"
)

// : a canary cut publishes its control plane without moving
// infra/channel.yaml, which every installation follows, stable ones
// included; a stable cut moves it.
func TestACanaryCutLeavesTheInfraPointerOnTheLastStableVersion(t *testing.T) {
	dir := t.TempDir()
	market, keys := filepath.Join(dir, "market"), filepath.Join(dir, "keys")
	a := authoring.New()
	if _, err := a.InitMarketplace(market, "acme", keys, "acme"); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(keys, "acme.key")
	dist := filepath.Join(dir, "dist")
	if err := os.MkdirAll(filepath.Join(dist, "windows-amd64"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "windows-amd64", "katyoti.exe"), []byte("k"), 0o755); err != nil {
		t.Fatal(err)
	}
	pointer := func() string {
		b, _ := os.ReadFile(filepath.Join(market, "infra", "channel.yaml"))
		return string(b)
	}

	if err := publishInfra(a, mktapi.ChannelStable, dist, market, "2026.09", secret, "archifactory@2026.09"); err != nil {
		t.Fatal(err)
	}
	if err := publishInfra(a, mktapi.ChannelCanary, dist, market, "2026.10-rc1", secret, "archifactory@2026.10-rc1"); err != nil {
		t.Fatal(err)
	}
	if p := pointer(); strings.Contains(p, "rc1") || !strings.Contains(p, "2026.09") {
		t.Fatalf("a canary cut moved the infra pointer:\n%s", p)
	}
	if _, err := os.Stat(filepath.Join(market, "infra", "2026.10-rc1")); err != nil {
		t.Fatalf("the candidate's control plane is not published: %v", err)
	}
	if err := publishInfra(a, "", dist, market, "2026.10", secret, "archifactory@2026.10"); err != nil {
		t.Fatal(err)
	}
	if p := pointer(); !strings.Contains(p, "2026.10") || strings.Contains(p, "rc1") {
		t.Fatalf("a stable cut did not move the infra pointer:\n%s", p)
	}
}
