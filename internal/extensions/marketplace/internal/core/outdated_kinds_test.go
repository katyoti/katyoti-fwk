// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// outdated reports only what an update moves: a sidecar or content item, never
// a config-set, a seed or a vault — a newer one in the store reaches no universe
// (each is applied or imported once), so listing it invites a no-op update.
func TestOutdatedListsOnlyWhatAnUpdateMoves(t *testing.T) {
	for kind, want := range map[mktapi.ItemKind]bool{
		mktapi.KindSidecar: true, mktapi.KindContent: true,
		mktapi.KindConfig: false, mktapi.KindSeed: false, mktapi.KindVault: false,
	} {
		if got := updatableInPlace(string(kind)); got != want {
			t.Errorf("updatableInPlace(%s) = %v, want %v", kind, got, want)
		}
	}
}
