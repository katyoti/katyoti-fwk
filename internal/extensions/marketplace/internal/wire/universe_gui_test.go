// SPDX-License-Identifier: Apache-2.0

package wire

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
)

// A solution's universe carries its GUI section: the universe it installs
// declares it in Univers.yaml. Installing again without one keeps it.
func TestUniverseGUIWrittenByASolution(t *testing.T) {
	t.Cleanup(cenv.ResetUniverseManifestCache)
	home := filepath.Join(t.TempDir(), "archi")
	gui := &cenv.UniverseGUI{Hide: []string{"panel:planning-deck", "runs"}, First: []string{"archifactory"}}
	if _, err := CreateUniverse(nil, Display{DisplayName: "ArchiFactory", GUI: gui}, home); err != nil {
		t.Fatal(err)
	}
	read := func() *cenv.UniverseGUI {
		cenv.ResetUniverseManifestCache()
		m, err := cenv.LoadUniverseManifest(application.EnvOf(home).HomeDir())
		if err != nil {
			t.Fatal(err)
		}
		return m.GUI
	}
	if got := read(); !reflect.DeepEqual(got, gui) {
		t.Fatalf("Univers.yaml gui = %+v, want %+v", got, gui)
	}
	if _, err := CreateUniverse(nil, Display{}, home); err != nil {
		t.Fatal(err)
	}
	if got := read(); !reflect.DeepEqual(got, gui) {
		t.Fatalf("installing again without a gui section dropped it: %+v", got)
	}
}
