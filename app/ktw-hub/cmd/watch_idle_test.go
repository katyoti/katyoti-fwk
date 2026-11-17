// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
)

// watch_idle_test.go — an idle hub costs near zero CPU. Its watches run a
// round every couple of seconds for every member, for the life of the hub;
// a round that decoded each member's manifest (megabytes for ktw-agents) kept
// an idle hub at about a tenth of a core.

// TestAnIdleWatchRoundReadsNoManifest drives the rounds an idle hub runs —
// the override watch's freshness of every record, the member watch's exe stat
// and read folders — over a record whose manifest weighs megabytes, and bounds
// what they allocate: a few lines of metadata per member, never the manifest.
func TestAnIdleWatchRoundReadsNoManifest(t *testing.T) {
	root := t.TempDir()
	cache := manifestlocal.NewCache(filepath.Join(root, "sidecars"))
	exe := filepath.Join(root, "bin", "ktw-a.exe")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("build-1"), 0o644); err != nil {
		t.Fatal(err)
	}
	members := []appcore.SidecarSpec{{Name: "ktw-a", Binary: exe}}

	// A manifest the size of ktw-agents' (about 4 MB of JSON).
	m := &mbapi.Manifest{Provider: "ktw-a", Version: "1", Hash: "a-1"}
	pad := strings.Repeat("x", 160)
	for i := 0; i < 20000; i++ {
		m.Contributions = append(m.Contributions, contribapi.Contribution{
			Provider: "ktw-a", Type: "skill", ID: fmt.Sprintf("skill-%05d-%s", i, pad),
		})
	}
	if err := cache.Save("ktw-a", m, exe, sourceDigestsIn("ktw-a", members)); err != nil {
		t.Fatal(err)
	}
	manifests, _ := filepath.Glob(filepath.Join(cache.Root, "ktw-a@*", "manifest.json"))
	if len(manifests) != 1 {
		t.Fatalf("record manifests: %v, want one", manifests)
	}
	info, err := os.Stat(manifests[0])
	if err != nil || info.Size() < 3<<20 {
		t.Fatalf("the record's manifest weighs %v (%v), want megabytes", info.Size(), err)
	}

	exeWatch := newExeStatWatch(cache)
	round := func() {
		if stale := staleRecords(cache, members); len(stale) != 0 {
			t.Fatalf("an untouched record reads stale: %v", stale)
		}
		for _, s := range members {
			if exeWatch.moved(s.Name, s.Binary) {
				t.Fatalf("an untouched executable reads moved")
			}
			if moved, why := cache.ReadsMoved(s.Name, s.Binary); moved {
				t.Fatalf("untouched read folders read moved: %s", why)
			}
		}
	}
	round() // seeds the exe watch

	// MemStats counts the whole process: goroutines another test left behind
	// allocate during a window too, and a loaded machine stretches the window.
	// What they add only ever adds, so each round is measured alone, one
	// processor at a time, and the least of the rounds is bounded: a round that
	// decodes the manifest allocates megabytes every time, noise or none.
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))
	const rounds = 10
	least := ^uint64(0)
	for i := 0; i < rounds; i++ {
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		round()
		runtime.ReadMemStats(&after)
		least = min(least, after.TotalAlloc-before.TotalAlloc)
	}
	if limit := uint64(256 << 10); least > limit {
		t.Errorf("an idle watch round allocates %d KB for a %d KB manifest, want under %d KB: it reads the manifest",
			least>>10, info.Size()>>10, limit>>10)
	}
}
