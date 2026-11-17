// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"testing"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// AN INSTALLATION FOLLOWS ITS CHANNEL - one following stable
// (the default) never sees a candidate, and an item that has only candidates
// is not there for it; one following canary sees every version.
func TestAnInstallationFollowsItsChannel(t *testing.T) {
	cat := mktapi.Catalog{Items: []mktapi.Item{
		{Name: "archifactory", Versions: []mktapi.ItemVersion{{Version: "2026.10-rc1", Channel: mktapi.ChannelCanary}, {Version: "2026.09"}}},
		{Name: "preview", Versions: []mktapi.ItemVersion{{Version: "0.1-rc1", Channel: mktapi.ChannelCanary}}},
	}}
	stable := ForChannel(cat, "")
	if len(stable.Items) != 1 || len(stable.Items[0].Versions) != 1 || stable.Items[0].Versions[0].Version != "2026.09" {
		t.Errorf("a stable installation sees %+v; want archifactory 2026.09 only", stable.Items)
	}
	canary := ForChannel(cat, mktapi.ChannelCanary)
	if len(canary.Items) != 2 || canary.Items[0].Versions[0].Version != "2026.10-rc1" {
		t.Errorf("a canary installation sees %+v; want every version, the candidate first", canary.Items)
	}
	if len(cat.Items[0].Versions) != 2 {
		t.Error("filtering changed the fetched catalog")
	}
}
