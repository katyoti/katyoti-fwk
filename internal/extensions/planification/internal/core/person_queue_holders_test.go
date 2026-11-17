// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"

	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// TestTheHolderIsThePlaceLinesFirstWord: with IN-039's
// switch on, a section's place line names who holds its cards first -
// « Pour moi », then each holding list with its members, an agent said so -,
// the person's own first; switch off, the place line is the release and the
// increment alone, as it was.
func TestTheHolderIsThePlaceLinesFirstWord(t *testing.T) {
	holder, label := holderOf(inboxapi.WaitingGroup{List: "sys-inbox.hl-cosmos", Title: "hl-cosmos",
		Members: []inboxapi.Member{{ID: "jdoe", Kind: "person"}, {ID: "relecteur", Kind: "agent"}}}, "fr")
	if holder != "list:sys-inbox.hl-cosmos" || label != "Liste hl-cosmos · membres : jdoe, agent relecteur" {
		t.Errorf("a holding list = %q %q; want it named with its members", holder, label)
	}
	if h, l := holderOf(inboxapi.WaitingGroup{Mine: true}, "en"); h != holderMine || l != "Mine" {
		t.Errorf("the person's own = %q %q; want mine, « Mine »", h, l)
	}

	list := api.PersonItem{Key: "a", Holder: holder, HolderLabel: label, ReleaseTitle: "1.006", Release: "RL-1"}
	planned := api.PersonItem{Key: "b", ReleaseTitle: "1.006", Release: "RL-1"} // planification's own: the person's
	mine := api.PersonItem{Key: "c", Holder: holderMine, HolderLabel: "Pour moi", ReleaseTitle: "1.007", Release: "RL-2"}

	got := byPlace([]api.PersonItem{list, planned, mine})
	if got[2].Key != "a" {
		t.Errorf("order = %s %s %s; want the person's own first, the holding list last", got[0].Key, got[1].Key, got[2].Key)
	}
	if p := placeOf(list, "fr"); p != "Liste hl-cosmos · membres : jdoe, agent relecteur · 1.006 › hors de tout incrément" {
		t.Errorf("a holding list's place line = %q", p)
	}
	if p := placeOf(planned, "fr"); p != "Pour moi · 1.006 › hors de tout incrément" {
		t.Errorf("planification's own place line = %q; want « Pour moi » first", p)
	}
}
