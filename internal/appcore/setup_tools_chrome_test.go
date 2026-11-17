// SPDX-License-Identifier: Apache-2.0

package appcore

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/publish"
)

// Chrome's winget step installs for the user alone: Google.Chrome is the
// enterprise MSI, machine-wide only, which winget refuses for --scope user
// (0x8A150010); Google.Chrome.EXE is Chrome's own per-user installer.
func TestChromeInstallsForTheUserAloneOnWindows(t *testing.T) {
	for _, r := range NewChromeToolDef().Recipes {
		if r.Kind == publish.RecipeWinget && r.Package != "Google.Chrome.EXE" {
			t.Errorf("Chrome's winget package %q has no user-scope installer: Google.Chrome.EXE has", r.Package)
		}
	}
}
