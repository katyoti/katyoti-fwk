// SPDX-License-Identifier: Apache-2.0

package cmd

import "testing"

// The catalog is built from the tier-1 cache before the boot-time hydration
// runs; the hydration changes that cache only when it saves a re-probed
// member's record. Reloading (and re-running the whole Phase 2 reindex) when
// nothing was saved rebuilt the same catalog a second time, and the hub writes
// its lockfile — what every client waits for — only after that block.
func TestBootReloadsTheCatalogOnlyWhenTheHydrateSavedARecord(t *testing.T) {
	for _, c := range []struct {
		probed int
		want   bool
	}{
		{0, false}, // every member fresh, or every probe failed: the cache is as the catalog read it
		{1, true},
		{13, true},
	} {
		if got := needsCatalogReload(c.probed); got != c.want {
			t.Errorf("needsCatalogReload(%d) = %v, want %v", c.probed, got, c.want)
		}
	}
}
