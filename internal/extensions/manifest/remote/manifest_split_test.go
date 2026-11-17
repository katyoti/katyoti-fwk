// SPDX-License-Identifier: Apache-2.0

package remote_test

import (
	"context"
	"testing"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/manifest/remote"
)

// A provider whose methods do not fit one reply is read one slot type per
// reply, and the caller gets every method — the timeout an action run applies
// included. Before, the whole reply was dropped and the provider had no
// callable lane in the universe view.
func TestManifestSlotsReadsASplitProviderPerSlotType(t *testing.T) {
	m := &mbapi.Manifest{Provider: "ktw-example", SlotTypes: []mbapi.SlotType{
		{Type: "auth", Methods: []mbapi.SlotMethod{{Name: "prove-wait", TimeoutS: 960}}},
		{Type: "msft-drive", Methods: []mbapi.SlotMethod{{Name: "list"}, {Name: "get"}}},
	}}
	f := &fakeSender{manifests: map[string]*mbapi.Manifest{"ktw-example": m}, split: map[string]bool{"ktw-example": true}}
	got, err := remote.New(f).ManifestSlots(context.Background(), "ktw-example")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.SlotTypes) != 2 || len(got.SlotTypes[0].Methods) != 1 || len(got.SlotTypes[1].Methods) != 2 {
		t.Fatalf("slot types read: %+v", got.SlotTypes)
	}
	if got.SlotTypes[0].Methods[0].TimeoutS != 960 {
		t.Errorf("prove-wait's timeout lost: %+v", got.SlotTypes[0].Methods[0])
	}
	if len(f.paths) != 3 {
		t.Errorf("calls: %d (%v), want one for the names and one per slot type", len(f.paths), f.paths)
	}
}

// A provider that fits is read in one call.
func TestManifestSlotsReadsAWholeProviderOnce(t *testing.T) {
	m := &mbapi.Manifest{Provider: "ktw-methods", SlotTypes: []mbapi.SlotType{{Type: "decisions", Methods: []mbapi.SlotMethod{{Name: "resolve"}}}}}
	f := &fakeSender{manifests: map[string]*mbapi.Manifest{"ktw-methods": m}}
	got, err := remote.New(f).ManifestSlots(context.Background(), "ktw-methods")
	if err != nil || len(got.SlotTypes[0].Methods) != 1 || len(f.paths) != 1 {
		t.Fatalf("got %+v err %v calls %v", got, err, f.paths)
	}
}
