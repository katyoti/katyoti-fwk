// SPDX-License-Identifier: Apache-2.0

package machine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/api"
)

// : the Sandbox's preparation comes from a cache on the machine
// running the tool - fetched once, a fresh file kept, a stale one fetched
// again, a failed fetch leaving no half-made file and the network path - and
// the cached stages install offline from the folder mapped read-only.
func TestThePreparationCacheIsFetchedOnceAndUsedOffline(t *testing.T) {
	dir := t.TempDir()
	var fetched []string
	fetch := func(_ context.Context, url, dst string) error {
		fetched = append(fetched, filepath.Base(strings.TrimSuffix(dst, ".part")))
		return os.WriteFile(dst, []byte(url), 0o644)
	}
	if !EnsureCache(context.Background(), dir, fetch, nil) || len(fetched) != len(preparationCache) {
		t.Fatalf("first fill fetched %v", fetched)
	}
	fetched = nil
	if !EnsureCache(context.Background(), dir, fetch, nil) || len(fetched) != 0 {
		t.Fatalf("a fresh cache fetched %v again", fetched)
	}
	old := time.Now().Add(-cacheMaxAge - time.Hour)
	if err := os.Chtimes(filepath.Join(dir, cacheVCRedist), old, old); err != nil {
		t.Fatal(err)
	}
	if !EnsureCache(context.Background(), dir, fetch, nil) || len(fetched) != 1 || fetched[0] != cacheVCRedist {
		t.Fatalf("a stale file: fetched %v, want %s alone", fetched, cacheVCRedist)
	}

	// A failed fetch: not whole, and no partial file left.
	empty := t.TempDir()
	broken := func(_ context.Context, _, dst string) error {
		_ = os.WriteFile(dst, []byte("half"), 0o644)
		return errors.New("network down")
	}
	if EnsureCache(context.Background(), empty, broken, nil) {
		t.Fatal("a cache that could not be fetched said whole")
	}
	if entries, _ := os.ReadDir(empty); len(entries) != 0 {
		t.Fatalf("a failed fetch left %v", entries)
	}

	// The cached preparation installs from the mapped cache; the network one
	// is unchanged.
	stages := sandboxStagesOf(&api.CaseMachine{Stages: []api.MachineStage{{ID: "install", Install: true}}}, true)
	winget := strings.Join(stages[0].Cmd, " ")
	if !strings.Contains(winget, `Add-AppxPackage -Path '`+SandboxCache+`\`+cacheWinget+`'`) || strings.Contains(winget, "PSGallery") {
		t.Errorf("the cached winget preparation: %s", winget)
	}
	if vc := strings.Join(stages[1].Cmd, " "); !strings.HasPrefix(vc, SandboxCache+`\`+cacheVCRedist+" /install /quiet") {
		t.Errorf("the cached runtime preparation: %s", vc)
	}
	raw, err := WSB(`C:\m`, `C:\t`, `C:\r`, dir, 0)
	if err != nil || !strings.Contains(string(raw), "<SandboxFolder>"+SandboxCache+"</SandboxFolder>") {
		t.Fatalf("the cache is not mapped: %s %v", raw, err)
	}
	if raw, _ := WSB(`C:\m`, `C:\t`, `C:\r`, "", 0); strings.Contains(string(raw), SandboxCache) {
		t.Error("no cache, yet mapped")
	}
}
