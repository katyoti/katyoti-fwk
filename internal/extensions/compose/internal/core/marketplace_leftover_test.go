// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/compose/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/authoring"
)

// : a cut over a tree holding swap leftovers - a running
// katyoti.exe renamed aside by an update (katyoti.exe.old-<stamp>), in the
// build, in the published tree and in its mirror - publishes none: the
// published tree and the mirror hold only what is published.
func TestACutPublishesNoSwapLeftover(t *testing.T) {
	dir := t.TempDir()
	host := runtime.GOOS + "-" + runtime.GOARCH
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	write := func(p, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	const stamp = ".old-1792597412233358100"
	dist := filepath.Join(dir, "dist")
	write(filepath.Join(dist, host, "katyoti"+exe), "the control plane")
	// The build's leftovers: an earlier katyoti and an earlier sidecar
	// renamed aside while they ran.
	write(filepath.Join(dist, host, "katyoti"+exe+stamp), "a stale katyoti")
	write(filepath.Join(dist, host, "ktw-old"+exe+stamp), "a stale sidecar")

	market, keys := filepath.Join(dir, "market"), filepath.Join(dir, "keys")
	if _, err := authoring.New().InitMarketplace(market, "team", keys, "team"); err != nil {
		t.Fatal(err)
	}
	// The published tree's leftovers, from before this cut: at its root, in
	// the infra channel and in a version folder this cut keeps.
	write(filepath.Join(market, "katyoti"+exe+stamp), "stale")
	write(filepath.Join(market, "infra", "katyoti"+exe+stamp), "stale")
	write(filepath.Join(market, "sidecars", "v0", host, "ktw-x"+exe+".old"), "stale")
	mirrorDir := filepath.Join(dir, "mirror")
	write(filepath.Join(mirrorDir, "katyoti"+exe+stamp), "stale")

	def := "name: team\noutput: " + filepath.ToSlash(market) + "\nkey: {folder: " + filepath.ToSlash(keys) +
		", prefix: team}\nplatforms: [" + host + "]\nmirrors: [" + filepath.ToSlash(mirrorDir) + "]\n"
	write(filepath.Join(dir, "marketplace.yaml"), def)
	if _, err := New(nil).BuildFromDefinition(context.Background(), api.BuildFromDefinitionInput{
		Definition: filepath.Join(dir, "marketplace.yaml"), Version: "v1", Dist: dist,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(market, "infra", "v1", host, "katyoti"+exe)); err != nil {
		t.Fatalf("the cut did not publish the control plane: %v", err)
	}
	for _, root := range []string{market, mirrorDir} {
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && (strings.Contains(d.Name(), ".old")) {
				t.Errorf("the cut left a leftover in its published tree: %s", p)
			}
			return nil
		})
	}
}

// The final gate refuses a published tree holding a leftover: a tree changed
// after its cut is never handed out with one.
func TestTheGateRefusesATreeHoldingALeftover(t *testing.T) {
	dir := t.TempDir()
	market, keys := filepath.Join(dir, "market"), filepath.Join(dir, "keys")
	a := authoring.New()
	if _, err := a.InitMarketplace(market, "team", keys, "team"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Publish(market, ""); err != nil {
		t.Fatalf("a clean tree is refused: %v", err)
	}
	stray := filepath.Join(market, "infra", "v1", "windows-amd64", "katyoti.exe.old-1792597412233358100")
	if err := os.MkdirAll(filepath.Dir(stray), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stray, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := a.Publish(market, "")
	if err == nil || !strings.Contains(err.Error(), "infra/v1/windows-amd64/katyoti.exe.old-1792597412233358100") {
		t.Fatalf("the gate should refuse the leftover by name: %v", err)
	}
}
