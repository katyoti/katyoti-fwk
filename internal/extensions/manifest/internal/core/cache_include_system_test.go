// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCache_RecordsIncludeSystem — e24 / N2, e26, the record half:
// metadata.yaml says how the manifest was taken (`include_system: true|false`,
// always written), and a record saved with one setting reads stale for a
// caller that needs the other, whatever the exe stat says. Since e26 only the
// hub's own record is taken with the set; the comparison is what refuses a
// member record written before e26 as its universe's carrier (a second copy
// beside the hub's) instead of serving it.
func TestCache_RecordsIncludeSystem(t *testing.T) {
	for _, saved := range []bool{true, false} {
		cache := NewCache(t.TempDir())
		m := makeManifest("ktw-x", "1.0", "h1")
		exe := filepath.Join(t.TempDir(), "ktw-x.exe")
		if err := os.WriteFile(exe, []byte{0}, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := cache.Save("ktw-x", m, exe, SourceDigests{IncludeSystem: saved}); err != nil {
			t.Fatalf("Save: %v", err)
		}

		raw, err := os.ReadFile(filepath.Join(cache.RecordDir("ktw-x", exe), "metadata.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		want := "include_system: false"
		if saved {
			want = "include_system: true"
		}
		if !strings.Contains(string(raw), want) {
			t.Errorf("metadata.yaml does not state %q:\n%s", want, raw)
		}

		_, meta, err := cache.Load("ktw-x", exe)
		if err != nil || meta == nil {
			t.Fatalf("Load: meta=%v err=%v", meta, err)
		}
		if meta.IncludeSystem != saved {
			t.Errorf("loaded include_system = %t, saved %t", meta.IncludeSystem, saved)
		}
		if stale, why := cache.IsStale(meta, SourceDigests{Binary: exe, IncludeSystem: saved}); stale {
			t.Errorf("record saved with include_system=%t reads stale for the same role: %s", saved, why)
		}
		stale, why := cache.IsStale(meta, SourceDigests{Binary: exe, IncludeSystem: !saved})
		if !stale {
			t.Errorf("record saved with include_system=%t reads FRESH for a caller needing %t — the other role would keep 0 or 2 system copies", saved, !saved)
		} else if !strings.Contains(why, "include_system") {
			t.Errorf("stale for the wrong reason: %q", why)
		}
	}
}
