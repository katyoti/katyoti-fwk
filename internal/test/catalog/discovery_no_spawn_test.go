// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestremote "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/remote"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
	"github.com/katyoti/katyoti-fwk/internal/test/hubharness"
)

// noSpawnMembers are the binaries this smoke copies away. Three is enough to
// hold the claim for a universe (several providers, several kinds) and small
// enough that copying them stays cheap.
var noSpawnMembers = []string{"ktw-docs", "ktw-methods", "ktw-weather"}

// TestDiscovery_AnswersWithEveryBinaryAbsent is goal G12's probe: learning what
// the universe offers never starts, wakes or calls a contributing binary.
//
// Boot 1 hydrates the hub's cache from copies of the member binaries. Then
// every copy is renamed away and the hub boots again on the same cache: the
// index, the bodies and the kind schemas must answer for the whole universe,
// because nothing exists that could be spawned. Then the cache is deleted and
// the hub boots a third time: with no cache and no binaries it must serve an
// EMPTY universe — fail loudly, never invent a view and never spawn.
func TestDiscovery_AnswersWithEveryBinaryAbsent(t *testing.T) {
	binDir := t.TempDir()
	var sidecars []hubharness.SidecarBin
	for _, name := range noSpawnMembers {
		src := binaryFor(t, name)
		if src == "" {
			hubharness.MissingBinary(t, "no-spawn discovery: sidecar binary %s not built", name)
			return
		}
		dst := filepath.Join(binDir, filepath.Base(src))
		copyFile(t, src, dst)
		sidecars = append(sidecars, hubharness.SidecarBin{Name: name, Binary: dst})
	}
	ws := makeWorkspace(t, sidecars)
	dataDir := t.TempDir()
	cacheDir := t.TempDir()
	t.Setenv("KTW_WORKSPACE", ws)
	t.Setenv("KTW_CACHE", cacheDir)

	first := bootAndSnapshot(t, dataDir)
	// Every member, plus the hub's own manifest (in the view since e24 / N11;
	// built in process by the running hub, never spawned or probed).
	if len(first.manifestHash) != len(sidecars)+1 {
		t.Fatalf("boot 1: %d providers, want %d members + the hub — the cache was not hydrated from the copies", len(first.manifestHash), len(sidecars))
	}
	tier1 := filepath.Join(cacheDir, "sidecars")
	t.Logf("tier 1 after boot 1: %v", dirNames(tier1))

	// Every binary renamed away. The workspace still names the old paths, so a
	// hub that tried to spawn or probe one would fail — visibly, in the view.
	for _, sc := range sidecars {
		if err := os.Rename(sc.Binary, sc.Binary+".away"); err != nil {
			t.Fatal(err)
		}
	}
	second := bootAndProbeDiscovery(t, dataDir)
	t.Logf("tier 1 after boot 2: %v", dirNames(tier1))
	for p, h := range first.manifestHash {
		if second.manifestHash[p] != h {
			t.Errorf("boot 2 (binaries absent): provider %s manifest %q → %q; discovery must answer from the cache alone", p, h, second.manifestHash[p])
		}
	}
	if len(second.verdicts) != len(first.verdicts) {
		t.Errorf("boot 2 (binaries absent): %d entries, want %d — the index shrank without its binaries", len(second.verdicts), len(first.verdicts))
	}

	// No cache and no binaries: the hub still DECLARES its members (they are
	// in the workspace), but carries no digest and no manifest for any of
	// them and serves an empty index — it fails loudly, it does not invent a
	// view, and it cannot have spawned anything.
	if err := os.RemoveAll(tier1); err != nil {
		t.Fatal(err)
	}
	bootAndAssertEmptyUniverse(t, dataDir, len(sidecars))
	t.Logf("no-spawn discovery: %d providers, %d entries answered with every binary absent; empty and loud with the cache gone", len(second.manifestHash), len(second.verdicts))
}

// bootAndAssertEmptyUniverse boots the hub over a workspace whose members
// have neither a cached record nor a binary, and asserts the universe reads
// as declared-but-uncollected: members named, no digests, no manifest, no
// entries.
func bootAndAssertEmptyUniverse(t *testing.T, dataDir string, declared int) {
	t.Helper()
	stop := hubharness.Start(t, dataDir)
	defer stop()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	hc, err := hubclient.ConnectToHub(ctx, hubclient.ConnectOptions{DataDir: dataDir, Mode: hubclient.ModeReuse})
	if err != nil {
		t.Fatalf("connect to hub: %v", err)
	}
	defer hc.Close()
	cat := manifestremote.New(messagebus.NewSender(hc.MessageBus(), tenantID, universeName))
	all, err := cat.Providers(ctx)
	if err != nil {
		t.Fatalf("boot 3: providers: %v", err)
	}
	// The hub's own manifest is built by the running hub itself (e24 / N11),
	// so it is present with no cache and no binary; every MEMBER must be
	// declared-but-uncollected.
	var names []string
	for _, p := range all {
		if p != hubSelfProvider {
			names = append(names, p)
		}
	}
	if len(names) != declared {
		t.Errorf("boot 3: %d members named, want the %d the workspace declares", len(names), declared)
	}
	digests, err := cat.Digests(ctx)
	if err != nil {
		t.Fatalf("boot 3: digests: %v", err)
	}
	delete(digests, hubSelfProvider)
	if len(digests) != 0 {
		t.Errorf("boot 3 (no cache, no binaries): %d digests — a record nobody could have collected", len(digests))
	}
	for _, p := range names {
		if _, err := cat.Manifest(ctx, p); err == nil {
			t.Errorf("boot 3: manifest %s answered with no cache and no binary — invented, or spawned", p)
		}
	}
	entries, err := mbapi.Query(ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat})
	if err != nil {
		t.Fatalf("boot 3: index: %v", err)
	}
	// The hub's manifest carries the system set (e26), so `system` entries are
	// the hub's too, present with no cache and no binary.
	members := 0
	for _, e := range entries {
		if e.Provider != hubSelfProvider && e.Provider != "system" {
			members++
		}
	}
	if members != 0 {
		t.Errorf("boot 3 (no cache, no binaries): %d member entries in the index", members)
	}
}

// hubSelfProvider is the hub's own provider name: its manifest is in every
// universe view (e24 / N11).
const hubSelfProvider = "ktw-hub"

// bootAndProbeDiscovery is bootAndSnapshot plus the two other discovery
// questions G12 names: a body read and the kind schemas, both answered for
// the whole universe without a producer.
func bootAndProbeDiscovery(t *testing.T, dataDir string) indexSnapshot {
	t.Helper()
	stop := hubharness.Start(t, dataDir)
	defer stop()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	hc, err := hubclient.ConnectToHub(ctx, hubclient.ConnectOptions{DataDir: dataDir, Mode: hubclient.ModeReuse})
	if err != nil {
		t.Fatalf("connect to hub: %v", err)
	}
	defer hc.Close()
	cat := manifestremote.New(messagebus.NewSender(hc.MessageBus(), tenantID, universeName))
	snap := snapshotUniverse(t, cat)

	schemas, err := mbapi.KindSchemas(ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat})
	if err != nil || len(schemas) == 0 {
		t.Errorf("KindSchemas with every binary absent: %d schemas, err=%v", len(schemas), err)
	}
	skills, err := mbapi.Query(ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Kind: "skill", Catalog: cat})
	if err != nil || len(skills) == 0 {
		t.Fatalf("Query(skill) with every binary absent: %d entries, err=%v", len(skills), err)
	}
	body, err := cat.GetBody(ctx, skills[0].ResolvedURI())
	if err != nil || len(body) == 0 {
		t.Errorf("GetBody(%s) with every binary absent: %d bytes, err=%v — the data lane must be served from the cache", skills[0].ResolvedURI(), len(body), err)
	}
	return snap
}

func dirNames(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{"(" + err.Error() + ")"}
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(dst, ".exe") {
		_ = os.Chmod(dst, 0o755)
	}
}
