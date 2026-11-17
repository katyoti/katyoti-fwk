// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// hash_test.go — the manifest hash, which lives in this package.
//
// These two travelled with slot_mapping_test.go out of proximity, not subject:
// they exercise computeHash / ReHash and only USE slots as the thing that
// changes. When the KTSP mapping moved to local/ they came along and stopped
// compiling, which is the proximity showing.

func TestComputeHash_SensitiveToSlots(t *testing.T) {
	base := &mbapi.Manifest{
		Provider:      "ktw-docs",
		Version:       "1.0.0",
		Contributions: []contribapi.Contribution{{Provider: "ktw-docs", Type: "skill", ID: "x"}},
	}
	withoutSlots := computeHash(base)

	base.Slots = []mbapi.Slot{{Name: "docs", Type: "DocService"}}
	withSlots := computeHash(base)

	if withoutSlots == withSlots {
		t.Error("hash unchanged when Slots added — should differ")
	}

	// Adding the same slot twice in different orders mustn't matter:
	// SlotTypes / Slots arrive sorted from Endpoint.Info; here we
	// verify the hash is stable for an identical-content manifest.
	clone := &mbapi.Manifest{
		Provider:      base.Provider,
		Version:       base.Version,
		Slots:         append([]mbapi.Slot(nil), base.Slots...),
		Contributions: append([]contribapi.Contribution(nil), base.Contributions...),
	}
	if computeHash(clone) != computeHash(base) {
		t.Error("identical manifests produced different hashes")
	}
}

func TestReHash_RecomputesAfterMutation(t *testing.T) {
	m := &mbapi.Manifest{Provider: "p", Version: "v"}
	ReHash(m)
	first := m.Hash

	m.Slots = []mbapi.Slot{{Name: "a", Type: "T"}}
	ReHash(m)
	if m.Hash == first {
		t.Error("ReHash didn't update Hash after mutation")
	}
}
