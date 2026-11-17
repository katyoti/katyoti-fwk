// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// bigManifest has two slot types whose methods weigh about `size` bytes each.
func bigManifest(size int) *mbapi.Manifest {
	desc := strings.Repeat("x", size)
	return &mbapi.Manifest{Provider: "ktw-big", SlotTypes: []mbapi.SlotType{
		{Type: "Auth", Version: "1", Methods: []mbapi.SlotMethod{{Name: "ProveWait", Description: desc, TimeoutS: 960}}},
		{Type: "msft-drive", Version: "1", Methods: []mbapi.SlotMethod{{Name: "list", Description: desc}}},
	}}
}

// A manifest past the budget is answered by slot-type name, flagged, and
// small enough to cross: the bus would drop the whole reply and the caller
// wait out its timeout with nothing.
func TestGetManifestTooLargeAnswersTheSlotTypesByName(t *testing.T) {
	got := fitManifestReply(bigManifest(1000), "", 1500)
	if !got.SplitSlotTypes {
		t.Fatal("a manifest past the budget is not flagged split")
	}
	if n := len(got.Manifest.SlotTypes); n != 2 {
		t.Fatalf("slot types answered: %d, want 2", n)
	}
	for _, st := range got.Manifest.SlotTypes {
		if st.Methods != nil {
			t.Errorf("slot type %s carries its methods in a split reply", st.Type)
		}
	}
	if raw, _ := json.Marshal(got.Manifest); len(raw) > 1500 {
		t.Errorf("the split reply weighs %d bytes, past the budget", len(raw))
	}
}

// Within the budget the manifest is answered whole, unflagged.
func TestGetManifestWithinBudgetIsWhole(t *testing.T) {
	got := fitManifestReply(bigManifest(10), "", 1<<20)
	if got.SplitSlotTypes || len(got.Manifest.SlotTypes[0].Methods) != 1 {
		t.Fatalf("a manifest within budget is not answered whole: %+v", got)
	}
}

// One slot type is answered with its methods, found on its declared or kebab
// form, whatever the whole manifest weighs.
func TestGetManifestOneSlotType(t *testing.T) {
	for _, name := range []string{"Auth", "auth"} {
		got := fitManifestReply(bigManifest(1000), name, 1500)
		if got.SplitSlotTypes || len(got.Manifest.SlotTypes) != 1 || got.Manifest.SlotTypes[0].Methods[0].TimeoutS != 960 {
			t.Errorf("slot type %q: %+v", name, got)
		}
	}
}
