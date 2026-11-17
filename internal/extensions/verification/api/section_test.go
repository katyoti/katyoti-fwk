// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// The lint bar is the type's `verification:` section's, read through the one
// section reader; a type writing none has the default bar.
func TestVerificationOfReadsTheSection(t *testing.T) {
	var ot cwmapi.ObjectType
	if err := json.Unmarshal([]byte(`{"name":"skill","verification":{"gate_blocks_on":"warn"}}`), &ot); err != nil {
		t.Fatal(err)
	}
	got, err := VerificationOf(ot)
	if err != nil || got.GateBlocksOn != "warn" {
		t.Fatalf("VerificationOf = %+v, %v", got, err)
	}
	none, err := VerificationOf(cwmapi.ObjectType{Name: "plain"})
	if err != nil || none.GateBlocksOn != "" {
		t.Errorf("no section = %+v, %v; want the zero settings", none, err)
	}
	if c := (cwmapi.Capability{Name: VerificationCapability, TypeSchema: "x"}); !c.IsTypeLevel() {
		t.Error("the verification capability with no group is not type-level")
	}
}
