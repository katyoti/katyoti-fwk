// SPDX-License-Identifier: Apache-2.0

package local

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/scripts/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// : a script's slot method advertises the script's run bound
// so the catalog gives callers (ActionService, the GUI's action dispatch) the
// wait the engine allows - the GUI keeps no blanket ceiling for scripts.
func TestAScriptSlotMethodAdvertisesItsRunBound(t *testing.T) {
	slot := ktsp.NewSlot("scripts", ktsp.DefaultInstance, "test", nil)
	addPerScriptMethod(slot, nil, api.ScriptInfo{Name: "build-site", Provider: "ktw-docs", TimeoutS: 900}, "build-site", nil, nil)
	def := slot.Def("build-site")
	if def == nil {
		t.Fatal("build-site not registered")
	}
	if got := def.Opts().TimeoutS; got != 900 {
		t.Errorf("TimeoutS = %d; want the script's declared 900", got)
	}
	if got := def.Info().TimeoutS; got != 900 {
		t.Errorf("advertised TimeoutS = %d; want 900", got)
	}
}
