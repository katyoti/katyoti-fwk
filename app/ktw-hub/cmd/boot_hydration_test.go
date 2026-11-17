// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
)

// TestBootIsReadyBeforeAStaleRecordIsReprobed: a rebuilt member's re-probe
// waits for the OS to scan the fresh executable (11-29 s measured), and the
// hub used to write its lockfile — what every client waits for — only after
// it. The hub is ready first now: the catalog serves the member's previous
// record, the re-probe runs after, the catalog refreshes when it lands, and
// the watches start once it has. A member with NO record is still probed
// before the lockfile — there is nothing of it to serve.
func TestBootIsReadyBeforeAStaleRecordIsReprobed(t *testing.T) {
	cache := manifestlocal.NewCache(t.TempDir())
	bin := t.TempDir()
	stale := filepath.Join(bin, "ktw-a.exe")  // recorded, then rebuilt
	absent := filepath.Join(bin, "ktw-b.exe") // never recorded
	for _, exe := range []string{stale, absent} {
		if err := os.WriteFile(exe, []byte("build-1"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := &mbapi.Manifest{Provider: "ktw-a", Version: "1", Hash: "h1"}
	if err := cache.Save("ktw-a", m, stale, manifestlocal.SourceDigests{Binary: stale}); err != nil {
		t.Fatal(err)
	}
	sidecars := []appcore.SidecarSpec{
		{Name: "ktw-a", Binary: stale},
		{Name: "ktw-b", Binary: absent},
	}

	var (
		mu    sync.Mutex
		steps []string
	)
	log := func(s string) { mu.Lock(); steps = append(steps, s); mu.Unlock() }
	scanDone := make(chan struct{}) // the OS scan of the rebuilt executable
	watching := make(chan struct{})
	returned := make(chan error, 1)
	go func() {
		returned <- bootHydration(cache, sidecars, bootSteps{
			hydrate: func(members []appcore.SidecarSpec) int {
				for _, s := range members {
					if s.Name == "ktw-a" {
						<-scanDone
					}
					log("probe " + s.Name)
				}
				return len(members)
			},
			refresh: func() { log("refresh") },
			ready:   func() error { log("ready"); return nil },
			after:   func() { log("watch"); close(watching) },
		})
	}()

	select {
	case err := <-returned:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		close(scanDone)
		t.Fatal("the hub is not ready while a stale record's re-probe waits: clients wait for the scan")
	}
	mu.Lock()
	early := append([]string(nil), steps...)
	mu.Unlock()
	if want := []string{"probe ktw-b", "refresh", "ready"}; !reflect.DeepEqual(early, want) {
		t.Fatalf("before the stale re-probe lands: %v, want %v", early, want)
	}

	close(scanDone)
	select {
	case <-watching:
	case <-time.After(5 * time.Second):
		t.Fatal("the watches never started after the re-probe")
	}
	mu.Lock()
	defer mu.Unlock()
	if want := []string{"probe ktw-b", "refresh", "ready", "probe ktw-a", "refresh", "watch"}; !reflect.DeepEqual(steps, want) {
		t.Fatalf("boot steps %v, want %v", steps, want)
	}
}

// TestBootWithEveryRecordFreshDoesNotRefresh: nothing probed, nothing
// reloaded — the catalog was built from these very records.
func TestBootWithEveryRecordFreshDoesNotRefresh(t *testing.T) {
	cache := manifestlocal.NewCache(t.TempDir())
	exe := filepath.Join(t.TempDir(), "ktw-a.exe")
	if err := os.WriteFile(exe, []byte("build-1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cache.Save("ktw-a", &mbapi.Manifest{Provider: "ktw-a", Version: "1", Hash: "h1"}, exe, manifestlocal.SourceDigests{Binary: exe}); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	refreshed := false
	err := bootHydration(cache, []appcore.SidecarSpec{{Name: "ktw-a", Binary: exe}}, bootSteps{
		hydrate: func([]appcore.SidecarSpec) int { return 0 },
		refresh: func() { refreshed = true },
		ready:   func() error { return nil },
		after:   func() { close(done) },
	})
	if err != nil {
		t.Fatal(err)
	}
	<-done
	if refreshed {
		t.Error("the catalog reloaded although no record was saved")
	}
}
