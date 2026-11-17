// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore/controlplane"
)

// `doctor` must never exec a control-plane binary.
//
// install-infra records all four in the installed registry — InfraOutdated reads
// them to know what this machine runs — and they carry source=marketplace like
// any other installed item. Doctor's only filter was that source, so it ran
// `<binary> sidecar config check` on each: `katyoti doctor` launched ktw-gui, a
// Wails desktop app, and OPENED A WINDOW mid-demo. ktw-hub would have tried to
// serve.
//
// This pins the skip at the level the bug lived: the predicate Doctor consults.
// A Doctor test proper needs an installed-registry fixture plus real binaries to
// exec, which is the machinery that made this path untested in the first place.
func TestInfraBinariesAreSkippedByDoctor(t *testing.T) {
	// Every control-plane binary must answer true — that is what makes Doctor
	// skip it. ktw-gui is the one that actually opened a window, so a regression
	// here is not cosmetic.
	for _, name := range controlplane.InfraBinaries {
		if !controlplane.IsInfra(name) {
			t.Errorf("IsInfra(%q) = false — doctor would exec it", name)
		}
	}
	// And an ordinary sidecar must still be checked, or doctor reports nothing.
	for _, name := range []string{"ktw-notify", "ktw-weather", "ktw-example"} {
		if controlplane.IsInfra(name) {
			t.Errorf("IsInfra(%q) = true — doctor would skip a real sidecar", name)
		}
	}
}
