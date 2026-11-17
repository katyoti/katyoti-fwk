// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"fmt"
	"sync"
	"testing"
	"testing/fstest"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gates/api"
)

func init() {
	pack["capabilities/notes.yaml"] = &fstest.MapFile{Data: []byte("name: notes\n")}
}

// A confirmation and another write of the same object at once both land: the
// confirmation is written on the version it read and redone when it lost the
// race, so neither erases the other.
func TestAConfirmationAndAConcurrentWriteBothLand(t *testing.T) {
	ctx, cwm, g, _ := world(t)
	const n = 30
	for i := 0; i < n; i++ {
		o := cwmapi.Object{ID: fmt.Sprintf("m.r%d", i), Type: "feature", Name: "r", Parent: "m"}
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("m.r%d", i)
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, err := g.Confirm(ctx, api.ConfirmInput{ID: id, Gate: "ready-to-build", Requirement: "owner-agrees", By: ana, Note: "ok"}); err != nil {
				t.Errorf("confirm %s: %v", id, err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := cwm.ObjectAddCapability(ctx, cwmapi.ObjectAddCapabilityInput{ID: id, Capability: "notes"}); err != nil {
				t.Errorf("add notes to %s: %v", id, err)
			}
		}()
	}
	wg.Wait()
	for i := 0; i < n; i++ {
		got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: fmt.Sprintf("m.r%d", i)})
		if err != nil {
			t.Fatal(err)
		}
		_, notes := got.Object.Capabilities["notes"]
		_, confirmed := got.Object.Capabilities[api.CapGates][api.GateKey("ready-to-build", "owner-agrees")]
		if !notes || !confirmed {
			t.Errorf("%s: notes %v, confirmation %v — one write erased the other", got.Object.ID, notes, confirmed)
		}
	}
}
