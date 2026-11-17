// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	manifestremote "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/remote"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
	"github.com/katyoti/katyoti-fwk/internal/test/hubharness"
)

// TestTwoPacksUnderOneHostAreTwoProviders is goal G26's probe: one generic host
// binary serves two files-only packs, and the universe shows two providers —
// each under the pack's own identity, each with its own content — with no
// per-pack code anywhere.
func TestTwoPacksUnderOneHostAreTwoProviders(t *testing.T) {
	hostBin := binaryFor(t, "ktw-host")
	if hostBin == "" {
		hubharness.MissingBinary(t, "two packs: bin/ktw-host not built")
		return
	}
	packs := []string{"pack-alpha", "pack-beta"}
	folders := map[string]string{}
	for _, p := range packs {
		folders[p] = contentPack(t, p, "instructions of "+p)
	}
	ws := makeTwoPackWorkspace(t, hostBin, packs, folders)
	t.Setenv("KTW_WORKSPACE", ws)
	dataDir := filepath.Join(ws, ".katyoti")
	stop := hubharness.Start(t, dataDir)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	hc, err := hubclient.ConnectToHub(ctx, hubclient.ConnectOptions{DataDir: dataDir, Mode: hubclient.ModeReuse})
	if err != nil {
		t.Fatalf("connect to hub: %v", err)
	}
	defer hc.Close()
	sender := messagebus.NewSender(hc.MessageBus(), tenantID, contentUniverse)
	cat := manifestremote.New(sender)

	all, err := cat.Providers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// The hub's own manifest is in every view (e24 / N11); the packs are what
	// the host contributes.
	var providers []string
	for _, p := range all {
		if p != hubSelfProvider {
			providers = append(providers, p)
		}
	}
	sort.Strings(providers)
	if strings.Join(providers, ",") != strings.Join(packs, ",") {
		t.Fatalf("universe providers = %v, want exactly the two packs %v", providers, packs)
	}
	for _, p := range packs {
		m, err := cat.Manifest(ctx, p)
		if err != nil {
			t.Fatalf("manifest %s: %v", p, err)
		}
		if m.Provider != p {
			t.Errorf("pack %s: manifest provider %q — the host did not adopt the pack's identity", p, m.Provider)
		}
		own := 0
		for _, pi := range m.Providers {
			if pi.Name == p {
				own++
			}
		}
		if own == 0 {
			t.Errorf("pack %s: no provider identity under its own name in %d providers", p, len(m.Providers))
		}
		body := readBody(t, sender, p+":/skill/live-skill", 20*time.Second)
		if !strings.Contains(body, "instructions of "+p) {
			t.Errorf("pack %s: its skill reads %q — content is not the pack's own", p, body)
		}
	}
	t.Logf("two packs under one host: providers %v, each serving its own skill", providers)
}

// makeTwoPackWorkspace declares two ktw-host members over two folders — the
// same host binary twice, two identities.
func makeTwoPackWorkspace(t *testing.T, hostBin string, packs []string, folders map[string]string) string {
	t.Helper()
	ws := t.TempDir()
	cfgDir := filepath.Join(ws, ".katyoti", "config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	var members strings.Builder
	for _, p := range packs {
		f := filepath.ToSlash(folders[p])
		fmt.Fprintf(&members, "  - name: %s\n    transport: stdio\n    binary: %s\n    content_root: %s\n    args: [\"--folder\", \"%s\", \"sc\", \"ktsp\", \"stdio\"]\n",
			p, filepath.ToSlash(hostBin), f, f)
	}
	manifest := fmt.Sprintf("version: 1\nname: %s\ndisplay_name: Two Packs\nicon: T\ncolor: gray\nsidecars:\n%s", contentUniverse, members.String())
	if err := os.WriteFile(filepath.Join(cfgDir, "Univers.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "Settings.yaml"), []byte("# two packs test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return ws
}
